package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

// prodDSN returns the production DATABASE_URL, which lives commented out in .env.
func prodDSN() string {
	b, err := os.ReadFile(".env")
	if err != nil {
		log.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#DATABASE_URL=") {
			continue
		}
		v := strings.TrimPrefix(line, "#DATABASE_URL=")
		// Drop the trailing "#..." note kept after the query string.
		if i := strings.Index(v, "?"); i >= 0 {
			if j := strings.Index(v[i:], "#"); j >= 0 {
				v = v[:i+j]
			}
		}
		return strings.TrimSpace(v)
	}
	return ""
}

// firstKeyword returns the upper-cased first word of the statement, skipping any
// leading "--" comment lines (a .sql file usually opens with a header comment).
func firstKeyword(q string) string {
	for _, line := range strings.Split(q, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		return strings.ToUpper(strings.Fields(line)[0])
	}
	return ""
}

// Ad-hoc query tool: go run ./cmd/dbq "SELECT ..."
// Set DB_TARGET=prod to use the production DSN instead of DATABASE_URL.
func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if os.Getenv("DB_TARGET") == "prod" {
		dsn = prodDSN()
	}
	if dsn == "" {
		log.Fatal("no DSN")
	}
	if len(os.Args) < 2 {
		log.Fatal("usage: dbq <sql>")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	q := os.Args[1]
	// "@path" runs the SQL stored in that file.
	if strings.HasPrefix(q, "@") {
		b, err := os.ReadFile(strings.TrimPrefix(q, "@"))
		if err != nil {
			log.Fatal(err)
		}
		q = string(b)
	}

	// Statements that return no rows go through Exec so we can report the row count.
	switch firstKeyword(q) {
	case "UPDATE", "INSERT", "DELETE":
		res, err := db.Exec(q)
		if err != nil {
			log.Fatal(err)
		}
		n, _ := res.RowsAffected()
		fmt.Fprintf(os.Stderr, "(%d rows affected)\n", n)
		return
	}

	rows, err := db.Query(q)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	enc := json.NewEncoder(os.Stdout)
	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			log.Fatal(err)
		}
		m := map[string]any{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		_ = enc.Encode(m)
		n++
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "(%d rows)\n", n)
}
