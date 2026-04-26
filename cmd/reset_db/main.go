package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("failed to open database:", err)
	}
	defer db.Close()

	log.Println("Wiping database schema...")
	_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	if err != nil {
		log.Fatalf("wipe failed: %v", err)
	}

	schemaPath := filepath.Join("db", "schema_consolidated.sql")
	content, err := os.ReadFile(schemaPath)
	if err != nil {
		log.Fatalf("failed to read schema file: %v", err)
	}

	log.Printf("Executing consolidated schema: %s", schemaPath)
	_, err = db.Exec(string(content))
	if err != nil {
		log.Fatalf("schema execution failed: %v", err)
	}

	log.Println("Database reset and schema set up successfully!")
}
