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
CREATE TABLE IF NOT EXISTS phrase_playlists (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    language VARCHAR(10) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS phrase_groups (
    id SERIAL PRIMARY KEY,
    phrase_playlist_id INTEGER NOT NULL REFERENCES phrase_playlists(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS phrases (
    id SERIAL PRIMARY KEY,
    phrase_group_id INTEGER NOT NULL REFERENCES phrase_groups(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    translation_es TEXT NOT NULL,
    pronunciation_es TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_phrase_playlists_user_id ON phrase_playlists(user_id);
CREATE INDEX IF NOT EXISTS idx_phrase_groups_playlist_id ON phrase_groups(phrase_playlist_id);
CREATE INDEX IF NOT EXISTS idx_phrases_group_id ON phrases(phrase_group_id);
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 020 completed successfully!")
}
