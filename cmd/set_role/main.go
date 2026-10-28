package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// Adds or removes a role for a user, e.g. the "levels" role that limits the app to
// the /niveles view:
//
//	go run ./cmd/set_role -email someone@mail.com -role levels
//	go run ./cmd/set_role -email someone@mail.com -role levels -remove
//
// The user has to log in again for the new role to reach their token.
func main() {
	email := flag.String("email", "", "user email")
	role := flag.String("role", "", "role to add or remove")
	remove := flag.Bool("remove", false, "remove the role instead of adding it")
	flag.Parse()
	if *email == "" || *role == "" {
		log.Fatal("usage: set_role -email <email> -role <role> [-remove]")
	}

	_ = godotenv.Load()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	q := `UPDATE users SET roles = array_append(array_remove(roles, $2), $2) WHERE email = $1 RETURNING roles`
	if *remove {
		q = `UPDATE users SET roles = array_remove(roles, $2) WHERE email = $1 RETURNING roles`
	}
	var roles string
	if err := db.QueryRow(q, *email, *role).Scan(&roles); err == sql.ErrNoRows {
		log.Fatalf("user %s not found", *email)
	} else if err != nil {
		log.Fatalf("update failed: %v", err)
	}
	log.Printf("%s roles: %s", *email, roles)
}
