package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents a user in the system
type User struct {
	ID           int
	Username     string
	PasswordHash string
	Role         string // "admin" or "user"
	CreatedAt    time.Time
}

// Session represents a user session
type Session struct {
	ID        string
	UserID    int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Auth handles authentication for the application
type Auth struct {
	users    map[string]*User
	sessions map[string]*Session
	mu       sync.RWMutex
}

// New creates a new Auth instance
func New() *Auth {
	return &Auth{
		users:    make(map[string]*User),
		sessions: make(map[string]*Session),
	}
}

// CreateUser creates a new user
func (a *Auth) CreateUser(username, password, role string) (*User, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if user already exists
	if _, exists := a.users[username]; exists {
		return nil, errors.New("user already exists")
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Create the user
	user := &User{
		ID:           len(a.users) + 1,
		Username:     username,
		PasswordHash: string(hashedPassword),
		Role:         role,
		CreatedAt:    time.Now(),
	}

	a.users[username] = user
	return user, nil
}

// Authenticate checks if the username and password are valid
func (a *Auth) Authenticate(username, password string) (*User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	user, exists := a.users[username]
	if !exists {
		return nil, errors.New("invalid username or password")
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid username or password")
	}

	return user, nil
}

// CreateSession creates a new session for a user
func (a *Auth) CreateSession(userID int) (*Session, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Generate random session ID
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	sessionID := base64.StdEncoding.EncodeToString(b)

	// Create session
	session := &Session{
		ID:        sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24 hour sessions
	}

	a.sessions[sessionID] = session
	return session, nil
}

// GetSession retrieves a session by ID
func (a *Auth) GetSession(sessionID string) (*Session, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, errors.New("session not found")
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		delete(a.sessions, sessionID)
		return nil, errors.New("session expired")
	}

	return session, nil
}

// GetUserByID retrieves a user by ID
func (a *Auth) GetUserByID(userID int) (*User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, user := range a.users {
		if user.ID == userID {
			return user, nil
		}
	}

	return nil, errors.New("user not found")
}

// DeleteSession removes a session
func (a *Auth) DeleteSession(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.sessions, sessionID)
}

// SetSessionCookie sets a session cookie on the response
func SetSessionCookie(w http.ResponseWriter, session *Session) {
	cookie := &http.Cookie{
		Name:     "session",
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Allow HTTP access
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie clears the session cookie
func ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Allow HTTP access
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(-1 * time.Hour),
		MaxAge:   -1,
	}
	http.SetCookie(w, cookie)
}

// InitializeDefaultUsers creates default users for the system
func (a *Auth) InitializeDefaultUsers() {
	// Check if admin user exists already
	a.mu.RLock()
	_, adminExists := a.users["admin"]
	a.mu.RUnlock()

	if !adminExists {
		a.CreateUser("admin", "admin123", "admin")
	}
}

// RequireAuth is middleware that checks if a user is authenticated
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Auth middleware checking request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		
		cookie, err := r.Cookie("session")
		if err != nil {
			log.Printf("No session cookie found: %v - redirecting to login", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Printf("Found session cookie: %s", cookie.Value)

		session, err := a.GetSession(cookie.Value)
		if err != nil {
			log.Printf("Invalid session: %v - clearing cookie and redirecting", err)
			ClearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Printf("Valid session found for user ID: %d", session.UserID)

		user, err := a.GetUserByID(session.UserID)
		if err != nil {
			log.Printf("User not found for session: %v - clearing cookie and redirecting", err)
			ClearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		log.Printf("User authenticated: %s (ID: %d), proceeding to handler", user.Username, user.ID)

		// Store user in request context
		ctx := SetUserInContext(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin is middleware that checks if a user is an admin
func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUserFromContext(r.Context())
		if user == nil || user.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}