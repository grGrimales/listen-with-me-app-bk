package middleware

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const ClaimsKey contextKey = "claims"

func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err != nil || !token.Valid {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, token.Claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminOnly must be chained after Auth.
func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			log.Printf("[DEBUG] AdminOnly: no claims found in context")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rolesRaw := claims["roles"]
		log.Printf("[DEBUG] AdminOnly: roles in token: %v (%T)", rolesRaw, rolesRaw)
		
		roles, _ := rolesRaw.([]interface{})
		for _, role := range roles {
			if role == "admin" {
				next.ServeHTTP(w, r)
				return
			}
		}
		log.Printf("[DEBUG] AdminOnly: 'admin' role not found in %v", roles)
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	})
}
