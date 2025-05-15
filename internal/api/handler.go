package api

import (
	"bytes"
	"example-api/internal/database"
	"example-api/internal/models"
	"example-api/internal/utils"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	db *database.Database
}

func New(db *database.Database) *Handler {
	return &Handler{db: db}
}

// For debugging - prints struct field names and their json tags
func init() {
	t := reflect.TypeOf(struct {
		Data struct {
			From                    string              `json:"from"`
			To                      string              `json:"to"`
			Subject                 string              `json:"subject"`
			Data                    string              `json:"body"`
			Cc                      []string            `json:"cc,omitempty"`
			Bcc                     []string            `json:"bcc,omitempty"`
		} `json:"data"`
	}{})
	
	field, _ := t.FieldByName("Data")
	log.Printf("DEBUG STRUCT: Outer field 'Data' has json tag: %q", field.Tag.Get("json"))
	
	inner := field.Type
	for i := 0; i < inner.NumField(); i++ {
		f := inner.Field(i)
		log.Printf("DEBUG STRUCT: Inner field %q has json tag: %q", f.Name, f.Tag.Get("json"))
	}
}

// AuthMiddleware checks for a valid token in the Authorization header
func AuthMiddleware(validToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			log.Printf("Auth failed: No token provided for %s %s", c.Request.Method, c.Request.URL.Path)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "No authorization token provided"})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		if token != validToken {
			log.Printf("Auth failed: Invalid token provided for %s %s", c.Request.Method, c.Request.URL.Path)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		log.Printf("Auth successful for %s %s", c.Request.Method, c.Request.URL.Path)
		c.Next()
	}
}

// HandleEventReceive processes incoming event data
func (h *Handler) HandleEventReceive(c *gin.Context) {
	log.Printf("Received event request with Content-Type: %s", c.GetHeader("Content-Type"))

	var incoming struct {
		Data struct {
			From                    string              `json:"from"`
			To                      string              `json:"to"`
			Subject                 string              `json:"subject"`
			Data                    string              `json:"body"`
			Cc                      []string            `json:"cc,omitempty"`
			Bcc                     []string            `json:"bcc,omitempty"`
			MessageID               string              `json:"message_id,omitempty"`
			InReplyTo               string              `json:"in_reply_to,omitempty"`
			References              []string            `json:"references,omitempty"`
			Date                    time.Time           `json:"date"`
			ContentType             string              `json:"content_type,omitempty"`
			ContentTransferEncoding string              `json:"content_transfer_encoding,omitempty"`
			HTMLBody                string              `json:"html_body,omitempty"`
			PlainBody               string              `json:"plain_body,omitempty"`
			ReceivedFrom            string              `json:"received_from,omitempty"`
			ReceivedAt              time.Time           `json:"received_at"`
			AuthenticatedAs         string              `json:"authenticated_as,omitempty"`
			Headers                 map[string][]string `json:"headers,omitempty"`
		} `json:"data"`
		Source string `json:"source"`
	}

	// Capture raw JSON for debugging
	var rawBody []byte
	if c.Request.Body != nil {
		rawBody, _ = io.ReadAll(c.Request.Body)
		// Restore body for binding
		c.Request.Body = io.NopCloser(bytes.NewBuffer(rawBody))
		log.Printf("DEBUG: Raw JSON body: %s", string(rawBody))
	}

	if err := c.ShouldBindJSON(&incoming); err != nil {
		log.Printf("Failed to decode JSON: %v", err)
		log.Printf("Raw request body: %s", string(rawBody))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	log.Printf("Decoded incoming data: %+v", incoming)
	log.Printf("DEBUG: Body field from JSON: %q", incoming.Data.Data)

	// Extract tags from subject
	tags := strings.Fields(incoming.Data.Subject)
	if len(tags) == 0 {
		tags = []string{"untagged"}
	}
	// Convert all tags to lowercase
	for i, tag := range tags {
		tags[i] = strings.ToLower(tag)
	}
	log.Printf("Extracted tags: %v", tags)

	// --- Begin: Extract only the inline MIME part if present ---
	var contentToProcess string
	// Check if Data field is empty, use PlainBody as fallback
	if incoming.Data.Data == "" && incoming.Data.PlainBody != "" {
		log.Printf("DEBUG: Data field (mapped from 'body' in JSON) is empty, using PlainBody instead")
		log.Printf("DEBUG: PlainBody content: %q", incoming.Data.PlainBody)
		contentToProcess = incoming.Data.PlainBody
	} else {
		log.Printf("DEBUG: Using Data field (mapped from 'body' in JSON): %q", incoming.Data.Data)
		contentToProcess = incoming.Data.Data
	}
	
	// Try a simple content extraction first - look for content after blank line
	// This works for simple MIME messages that follow the standard format
	actualContent := extractSimpleContent(contentToProcess)
	if actualContent != "" {
		log.Printf("Successfully extracted simple content: %q", actualContent)
		dataToStore := actualContent
		// Store in database
		event := &models.EventRequest{
			Tags:   tags,
			Data:   dataToStore,
			Source: incoming.Source,
		}
		log.Printf("Storing event with simple extraction: %+v", event)
		storedEvent, err := h.db.StoreEvent(event)
		if err != nil {
			log.Printf("Failed to store event: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store event"})
			return
		}
		log.Printf("Successfully stored event with ID: %d", storedEvent.ID)
		c.JSON(http.StatusCreated, storedEvent)
		return
	}
	
	// If simple extraction didn't work, try the more complex MIME parsing
	log.Printf("Simple extraction failed, trying MIME parsing for content: %q", contentToProcess)
	plainData, err := utils.ExtractPlain([]byte(contentToProcess))
	if err != nil {
		log.Printf("Failed to extract plain data: %v", err)
		plainData = contentToProcess // fallback to original
	}
	log.Printf("Result after MIME extraction: %q", plainData)
	dataToStore := plainData
	// --- End: Extract only the inline MIME part if present ---

	// Store in database
	event := &models.EventRequest{
		Tags:   tags,
		Data:   dataToStore,
		Source: incoming.Source,
	}

	log.Printf("Storing event: %+v", event)
	log.Printf("DEBUG: EventRequest struct: tags=%v, data=%q (length=%d), source=%s", 
		event.Tags, event.Data, len(event.Data), event.Source)
	storedEvent, err := h.db.StoreEvent(event)
	if err != nil {
		log.Printf("Failed to store event: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store event"})
		return
	}

	log.Printf("Successfully stored event with ID: %d", storedEvent.ID)
	c.JSON(http.StatusCreated, storedEvent)
}

