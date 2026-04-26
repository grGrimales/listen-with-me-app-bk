package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	log.Println("Adding position column to paragraph_images...")
	_, err = db.Exec("ALTER TABLE paragraph_images ADD COLUMN position INTEGER NOT NULL DEFAULT 0")
	if err != nil {
		log.Printf("Warning (maybe column already exists): %v", err)
	} else {
		log.Println("Column added successfully!")
	}
}
