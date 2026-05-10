package handler

import (
	"encoding/json"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"listen-with-me/backend/internal/middleware"
	"listen-with-me/backend/internal/repository"
)

var validLanguages = map[string]bool{"en": true, "pt": true}

type UserHandler struct {
	users *repository.UserRepo
}

func NewUserHandler(users *repository.UserRepo) *UserHandler {
	return &UserHandler{users: users}
}

// PUT /api/user/language
func (h *UserHandler) UpdateLanguage(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(middleware.ClaimsKey).(jwt.MapClaims)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, _ := claims["sub"].(string)

	var req struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if !validLanguages[req.Language] {
		jsonError(w, "unsupported language", http.StatusBadRequest)
		return
	}

	if err := h.users.UpdateLanguage(userID, req.Language); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"targetLanguage": req.Language})
}
