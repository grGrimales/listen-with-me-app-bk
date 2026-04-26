package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	email := "admin@listenwithme.com"
	log.Printf("Setting admin role for: %s", email)

	result, err := db.Exec(`UPDATE users SET roles = '{user, admin}' WHERE email = $1`, email)
	if err != nil {
		log.Fatalf("Update failed: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		log.Printf("User %s not found. Make sure you registered first!", email)
	} else {
		log.Printf("Success! User %s is now an administrator.", email)
	}
}
