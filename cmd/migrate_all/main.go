package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"

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

	files, err := filepath.Glob(filepath.Join("db", "migrations", "*.sql"))
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(files)

	for _, file := range files {
		log.Printf("Executing migration: %s", file)
		content, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("failed to read migration file %s: %v", file, err)
		}

		_, err = db.Exec(string(content))
		if err != nil {
			log.Fatalf("migration failed for %s: %v", file, err)
		}
	}

	log.Println("All migrations completed successfully!")
}
