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
CREATE TABLE IF NOT EXISTS phrase_playlist_shares (
    playlist_id INTEGER NOT NULL REFERENCES phrase_playlists(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission VARCHAR(20) NOT NULL CHECK (permission IN ('read', 'editor')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (playlist_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_phrase_playlist_shares_user ON phrase_playlist_shares(user_id);
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 024 completed successfully!")
}
