package main

import (
	"log"
	"net/http"
	"os"

	"listen-with-me/backend/internal/server"
)

func main() {
	h := server.Setup()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	log.Printf("server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, h); err != nil {
		log.Fatal(err)
	}
}
