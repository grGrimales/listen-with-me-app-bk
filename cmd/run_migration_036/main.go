package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	content, err := os.ReadFile(filepath.Join("db", "migrations", "036_phrase_word_timestamps.sql"))
	if err != nil {
		log.Fatalf("read migration: %v", err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 036 completed successfully!")
}
