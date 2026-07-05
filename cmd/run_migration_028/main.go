package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib"
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
DROP TABLE IF EXISTS phrase_playlist_vocabulary;
ALTER TABLE phrase_playlists
    ADD COLUMN IF NOT EXISTS parent_playlist_id INTEGER
        REFERENCES phrase_playlists(id) ON DELETE CASCADE;
CREATE UNIQUE INDEX IF NOT EXISTS ux_vocab_child_per_user_parent
    ON phrase_playlists(user_id, parent_playlist_id)
    WHERE parent_playlist_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_phrase_playlists_parent
    ON phrase_playlists(parent_playlist_id);
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 028 completed successfully!")
}
