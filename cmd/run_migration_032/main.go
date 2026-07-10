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
DELETE FROM phrase_playlists WHERE story_id IS NOT NULL;
ALTER TABLE phrase_playlists DROP COLUMN IF EXISTS story_id;
ALTER TABLE phrase_playlists
    ADD COLUMN IF NOT EXISTS story_playlist_id INT REFERENCES playlists(id) ON DELETE CASCADE;
CREATE UNIQUE INDEX IF NOT EXISTS ux_storyplaylist_phrase_playlist
    ON phrase_playlists(user_id, story_playlist_id)
    WHERE story_playlist_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_phrase_playlists_story_playlist
    ON phrase_playlists(story_playlist_id);
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_story_id INT REFERENCES stories(id) ON DELETE SET NULL;
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 032 completed successfully!")
}
