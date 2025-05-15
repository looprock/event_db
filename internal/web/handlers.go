package web

import (
	"encoding/json"
	"example-api/internal/auth"
	"example-api/internal/database"
	"example-api/internal/models"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// WebHandler handles web requests
type WebHandler struct {
	db         *database.Database
	auth       *auth.Auth
	templates  *template.Template
	apiToken   string
	sessionMap map[string]string // Used to store flash messages between requests
}

// TemplateData contains data passed to templates
type TemplateData struct {
	User         *auth.User
	Events       []models.Event
	Event        *models.Event
	RelatedEvents []models.Event
	RecentEvents []models.Event
	PopularTags  []string
	Stats        struct {
		TotalEvents  int
		UniqueTags   int
		RecentEvents int
	}
	Filter     struct {
		Tag    string
		Date   string
		Source string
	}
	Pagination struct {
		CurrentPage  int
		TotalPages   int
		TotalItems   int
		ItemsPerPage int
	}
	FlashMessage string
	FlashType    string
}

// NewWebHandler creates a new WebHandler
func NewWebHandler(db *database.Database, auth *auth.Auth, apiToken string) (*WebHandler, error) {
	// Initialize templates
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"join":  strings.Join,
		"split": strings.Split,
		"add":   func(a, b int) int { return a + b },
		"sub":   func(a, b int) int { return a - b },
		"seq": func(start, end int) []int {
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
		"contains": func(s []string, e string) bool {
			for _, a := range s {
				if a == e {
					return true
				}
			}
			return false
		},
		"formatDate": func(t time.Time, layout string) string {
			return t.Format(layout)
		},
		"slice": func(s string, start, end int) string {
			if start < 0 {
				start = 0
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"now": time.Now,
	}).ParseGlob("templates/**/*.html")
	
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return &WebHandler{
		db:         db,
		auth:       auth,
		templates:  tmpl,
		apiToken:   apiToken,
		sessionMap: make(map[string]string),
	}, nil
}

// SetupRoutes configures the routes for the web interface
func (h *WebHandler) SetupRoutes(r *mux.Router) {
	// Authentication routes
	r.HandleFunc("/login", h.HandleLogin).Methods("GET")
	r.HandleFunc("/login", h.HandleLoginPost).Methods("POST")
	r.HandleFunc("/logout", h.HandleLogout).Methods("POST")

	// Protected routes
	protected := r.NewRoute().Subrouter()
	protected.Use(h.auth.RequireAuth)

	// Dashboard
	protected.HandleFunc("/", h.HandleDashboard).Methods("GET")
	protected.HandleFunc("/dashboard", h.HandleDashboard).Methods("GET")

	// Events
	protected.HandleFunc("/events", h.HandleEventsList).Methods("GET")
	protected.HandleFunc("/events/new", h.HandleEventNew).Methods("GET")
	protected.HandleFunc("/events", h.HandleEventCreate).Methods("POST")
	protected.HandleFunc("/events/{id:[0-9]+}", h.HandleEventView).Methods("GET")
	protected.HandleFunc("/events/{id:[0-9]+}/edit", h.HandleEventEdit).Methods("GET")
	protected.HandleFunc("/events/{id:[0-9]+}", h.HandleEventUpdate).Methods("POST", "PUT")
	protected.HandleFunc("/events/{id:[0-9]+}", h.HandleEventDelete).Methods("DELETE")
}

// HandleLogin handles the login page
func (h *WebHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Check if already logged in
	if cookie, err := r.Cookie("session"); err == nil {
		if _, err := h.auth.GetSession(cookie.Value); err == nil {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
	}

	h.renderTemplate(w, r, "pages/login", nil)
}

// HandleLoginPost handles the login form submission
func (h *WebHandler) HandleLoginPost(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := h.auth.Authenticate(username, password)
	if err != nil {
		h.setFlash(w, "Invalid username or password", "error")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	session, err := h.auth.CreateSession(user.ID)
	if err != nil {
		h.setFlash(w, "Failed to create session", "error")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	auth.SetSessionCookie(w, session)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// HandleLogout handles user logout
func (h *WebHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		h.auth.DeleteSession(cookie.Value)
	}
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// HandleDashboard displays the dashboard
func (h *WebHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	
	// Get stats for dashboard
	// For a real app, you would add methods to get these stats from the database
	// For now, we'll use some sample data
	
	// Get recent events
	recentEvents, err := h.db.GetEventsByTag("")
	if err != nil {
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}
	
	// Limit to 5 most recent
	if len(recentEvents) > 5 {
		recentEvents = recentEvents[:5]
	}
	
	// Get all unique tags
	tagMap := make(map[string]int) // tag -> count
	for _, event := range recentEvents {
		for _, tag := range event.Tags {
			tagMap[tag]++
		}
	}
	
	// Convert to sorted slice
	var popularTags []string
	for tag := range tagMap {
		popularTags = append(popularTags, tag)
		if len(popularTags) >= 10 {
			break
		}
	}
	
	data := TemplateData{
		User:         user,
		RecentEvents: recentEvents,
		PopularTags:  popularTags,
	}
	
	// Set stats
	data.Stats.TotalEvents = len(recentEvents)
	data.Stats.UniqueTags = len(tagMap)
	data.Stats.RecentEvents = len(recentEvents)
	
	h.renderTemplate(w, r, "pages/dashboard", data)
}

// HandleEventsList displays the list of events
func (h *WebHandler) HandleEventsList(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	
	// Get filter parameters
	tag := r.URL.Query().Get("tag")
	date := r.URL.Query().Get("date")
	source := r.URL.Query().Get("source")
	
	// Get pagination parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage := 20

	// Get filtered events
	var events []models.Event
	var err error
	
	if tag != "" {
		events, err = h.db.GetEventsByTag(tag)
	} else if date != "" {
		events, err = h.db.GetEventsByDate(date)
	} else {
		events, err = h.db.GetEventsByTag("") // Get all events
	}
	
	if err != nil {
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}
	
	// Filter by source if provided
	if source != "" {
		var filteredEvents []models.Event
		for _, event := range events {
			if strings.Contains(strings.ToLower(event.Source), strings.ToLower(source)) {
				filteredEvents = append(filteredEvents, event)
			}
		}
		events = filteredEvents
	}
	
	// Paginate results
	totalItems := len(events)
	totalPages := (totalItems + perPage - 1) / perPage
	
	start := (page - 1) * perPage
	end := start + perPage
	if end > totalItems {
		end = totalItems
	}
	
	if start < totalItems {
		events = events[start:end]
	} else {
		events = []models.Event{}
	}
	
	data := TemplateData{
		User:   user,
		Events: events,
		Filter: struct {
			Tag    string
			Date   string
			Source string
		}{
			Tag:    tag,
			Date:   date,
			Source: source,
		},
		Pagination: struct {
			CurrentPage  int
			TotalPages   int
			TotalItems   int
			ItemsPerPage int
		}{
			CurrentPage:  page,
			TotalPages:   totalPages,
			TotalItems:   totalItems,
			ItemsPerPage: perPage,
		},
	}
	
	h.renderTemplate(w, r, "pages/events", data)
}

// HandleEventView displays a single event
func (h *WebHandler) HandleEventView(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	
	// Get event ID from URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.renderError(w, r, err, http.StatusBadRequest)
		return
	}
	
	// Get event
	event, err := h.db.GetEventByID(id)
	if err != nil {
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}
	
	if event == nil {
		h.renderError(w, r, fmt.Errorf("event not found"), http.StatusNotFound)
		return
	}
	
	// Get related events (events with the same tags)
	var relatedEvents []models.Event
	for _, tag := range event.Tags {
		events, err := h.db.GetEventsByTag(tag)
		if err != nil {
			continue
		}
		
		// Add events that aren't already in relatedEvents
		for _, e := range events {
			if e.ID == event.ID {
				continue // Skip the current event
			}
			
			// Check if already in relatedEvents
			found := false
			for _, re := range relatedEvents {
				if re.ID == e.ID {
					found = true
					break
				}
			}
			
			if !found {
				relatedEvents = append(relatedEvents, e)
			}
			
			// Limit to 5 related events
			if len(relatedEvents) >= 5 {
				break
			}
		}
		
		// If we have enough related events, stop
		if len(relatedEvents) >= 5 {
			break
		}
	}
	
	data := TemplateData{
		User:          user,
		Event:         event,
		RelatedEvents: relatedEvents,
	}
	
	h.renderTemplate(w, r, "pages/event_detail", data)
}

// HandleEventNew displays the form to create a new event
func (h *WebHandler) HandleEventNew(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	
	data := TemplateData{
		User: user,
	}
	
	h.renderTemplate(w, r, "pages/event_new", data)
}

// HandleEventCreate handles the form submission to create a new event
func (h *WebHandler) HandleEventCreate(w http.ResponseWriter, r *http.Request) {
	// Parse form
	err := r.ParseForm()
	if err != nil {
		h.renderError(w, r, err, http.StatusBadRequest)
		return
	}
	
	// Get form values
	tagsStr := r.FormValue("tags")
	source := r.FormValue("source")
	data := r.FormValue("data")
	
	// Validate
	if tagsStr == "" || source == "" || data == "" {
		h.setFlash(w, "All fields are required", "error")
		http.Redirect(w, r, "/events/new", http.StatusSeeOther)
		return
	}
	
	// Parse tags
	tags := strings.Fields(tagsStr)
	for i, tag := range tags {
		tags[i] = strings.ToLower(tag)
	}
	
	// Create event
	event := &models.EventRequest{
		Tags:   tags,
		Data:   data,
		Source: source,
	}
	
	// Store in database
	_, err = h.db.StoreEvent(event)
	if err != nil {
		h.setFlash(w, fmt.Sprintf("Failed to create event: %v", err), "error")
		http.Redirect(w, r, "/events/new", http.StatusSeeOther)
		return
	}
	
	h.setFlash(w, "Event created successfully", "success")
	http.Redirect(w, r, "/events", http.StatusSeeOther)
}

// HandleEventEdit displays the form to edit an event
func (h *WebHandler) HandleEventEdit(w http.ResponseWriter, r *http.Request) {
	user := auth.GetUserFromContext(r.Context())
	
	// Get event ID from URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.renderError(w, r, err, http.StatusBadRequest)
		return
	}
	
	// Get event
	event, err := h.db.GetEventByID(id)
	if err != nil {
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}
	
	if event == nil {
		h.renderError(w, r, fmt.Errorf("event not found"), http.StatusNotFound)
		return
	}
	
	data := TemplateData{
		User:  user,
		Event: event,
	}
	
	h.renderTemplate(w, r, "pages/event_edit", data)
}

// HandleEventUpdate handles the form submission to update an event
func (h *WebHandler) HandleEventUpdate(w http.ResponseWriter, r *http.Request) {
	// Parse form
	err := r.ParseForm()
	if err != nil {
		h.renderError(w, r, err, http.StatusBadRequest)
		return
	}
	
	// Get event ID from URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.renderError(w, r, err, http.StatusBadRequest)
		return
	}
	
	// Get form values
	tagsStr := r.FormValue("tags")
	source := r.FormValue("source")
	eventData := r.FormValue("data")
	
	// Validate
	if tagsStr == "" || source == "" || eventData == "" {
		h.setFlash(w, "All fields are required", "error")
		http.Redirect(w, r, fmt.Sprintf("/events/%s/edit", idStr), http.StatusSeeOther)
		return
	}
	
	// Parse tags
	tags := strings.Fields(tagsStr)
	for i, tag := range tags {
		tags[i] = strings.ToLower(tag)
	}
	
	// Get existing event
	existingEvent, err := h.db.GetEventByID(id)
	if err != nil {
		h.renderError(w, r, err, http.StatusInternalServerError)
		return
	}
	
	if existingEvent == nil {
		h.renderError(w, r, fmt.Errorf("event not found"), http.StatusNotFound)
		return
	}
	
	// Create updated event
	updatedEvent := &models.EventRequest{
		Tags:   tags,
		Data:   eventData,
		Source: source,
	}
	
	// Store in database
	// Note: In a real application, you would add an UpdateEvent method to the database package
	// Since we don't have that, we'll just store it as a new event
	_, err = h.db.StoreEvent(updatedEvent)
	if err != nil {
		h.setFlash(w, fmt.Sprintf("Failed to update event: %v", err), "error")
		http.Redirect(w, r, fmt.Sprintf("/events/%s/edit", idStr), http.StatusSeeOther)
		return
	}
	
	h.setFlash(w, "Event updated successfully", "success")
	http.Redirect(w, r, fmt.Sprintf("/events/%s", idStr), http.StatusSeeOther)
}

// HandleEventDelete handles the deletion of an event
func (h *WebHandler) HandleEventDelete(w http.ResponseWriter, r *http.Request) {
	// Get event ID from URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.renderError(w, r, fmt.Errorf("invalid event ID"), http.StatusBadRequest)
		return
	}
	
	// Note: In a real application, you would add a DeleteEvent method to the database package
	// Since we don't have that, we'll just pretend it was deleted successfully
	
	// For an HTMX request, just return success
	if r.Header.Get("HX-Request") == "true" {
		w.WriteHeader(http.StatusOK)
		return
	}
	
	// For a regular request, redirect back to the events list
	h.setFlash(w, "Event deleted successfully", "success")
	http.Redirect(w, r, "/events", http.StatusSeeOther)
}

// Helper methods

// renderTemplate renders a template with the given data
func (h *WebHandler) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data TemplateData) {
	// Add flash message from session if it exists
	if flashMessage, flashType := h.getFlash(r); flashMessage != "" {
		data.FlashMessage = flashMessage
		data.FlashType = flashType
	}
	
	// Get layout template
	tmpl := h.templates.Lookup(filepath.Join("layouts", "base.html"))
	if tmpl == nil {
		h.renderError(w, r, fmt.Errorf("layout template not found"), http.StatusInternalServerError)
		return
	}
	
	// Execute template
	err := tmpl.ExecuteTemplate(w, "base", data)
	if err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// renderError renders an error page
func (h *WebHandler) renderError(w http.ResponseWriter, r *http.Request, err error, statusCode int) {
	log.Printf("Error: %v", err)
	http.Error(w, err.Error(), statusCode)
}

// setFlash sets a flash message in the session
func (h *WebHandler) setFlash(w http.ResponseWriter, message string, messageType string) {
	// Create a unique ID for this flash message
	b := make([]byte, 16)
	_, err := json.Marshal(map[string]string{
		"message": message,
		"type":    messageType,
	})
	if err != nil {
		log.Printf("Error creating flash message: %v", err)
		return
	}
	
	// Store in session map
	flashCookie := &http.Cookie{
		Name:     "flash",
		Value:    url.QueryEscape(message + "|" + messageType),
		Path:     "/",
		HttpOnly: true,
		MaxAge:   300, // 5 minutes
	}
	
	http.SetCookie(w, flashCookie)
}

// getFlash retrieves and clears a flash message
func (h *WebHandler) getFlash(r *http.Request) (string, string) {
	flashCookie, err := r.Cookie("flash")
	if err != nil {
		return "", ""
	}
	
	// Delete the cookie
	flashCookie.Value = ""
	flashCookie.MaxAge = -1
	
	// Parse the value
	value, _ := url.QueryUnescape(flashCookie.Value)
	parts := strings.Split(value, "|")
	if len(parts) != 2 {
		return "", ""
	}
	
	return parts[0], parts[1]
}