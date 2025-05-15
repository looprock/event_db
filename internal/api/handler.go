package api

import (
	"example-api/internal/database"
	"example-api/internal/models"
	"example-api/internal/utils"
	"fmt"
	"log"
	"net/http"
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

	if err := c.ShouldBindJSON(&incoming); err != nil {
		log.Printf("Failed to decode JSON: %v", err)
		log.Printf("Raw request body: %+v", c.Request.Body)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	log.Printf("Decoded incoming data: %+v", incoming)

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
	plainData, err := utils.ExtractPlain([]byte(incoming.Data.Data))
	if err != nil {
		log.Printf("Failed to extract plain data: %v", err)
		plainData = incoming.Data.Data // fallback to original
	}
	dataToStore := plainData
	// --- End: Extract only the inline MIME part if present ---

	// Store in database
	event := &models.EventRequest{
		Tags:   tags,
		Data:   dataToStore,
		Source: incoming.Source,
	}

	log.Printf("Storing event: %+v", event)
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
