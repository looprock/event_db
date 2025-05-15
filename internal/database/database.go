package database

import (
	"database/sql"
	"encoding/json"
	"example-api/internal/models"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Database struct {
	db *sql.DB
}

// NewPostgres creates a new Database instance using PostgreSQL connection info.
func NewPostgres(connStr string) (*Database, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	// Optionally: ping to check connection
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Database{db: db}, nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

// LogEventStatus logs the status of an event operation
// If eventID is 0, this logs to stdout instead of the database (avoids foreign key constraint violation)
func (d *Database) LogEventStatus(eventID int64, status string, errorMessage string) error {
	// If eventID is 0, it means the event hasn't been created yet
	// Just log to stdout to avoid foreign key constraint violation
	if eventID == 0 {
		log.Printf("Event log (pre-insert): status=%s, error=%s", status, errorMessage)
		return nil
	}
	
	// For valid event IDs, log to the database
	_, err := d.db.Exec(
		"INSERT INTO event_logs (event_id, status, error_message) VALUES ($1, $2, $3)",
		eventID,
		status,
		errorMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to log event status: %w", err)
	}
	return nil
}

func (d *Database) StoreEvent(event *models.EventRequest) (*models.Event, error) {
	tagsJSON, err := json.Marshal(event.Tags)
	if err != nil {
		// Log pre-insert error (will be logged to stdout since event ID is 0)
		_ = d.LogEventStatus(0, "error", fmt.Sprintf("failed to marshal tags: %v", err))
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}

	cleanData := strings.TrimRight(event.Data, "\r\n")
	log.Printf("DEBUG database: Original Data: %q, CleanData: %q (length=%d)", event.Data, cleanData, len(cleanData))

	var id int64
	log.Printf("DEBUG database: Executing SQL with params: tags=%s, data=%q, source=%s", 
		string(tagsJSON), cleanData, event.Source)
	err = d.db.QueryRow(
		"INSERT INTO events (tags, data, source, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
		string(tagsJSON),
		cleanData,
		event.Source,
		time.Now(),
	).Scan(&id)
	log.Printf("DEBUG database: Insert result: id=%d, err=%v", id, err)
	if err != nil {
		// Log pre-insert error (will be logged to stdout since event ID is 0)
		_ = d.LogEventStatus(0, "error", fmt.Sprintf("failed to insert event: %v", err))
		return nil, fmt.Errorf("failed to insert event: %w", err)
	}

	// Now that we have a valid event ID, log the success in the database
	if err := d.LogEventStatus(id, "success", ""); err != nil {
		log.Printf("Warning: Event was stored but failed to log success: %v", err)
	}

	result := &models.Event{
		ID:        id,
		Tags:      event.Tags,
		Data:      cleanData,
		Source:    event.Source,
		CreatedAt: time.Now(),
	}
	log.Printf("DEBUG database: Returning event result: %+v", result)
	return result, nil
}

func (d *Database) GetEventByID(id int64) (*models.Event, error) {
	var event models.Event
	var tagsJSON string
	var createdAt time.Time

	err := d.db.QueryRow(
		"SELECT id, tags, data, source, created_at FROM events WHERE id = $1",
		id,
	).Scan(&event.ID, &tagsJSON, &event.Data, &event.Source, &createdAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	event.CreatedAt = createdAt
	if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
		return nil, fmt.Errorf("failed to parse tags: %w", err)
	}

	return &event, nil
}

func (d *Database) GetEventsByTag(tag string) ([]models.Event, error) {
	rows, err := d.db.Query(
		`SELECT id, tags, data, source, created_at 
		FROM events 
		WHERE tags::text LIKE $1 
		ORDER BY created_at DESC`,
		fmt.Sprintf("%%\"%s\"%%", tag),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		var tagsJSON string
		var createdAt time.Time

		if err := rows.Scan(&event.ID, &tagsJSON, &event.Data, &event.Source, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		event.CreatedAt = createdAt
		if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
			return nil, fmt.Errorf("failed to parse tags: %w", err)
		}

		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return events, nil
}

// GetEventsByDate retrieves all events created on a specific date (YYYY-MM-DD)
func (d *Database) GetEventsByDate(date string) ([]models.Event, error) {
	// Handle empty date parameter
	if date == "" {
		return d.GetEventsByTag("")
	}
	
	// Validate date format (YYYY-MM-DD)
	_, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
	}
	
	// Create time bounds for the given date
	start := date + " 00:00:00"
	end := date + " 23:59:59.999999"
	
	log.Printf("Querying events between %s and %s", start, end)
	
	rows, err := d.db.Query(
		`SELECT id, tags, data, source, created_at 
		FROM events 
		WHERE created_at::date = $1::date
		ORDER BY created_at DESC`,
		date,
	)
	if err != nil {
		return nil, fmt.Errorf("database query failed: %w", err)
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		var tagsJSON string
		var createdAt time.Time
		if err := rows.Scan(&event.ID, &tagsJSON, &event.Data, &event.Source, &createdAt); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		event.CreatedAt = createdAt
		if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
			return nil, fmt.Errorf("error unmarshaling tags: %w", err)
		}
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	log.Printf("Found %d events for date %s", len(events), date)
	return events, nil
}

// GetAllTags retrieves all unique tags used in events
func (d *Database) GetAllTags() ([]string, error) {
	rows, err := d.db.Query("SELECT tags FROM events")
	if err != nil {
		return nil, fmt.Errorf("failed to query events for tags: %w", err)
	}
	defer rows.Close()
	
	// Use a map to ensure uniqueness of tags
	uniqueTags := make(map[string]bool)
	
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			return nil, fmt.Errorf("failed to scan tags row: %w", err)
		}
		
		var tags []string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			// If one event has invalid tags, we just skip it
			continue
		}
		
		// Add all tags to the uniqueTags map
		for _, tag := range tags {
			if tag != "" {
				uniqueTags[tag] = true
			}
		}
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	// Convert map keys to slice
	result := make([]string, 0, len(uniqueTags))
	for tag := range uniqueTags {
		result = append(result, tag)
	}
	
	// Sort tags alphabetically for consistency
	sort.Strings(result)
	
	return result, nil
}

