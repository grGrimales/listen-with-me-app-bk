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
ALTER TABLE phrase_playlists
    ADD COLUMN IF NOT EXISTS story_id INT REFERENCES stories(id) ON DELETE CASCADE;
CREATE UNIQUE INDEX IF NOT EXISTS ux_story_phrase_playlist
    ON phrase_playlists(user_id, story_id)
    WHERE story_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_phrase_playlists_story
    ON phrase_playlists(story_id);
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_audio_url TEXT;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_start_ms  INT;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_end_ms    INT;
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 031 completed successfully!")
}
