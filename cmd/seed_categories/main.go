package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	categories := []struct {
		Name string
		Slug string
	}{
		{"General", "general"},
		{"Sports", "sports"},
		{"Science", "science"},
		{"History", "history"},
		{"Technology", "technology"},
		{"Culture", "culture"},
		{"Politics", "politics"},
		{"Travel", "travel"},
	}

	for _, c := range categories {
		_, err := db.Exec(`INSERT INTO categories (name, slug) VALUES ($1, $2) ON CONFLICT DO NOTHING`, c.Name, c.Slug)
		if err != nil {
			log.Printf("Error inserting category %s: %v", c.Name, err)
		}
	}

	fmt.Println("Categories seeded successfully!")
}
