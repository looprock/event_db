package main

import (
	"database/sql"
	"example-api/internal/config"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	_ "github.com/lib/pq"
)

func main() {
	log.SetPrefix("[example-api] ")
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	pgConnStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	log.Printf("Running migrations on database: %s", cfg.Database.Name)

	// Open database connection
	db, err := sql.Open("postgres", pgConnStr)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Get all migration files
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		log.Fatalf("Failed to list migration files: %v", err)
	}

	// Sort migration files to ensure they run in order
	sort.Strings(files)

	// Execute each migration
	for _, file := range files {
		log.Printf("Running migration: %s", file)

		// Read migration file
		migration, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("Failed to read migration file %s: %v", file, err)
		}

		// Execute migration
		if _, err := db.Exec(string(migration)); err != nil {
			log.Fatalf("Failed to execute migration %s: %v", file, err)
		}
	}

	log.Println("All migrations completed successfully")
}
