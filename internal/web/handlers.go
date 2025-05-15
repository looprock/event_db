package web

import (
	"example-api/internal/auth"
	"example-api/internal/database"
	"example-api/internal/models"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
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
	Tags         []string
	Sources      []string
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
	// Get the working directory
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	
	// Create template function map
	funcMap := template.FuncMap{
		"join":  strings.Join,
		"split": strings.Split,
		"add":   func(a, b int) int { return a + b },
		"sub":   func(a, b int) int { return a - b },
		"slice": func(s string, start, end int) string {
			if start < 0 {
				start = 0
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"now": time.Now,
	}
	
	// Initialize templates with simple approach
	tmpl := template.New("").Funcs(funcMap)
	
	// Define template directories to scan
	templateRootDir := filepath.Join(workingDir, "templates")
	log.Printf("Loading templates from: %s", templateRootDir)
	
	// Find all template files recursively
	var templateFiles []string
	
	// Helper function to walk through directories and find template files
	err = filepath.Walk(templateRootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".html") {
			log.Printf("Found template: %s", path)
			templateFiles = append(templateFiles, path)
		}
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("error scanning templates: %w", err)
	}
	
	// Parse all templates at once
	tmpl, err = tmpl.ParseFiles(templateFiles...)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %w", err)
	}
	
	// List all templates that were loaded
	for _, t := range tmpl.Templates() {
		log.Printf("Template loaded: %s", t.Name())
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
	// Serve static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	r.PathPrefix("/public/").Handler(http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))
	
	// Authentication routes
	r.HandleFunc("/login", h.HandleLogin).Methods("GET")
	r.HandleFunc("/login", h.HandleLoginPost).Methods("POST")
	r.HandleFunc("/logout", h.HandleLogout).Methods("GET", "POST")

	// Root route handler - will show welcome page when logged out, events when logged in
	r.HandleFunc("/", h.HandleRoot).Methods("GET")
	
	// Events redirect for backward compatibility with existing links
	r.HandleFunc("/events", h.HandleEventsRedirect).Methods("GET")
	
	// Protected routes
	protected := r.NewRoute().Subrouter()
	protected.Use(h.auth.RequireAuth)
	protected.HandleFunc("/events/new", h.HandleCreateEvent).Methods("GET")
	protected.HandleFunc("/events/new", h.HandleCreateEventPost).Methods("POST")
	protected.HandleFunc("/events/{id}", h.HandleViewEvent).Methods("GET")
	protected.HandleFunc("/events/{id}/edit", h.HandleEditEvent).Methods("GET")
	protected.HandleFunc("/events/{id}/edit", h.HandleEditEventPost).Methods("POST")
	protected.HandleFunc("/events/{id}/delete", h.HandleDeleteEvent).Methods("GET")
}

// renderTemplate is a helper function to render templates with proper content
func (h *WebHandler) renderTemplate(w http.ResponseWriter, name string, data TemplateData) {
	// Set content type for all templates
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Execute the named template
	err := h.templates.ExecuteTemplate(w, name, data)
	
	if err != nil {
		log.Printf("Error rendering template %s: %v", name, err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// HandleLogin handles the login page
func (h *WebHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Check if user is already logged in
	if cookie, err := r.Cookie("session"); err == nil {
		if session, err := h.auth.GetSession(cookie.Value); err == nil {
			if user, _ := h.auth.GetUserByID(session.UserID); user != nil {
				// User is logged in, redirect to home (which will show events)
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}
	}
	
	data := TemplateData{}
	
	// Set data properties if needed
	if msg, ok := r.URL.Query()["error"]; ok && len(msg) > 0 {
		data.FlashMessage = msg[0]
		data.FlashType = "error"
	}
	
	// Set content type
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Execute template directly
	err := h.templates.ExecuteTemplate(w, "login.html", data)
	if err != nil {
		log.Printf("Error rendering login template: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// HandleLoginPost handles the login form submission
func (h *WebHandler) HandleLoginPost(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := h.auth.Authenticate(username, password)
	if err != nil {
		// Redirect back to login with error
		http.Redirect(w, r, "/login?error=Invalid+username+or+password.+Please+try+again.", http.StatusSeeOther)
		return
	}

	session, err := h.auth.CreateSession(user.ID)
	if err != nil {
		http.Redirect(w, r, "/login?error=Failed+to+create+session.+Please+try+again+later.", http.StatusSeeOther)
		return
	}

	auth.SetSessionCookie(w, session)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleLogout handles user logout
func (h *WebHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Handle both GET and POST requests for logout
	if cookie, err := r.Cookie("session"); err == nil {
		h.auth.DeleteSession(cookie.Value)
	}
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}



// HandleRoot displays either welcome page or events based on login status
func (h *WebHandler) HandleRoot(w http.ResponseWriter, r *http.Request) {
	// Check if user is logged in
	var user *auth.User
	if cookie, err := r.Cookie("session"); err == nil {
		if session, err := h.auth.GetSession(cookie.Value); err == nil {
			user, _ = h.auth.GetUserByID(session.UserID)
		}
	}
	
	// If user is logged in, show events list
	if user != nil {
		h.displayEventsList(w, r, user)
		return
	}
	
	// User is not logged in, show welcome page
	data := TemplateData{}
	
	// Set content type
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Execute template
	err := h.templates.ExecuteTemplate(w, "index.html", data)
	if err != nil {
		log.Printf("Error rendering index template: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// HandleEventsRedirect redirects /events to root while preserving query parameters
func (h *WebHandler) HandleEventsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/?"+r.URL.RawQuery, http.StatusSeeOther)
}

// HandleViewEvent displays a single event
func (h *WebHandler) HandleViewEvent(w http.ResponseWriter, r *http.Request) {
	// Get the event ID from the URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	// Convert ID to int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid event ID", http.StatusBadRequest)
		return
	}
	
	// Get the event
	event, err := h.db.GetEventByID(id)
	if err != nil {
		http.Error(w, "Error retrieving event", http.StatusInternalServerError)
		return
	}
	
	if event == nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}
	
	// Prepare template data
	data := TemplateData{
		Event: event,
	}
	
	// Set content type
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Execute template
	templateName := "view.html"
	err = h.templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("Error rendering event view template %s: %v", templateName, err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// displayEventsList is a helper function to show the events list
func (h *WebHandler) displayEventsList(w http.ResponseWriter, r *http.Request, user *auth.User) {
	// Get query parameters for filtering
	tag := r.URL.Query().Get("tag")
	date := r.URL.Query().Get("date")
	source := r.URL.Query().Get("source")
	
	// Get all unique tags from the database
	allTags, err := h.db.GetAllTags()
	if err != nil {
		log.Printf("Error fetching tags: %v", err)
		allTags = []string{} // Use empty list if there's an error
	}
	
	// Get all unique sources from the database
	allSources, err := h.db.GetAllSources()
	if err != nil {
		log.Printf("Error fetching sources: %v", err)
		allSources = []string{} // Use empty list if there's an error
	}
	
	// Fetch events based on filters
	var events []models.Event
	var fetchErr error
	
	log.Printf("Filtering events - Tag: '%s', Date: '%s', Source: '%s'", tag, date, source)
	
	if date != "" {
		// If date filter is provided
		log.Printf("Fetching events by date: %s", date)
		events, fetchErr = h.db.GetEventsByDate(date)
		
		// Apply additional source filter if provided
		if source != "" && fetchErr == nil {
			log.Printf("Additionally filtering %d events by source: %s", len(events), source)
			var filteredEvents []models.Event
			for _, event := range events {
				if event.Source != "" && strings.EqualFold(event.Source, source) {
					filteredEvents = append(filteredEvents, event)
				}
			}
			events = filteredEvents
			log.Printf("After source filtering: %d events remain", len(events))
		}
		
		// Apply additional tag filter if provided
		if tag != "" && fetchErr == nil {
			log.Printf("Additionally filtering %d events by tag: %s", len(events), tag)
			var filteredEvents []models.Event
			for _, event := range events {
				for _, eventTag := range event.Tags {
					if strings.EqualFold(eventTag, tag) {
						filteredEvents = append(filteredEvents, event)
						break
					}
				}
			}
			events = filteredEvents
			log.Printf("After tag filtering: %d events remain", len(events))
		}
	} else if source != "" {
		// If source filter is provided
		log.Printf("Fetching events by source: %s", source)
		events, fetchErr = h.db.GetEventsBySource(source)
		
		// Apply additional tag filter if provided
		if tag != "" && fetchErr == nil {
			log.Printf("Additionally filtering %d events by tag: %s", len(events), tag)
			var filteredEvents []models.Event
			for _, event := range events {
				for _, eventTag := range event.Tags {
					if strings.EqualFold(eventTag, tag) {
						filteredEvents = append(filteredEvents, event)
						break
					}
				}
			}
			events = filteredEvents
			log.Printf("After tag filtering: %d events remain", len(events))
		}
	} else {
		// Default: filter by tag (or get all if tag is empty)
		log.Printf("Fetching events by tag: %s", tag)
		events, fetchErr = h.db.GetEventsByTag(tag)
	}
	
	if fetchErr != nil {
		log.Printf("Error fetching events: %v", fetchErr)
		http.Error(w, "Error fetching events", http.StatusInternalServerError)
		return
	}
	
	log.Printf("Total events found: %d", len(events))
	
	// Prepare template data
	data := TemplateData{
		User:    user,
		Events:  events,
		Tags:    allTags,
		Sources: allSources,
	}
	
	// Set filter info
	data.Filter.Tag = tag
	data.Filter.Date = date
	data.Filter.Source = source
	
	// Set pagination info
	data.Pagination.CurrentPage = 1
	data.Pagination.TotalPages = 1
	data.Pagination.TotalItems = len(events)
	data.Pagination.ItemsPerPage = 20
	
	// Set content type
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Execute the template
	templateName := "list.html"
	err = h.templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("Error rendering events template %s: %v", templateName, err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// HandleCreateEvent displays the event creation form
func (h *WebHandler) HandleCreateEvent(w http.ResponseWriter, r *http.Request) {
	// Get user from context (if authenticated)
	var user *auth.User
	if cookie, err := r.Cookie("session"); err == nil {
		if session, err := h.auth.GetSession(cookie.Value); err == nil {
			user, _ = h.auth.GetUserByID(session.UserID)
		}
	}
	
	data := TemplateData{
		User: user,
	}
	
	// Set content type
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Execute template
	templateName := "new.html"
	err := h.templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("Error rendering event creation template %s: %v", templateName, err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// HandleCreateEventPost handles the event creation form submission
func (h *WebHandler) HandleCreateEventPost(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		h.setFlash(w, "Error processing form data", "error")
		http.Redirect(w, r, "/events/new", http.StatusSeeOther)
		return
	}
	
	// Get form data
	data := r.FormValue("data")
	tagsStr := r.FormValue("tags")
	source := r.FormValue("source")
	
	// Validate required fields
	if data == "" {
		h.setFlash(w, "Event data is required", "error")
		http.Redirect(w, r, "/events/new", http.StatusSeeOther)
		return
	}
	
	// Process tags
	var tags []string
	if tagsStr != "" {
		for _, tag := range strings.Split(tagsStr, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	
	// Create event
	event := models.Event{
		Data:      data,
		Tags:      tags,
		Source:    source,
		CreatedAt: time.Now(),
	}
	
	// Save event to database
	err := h.db.SaveEvent(&event)
	if err != nil {
		h.setFlash(w, fmt.Sprintf("Error creating event: %v", err), "error")
		http.Redirect(w, r, "/events/new", http.StatusSeeOther)
		return
	}
	
	// Set success flash message
	h.setFlash(w, "Event created successfully", "success")
	
	// Redirect to the home page (which shows events when logged in)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleEditEvent displays the event edit form
func (h *WebHandler) HandleEditEvent(w http.ResponseWriter, r *http.Request) {
	// Get the event ID from the URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	// Convert ID to int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid event ID", http.StatusBadRequest)
		return
	}
	
	// Get the event
	event, err := h.db.GetEventByID(id)
	if err != nil {
		http.Error(w, "Error retrieving event", http.StatusInternalServerError)
		return
	}
	
	if event == nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}
	
	// Prepare template data
	data := TemplateData{
		Event: event,
	}
	
	// Set content type
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	// Execute template
	templateName := "edit.html"
	err = h.templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("Error rendering event edit template %s: %v", templateName, err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

// HandleEditEventPost processes the event edit form submission
func (h *WebHandler) HandleEditEventPost(w http.ResponseWriter, r *http.Request) {
	// Get the event ID from the URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	// Convert ID to int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid event ID", http.StatusBadRequest)
		return
	}
	
	// Get the existing event
	event, err := h.db.GetEventByID(id)
	if err != nil {
		http.Error(w, "Error retrieving event", http.StatusInternalServerError)
		return
	}
	
	if event == nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}
	
	// Parse form data
	if err := r.ParseForm(); err != nil {
		h.setFlash(w, "Error processing form data", "error")
		http.Redirect(w, r, fmt.Sprintf("/events/%d/edit", id), http.StatusSeeOther)
		return
	}
	
	// Get form data
	data := r.FormValue("data")
	tagsStr := r.FormValue("tags")
	source := r.FormValue("source")
	
	// Log received form data for debugging
	log.Printf("Edit event form data - ID: %d, Data length: %d, Tags: %s, Source: %s", 
		id, len(data), tagsStr, source)
	
	// Validate required fields
	if data == "" {
		h.setFlash(w, "Event data is required", "error")
		http.Redirect(w, r, fmt.Sprintf("/events/%d/edit", id), http.StatusSeeOther)
		return
	}
	
	// Process tags
	var tags []string
	if tagsStr != "" {
		for _, tag := range strings.Split(tagsStr, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
				log.Printf("Added tag: %s", tag)
			}
		}
	}
	log.Printf("Final processed tags: %v", tags)
	
	// Update event fields
	event.Data = data
	if len(tags) > 0 {
		event.Tags = tags
	}
	event.Source = source
	
	// Ensure we're not saving empty tags array if we had tags before
	if len(event.Tags) == 0 && len(tagsStr) == 0 {
		log.Printf("Warning: No tags provided but event previously had tags. Keeping existing tags.")
		// Get fresh copy of event to ensure we have original tags
		originalEvent, _ := h.db.GetEventByID(id)
		if originalEvent != nil && len(originalEvent.Tags) > 0 {
			event.Tags = originalEvent.Tags
		}
	}
	
	// Save updated event to database
	err = h.db.UpdateEvent(event)
	if err != nil {
		log.Printf("Error updating event: %v", err)
		h.setFlash(w, fmt.Sprintf("Error updating event: %v", err), "error")
		http.Redirect(w, r, fmt.Sprintf("/events/%d/edit", id), http.StatusSeeOther)
		return
	}
	
	// Set success flash message
	h.setFlash(w, "Event updated successfully", "success")
	
	// Redirect to the home page (which shows events when logged in)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleDeleteEvent handles the deletion of an event
func (h *WebHandler) HandleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	// Get the event ID from the URL
	vars := mux.Vars(r)
	idStr := vars["id"]
	
	// Convert ID to int64
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid event ID", http.StatusBadRequest)
		return
	}
	
	// Delete the event
	err = h.db.DeleteEvent(id)
	if err != nil {
		log.Printf("Error deleting event: %v", err)
		h.setFlash(w, fmt.Sprintf("Error deleting event: %v", err), "error")
	} else {
		h.setFlash(w, "Event deleted successfully", "success")
	}
	
	// Redirect to the events list
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleDebug displays template debugging information
func (h *WebHandler) HandleDebug(w http.ResponseWriter, r *http.Request) {
	log.Printf("HandleDebug called with URL: %s", r.URL.String())
	
	// Build debug info
	output := "Template Debug Information\n\n"
	output += "Available Templates:\n"
	
	for _, t := range h.templates.Templates() {
		output += fmt.Sprintf("- %s\n", t.Name())
	}
	
	// Try to identify which template would be used
	output += "\nTemplate Lookup Test:\n"
	for _, name := range []string{"base.html", "login.html", "dashboard.html", "index.html"} {
		tmpl := h.templates.Lookup(name)
		if tmpl != nil {
			output += fmt.Sprintf("- %s: FOUND\n", name)
		} else {
			output += fmt.Sprintf("- %s: NOT FOUND\n", name)
		}
	}
	
	// Current template being used
	output += "\nCurrent route: " + r.URL.Path + "\n"
	
	// Render raw debug info
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, output)
}

// Helper methods
func (h *WebHandler) setFlash(w http.ResponseWriter, message string, messageType string) {
	flashCookie := &http.Cookie{
		Name:     "flash",
		Value:    url.QueryEscape(message + "|" + messageType),
		Path:     "/",
		HttpOnly: true,
		MaxAge:   300, // 5 minutes
	}
	
	http.SetCookie(w, flashCookie)
}

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