// HandleGetEventByID handles GET requests to retrieve an event by ID
func (h *Handler) HandleGetEventByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}

	event, err := h.db.GetEventByID(id)
	if err != nil {
		log.Printf("Failed to get event: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve event"})
		return
	}

	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	c.JSON(http.StatusOK, event)
}

// HandleGetEventsByTag handles GET requests to retrieve events by tag
func (h *Handler) HandleGetEventsByTag(c *gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tag parameter is required"})
		return
	}

	log.Printf("Searching for events with tag: %s", tag)
	events, err := h.db.GetEventsByTag(tag)
	if err != nil {
		log.Printf("Failed to get events by tag %q: %+v", tag, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve events: %v", err)})
		return
	}

	log.Printf("Found %d events with tag %q", len(events), tag)
	response := models.EventResponse{
		Events: events,
		Total:  len(events),
	}

	c.JSON(http.StatusOK, response)
}

// extractSimpleContent tries to extract content from MIME messages by looking for content after headers
func extractSimpleContent(content string) string {
	// Split by lines
	lines := strings.Split(content, "\n")
	
	// Find a blank line (which typically separates headers from content)
	inContent := false
	var contentLines []string
	
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		
		// When we find a blank line, we're transitioning into content
		if trimmedLine == "" && !inContent {
			inContent = true
			continue
		}
		
		// If we're in the content section and hit a boundary line, we're done
		if inContent && strings.HasPrefix(trimmedLine, "--") {
			break
		}
		
		// If we're in content, collect the line
		if inContent {
			contentLines = append(contentLines, line)
		}
	}
	
	// Join the content lines
	if len(contentLines) > 0 {
		return strings.TrimSpace(strings.Join(contentLines, "\n"))
	}
	
	return ""
}

// HandleGetEventsByDate handles GET requests to retrieve events by date (YYYY-MM-DD)
func (h *Handler) HandleGetEventsByDate(c *gin.Context) {
	date := c.Query("date")
	if date == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Date parameter is required (YYYY-MM-DD)"})
		return
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Use YYYY-MM-DD."})
		return
	}

	events, err := h.db.GetEventsByDate(date)
	if err != nil {
		log.Printf("Failed to get events by date %q: %+v", date, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve events: %v", err)})
		return
	}

	log.Printf("Found %d events for date %q", len(events), date)
	response := models.EventResponse{
		Events: events,
		Total:  len(events),
	}

	c.JSON(http.StatusOK, response)
}
