package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	email := "admin@listenwithme.com"
	password := os.Getenv("ADMIN_SEED_PASSWORD")
	if password == "" {
		password = "change_me_immediately"
	}
	fullName := "Admin User"

	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	var id string
	err = db.QueryRow(
		`INSERT INTO users ("fullName", email, password, roles, "isActive") 
		 VALUES ($1, $2, $3, $4, $5) 
		 ON CONFLICT (email) DO UPDATE SET roles = $4
		 RETURNING id`,
		fullName, email, string(hash), "{user,admin}", true,
	).Scan(&id)

	if err != nil {
		log.Fatal("Error creating admin:", err)
	}

	fmt.Printf("Admin user created/updated successfully!\nEmail: %s\nPassword: %s\nID: %s\n", email, password, id)
}
