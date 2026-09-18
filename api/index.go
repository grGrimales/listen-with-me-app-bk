package handler

import (
	"net/http"
	"listen-with-me/backend/internal/server"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	h := server.Setup()
	h.ServeHTTP(w, r)
}