// GetAllSources retrieves all unique sources used in events
func (d *Database) GetAllSources() ([]string, error) {
	rows, err := d.db.Query("SELECT DISTINCT source FROM events WHERE source != '' ORDER BY source")
	if err != nil {
		return nil, fmt.Errorf("failed to query events for sources: %w", err)
	}
	defer rows.Close()
	
	// Use a map to ensure uniqueness of sources
	uniqueSources := make(map[string]bool)
	
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, fmt.Errorf("failed to scan source row: %w", err)
		}
		
		if source != "" {
			uniqueSources[source] = true
		}
	}
	
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}
	
	// Convert map keys to slice
	result := make([]string, 0, len(uniqueSources))
	for source := range uniqueSources {
		result = append(result, source)
	}
	
	// Sort sources alphabetically for consistency
	sort.Strings(result)
	
	return result, nil
}

// GetEventsBySource retrieves all events with a specific source
func (d *Database) GetEventsBySource(source string) ([]models.Event, error) {
	// If source is empty, return all events
	if source == "" {
		return d.GetEventsByTag("")
	}

	log.Printf("Querying events with source: %s", source)
	
	rows, err := d.db.Query(
		`SELECT id, tags, data, source, created_at 
		FROM events 
		WHERE source ILIKE $1 
		ORDER BY created_at DESC`,
		source,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query events by source: %w", err)
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		var tagsJSON string
		var createdAt time.Time

		if err := rows.Scan(&event.ID, &tagsJSON, &event.Data, &event.Source, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		event.CreatedAt = createdAt
		if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
			return nil, fmt.Errorf("failed to parse tags: %w", err)
		}

		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	log.Printf("Found %d events with source: %s", len(events), source)
	return events, nil
}

// SaveEvent stores an Event in the database
func (d *Database) SaveEvent(event *models.Event) error {
	tagsJSON, err := json.Marshal(event.Tags)
	if err != nil {
		// Log pre-insert error (will be logged to stdout since event ID is 0)
		_ = d.LogEventStatus(0, "error", fmt.Sprintf("failed to marshal tags: %v", err))
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	cleanData := strings.TrimRight(event.Data, "\r\n")
	
	var id int64
	err = d.db.QueryRow(
		"INSERT INTO events (tags, data, source, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
		string(tagsJSON),
		cleanData,
		event.Source,
		event.CreatedAt,
	).Scan(&id)
	
	if err != nil {
		// Log pre-insert error (will be logged to stdout since event ID is 0)
		_ = d.LogEventStatus(0, "error", fmt.Sprintf("failed to insert event: %v", err))
		return fmt.Errorf("failed to insert event: %w", err)
	}

	// Now that we have a valid event ID, log the success in the database
	if err := d.LogEventStatus(id, "success", ""); err != nil {
		log.Printf("Warning: Event was stored but failed to log success: %v", err)
	}

	// Update the ID of the passed event
	event.ID = id
	
	return nil
}

// UpdateEvent updates an existing Event in the database
func (d *Database) UpdateEvent(event *models.Event) error {
	// Ensure we have a valid tags array
	if event.Tags == nil {
		event.Tags = []string{}
	}
	
	// Log the tags for debugging
	log.Printf("Updating event %d with tags: %v", event.ID, event.Tags)
	
	// Marshal tags to JSON
	tagsJSON, err := json.Marshal(event.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}
	
	log.Printf("Tags JSON for event %d: %s", event.ID, string(tagsJSON))

	// Clean data by removing trailing whitespace
	cleanData := strings.TrimRight(event.Data, "\r\n")

	// Execute update query
	result, err := d.db.Exec(
		"UPDATE events SET tags = $1, data = $2, source = $3 WHERE id = $4",
		string(tagsJSON),
		cleanData,
		event.Source,
		event.ID,
	)
	
	if err != nil {
		log.Printf("Error in SQL update for event %d: %v", event.ID, err)
		return fmt.Errorf("failed to update event: %w", err)
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("event with ID %d not found", event.ID)
	}

	// Log the update
	if err := d.LogEventStatus(event.ID, "updated", ""); err != nil {
		log.Printf("Warning: Event was updated but failed to log update: %v", err)
	}
	
	// Verify the update
	updated, err := d.GetEventByID(event.ID)
	if err == nil && updated != nil {
		log.Printf("Verified update for event %d. Tags now: %v", event.ID, updated.Tags)
	}
	
	return nil
}

// DeleteEvent removes an event from the database by ID
func (d *Database) DeleteEvent(id int64) error {
	// Start a transaction
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	// Defer a rollback in case anything fails
	defer tx.Rollback()
	
	// First delete related logs to avoid foreign key constraint
	_, err = tx.Exec("DELETE FROM event_logs WHERE event_id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete event logs: %w", err)
	}
	
	// Now delete the event
	result, err := tx.Exec("DELETE FROM events WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}
	
	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("event with ID %d not found", id)
	}
	
	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	// Log the deletion (outside the transaction)
	log.Printf("Event %d deleted successfully", id)
	
	return nil
}
