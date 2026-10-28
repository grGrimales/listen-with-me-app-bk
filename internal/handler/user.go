package handler

import (
	"encoding/json"
	"log"
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

// GET /api/admin/users — admin only. Every user with their roles, for the share page.
func (h *UserHandler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.ListAll()
	if err != nil {
		log.Printf("AdminListUsers: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, users)
}

// PUT /api/admin/users/{id}/shared-role  body: {enabled} — admin only.
// The "shared" role limits the user to the /compartidos view; it applies on their next login.
func (h *UserHandler) AdminSetSharedRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	roles, ok, err := h.users.SetRole(r.PathValue("id"), "shared", body.Enabled)
	if err != nil {
		log.Printf("AdminSetSharedRole: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{"roles": roles})
}
