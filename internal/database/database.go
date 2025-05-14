package database

import (
	"database/sql"
	"encoding/json"
	"example-api/internal/models"
	"fmt"
	"log"
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
func (d *Database) LogEventStatus(eventID int64, status string, errorMessage string) error {
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
		if logErr := d.LogEventStatus(0, "error", fmt.Sprintf("failed to marshal tags: %v", err)); logErr != nil {
			log.Printf("Failed to log error: %v", logErr)
		}
		return nil, err
	}

	cleanData := strings.TrimRight(event.Data, "\r\n")

	var id int64
	err = d.db.QueryRow(
		"INSERT INTO events (tags, data, source, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
		string(tagsJSON),
		cleanData,
		event.Source,
		time.Now(),
	).Scan(&id)
	if err != nil {
		if logErr := d.LogEventStatus(0, "error", fmt.Sprintf("failed to insert event: %v", err)); logErr != nil {
			log.Printf("Failed to log error: %v", logErr)
		}
		return nil, err
	}

	if err := d.LogEventStatus(id, "success", ""); err != nil {
		log.Printf("Failed to log success: %v", err)
	}

	return &models.Event{
		ID:        id,
		Tags:      event.Tags,
		Data:      cleanData,
		Source:    event.Source,
		CreatedAt: time.Now(),
	}, nil
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
	start := date + " 00:00:00"
	end := date + " 23:59:59.999999"
	rows, err := d.db.Query(
		`SELECT id, tags, data, source, created_at 
		FROM events 
		WHERE created_at >= $1 AND created_at <= $2 
		ORDER BY created_at ASC`,
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		var tagsJSON string
		var createdAt time.Time
		if err := rows.Scan(&event.ID, &tagsJSON, &event.Data, &event.Source, &createdAt); err != nil {
			return nil, err
		}
		event.CreatedAt = createdAt
		if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}
