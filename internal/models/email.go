package models

import "time"

type Event struct {
	ID        int64     `json:"id"`
	Tags      []string  `json:"tags"`
	Body      string    `json:"body"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type EventRequest struct {
	Tags   []string `json:"tags"`
	Body   string   `json:"body"`
	Source string   `json:"source"`
}

// EventResponse represents a list of events
type EventResponse struct {
	Events []Event `json:"events"`
	Total  int     `json:"total"`
}
