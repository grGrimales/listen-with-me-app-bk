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
CREATE TABLE IF NOT EXISTS phrase_playlist_vocabulary (
    id SERIAL PRIMARY KEY,
    phrase_playlist_id INTEGER NOT NULL REFERENCES phrase_playlists(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phrase_id INTEGER REFERENCES phrases(id) ON DELETE SET NULL,
    text TEXT NOT NULL,
    translation_es TEXT,
    notes TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ppv_user_playlist ON phrase_playlist_vocabulary(user_id, phrase_playlist_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ppv_user_playlist_text
    ON phrase_playlist_vocabulary(user_id, phrase_playlist_id, lower(text));
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 027 completed successfully!")
}
