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
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'phrases' AND column_name = 'polly_audio_url') THEN
    ALTER TABLE phrases RENAME COLUMN polly_audio_url TO polly_audio_url_female;
  END IF;
END $$;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS polly_audio_url_female TEXT;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS polly_audio_url_male TEXT;
`
	if _, err := db.Exec(migration); err != nil {
		log.Fatal("migration failed:", err)
	}
	log.Println("Migration 026 completed successfully!")
}
