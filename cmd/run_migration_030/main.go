package main

import (
	"database/sql"
	"log"
	"os"

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

	migration := `
CREATE TABLE IF NOT EXISTS paragraph_word_timestamps (
    id           SERIAL PRIMARY KEY,
    paragraph_id INT  NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    word_index   INT  NOT NULL,
    word         TEXT NOT NULL,
    start_ms     INT  NOT NULL,
    end_ms       INT  NOT NULL,
    UNIQUE (paragraph_id, word_index)
);

CREATE INDEX IF NOT EXISTS idx_para_word_ts_paragraph
    ON paragraph_word_timestamps (paragraph_id);
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 030 completed successfully!")
}
