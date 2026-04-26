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

	migrationPath := filepath.Join("db", "migrations", "009_playlist_unique_name.sql")
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		log.Fatalf("failed to read migration file: %v", err)
	}

	log.Printf("Executing migration: %s", migrationPath)
	_, err = db.Exec(string(content))
	if err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	log.Println("Migration completed successfully!")
}
