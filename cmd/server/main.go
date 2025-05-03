package main

import (
	"example-api/internal/api"
	"example-api/internal/config"
	"example-api/internal/database"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// Configure logging
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.SetPrefix("[example-api] ")

	log.Println("Starting example API server...")

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

	log.Println("Initializing database...")
	db, err := database.NewPostgres(pgConnStr)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Println("Database initialized successfully")

	// Initialize router and handler
	gin.SetMode(gin.ReleaseMode)
	router := gin.New() // Use New() instead of Default() for custom logging

	// Add custom logging middleware
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("[example-api] %s | %d | %s | %s %s | %s\n",
			param.TimeStamp.Format(time.RFC3339),
			param.StatusCode,
			param.Latency,
			param.Method,
			param.Path,
			param.ErrorMessage,
		)
	}))
	router.Use(gin.Recovery())

	handler := api.New(db)

	// Set up routes
	router.POST("/api/events", api.AuthMiddleware(cfg.Server.APIToken), handler.HandleEventReceive)
	router.GET("/api/events/:id", handler.HandleGetEventByID)
	router.GET("/api/events", handler.HandleGetEventsByTag)
	router.GET("/api/events/by-date", handler.HandleGetEventsByDate)

	// Start server
	address := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Server initialization complete. Listening on %s", address)
	if err := router.Run(address); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
