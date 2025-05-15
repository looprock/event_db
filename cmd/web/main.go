package main

import (
	"example-api/internal/auth"
	"example-api/internal/config"
	"example-api/internal/database"
	"example-api/internal/web"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

func main() {
	// Configure logging
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.SetPrefix("[event-web] ")

	log.Println("Starting Event Database Web Interface...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Build PostgreSQL connection string
	pgConnStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	// Initialize database
	log.Println("Initializing database connection...")
	db, err := database.NewPostgres(pgConnStr)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Println("Database connection established successfully")

	// Initialize authentication system
	authSystem := auth.New()
	authSystem.InitializeDefaultUsers()
	log.Println("Authentication system initialized")

	// Create web handler
	webHandler, err := web.NewWebHandler(db, authSystem, cfg.Server.APIToken)
	if err != nil {
		log.Fatalf("Failed to create web handler: %v", err)
	}

	// Initialize router
	router := mux.NewRouter()
	
	// Set up static file server for CSS, JS, and images
	router.PathPrefix("/assets/").Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("public/assets"))))

	// Configure routes
	webHandler.SetupRoutes(router)
	
	// Add a catch-all route for 404s
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "404 - Page not found")
	})

	// Create HTTP server
	webAddr := fmt.Sprintf(":%d", 8082) // Changed port to 8082
	server := &http.Server{
		Addr:         webAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine so that it doesn't block
	go func() {
		log.Printf("Web server listening on %s", webAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Set up graceful shutdown
	quit := make(chan os.Signal, 1)
	// Kill (no param) default sends syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need to add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	
	// Block until we receive a shutdown signal
	sig := <-quit
	log.Printf("Shutting down server... (Signal: %v)", sig)

	log.Println("Server gracefully stopped")
}