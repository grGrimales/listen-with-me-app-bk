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
			log.Printf("[DEBUG] Auth: missing or invalid Authorization header prefix")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				log.Printf("[DEBUG] Auth: unexpected signing method: %v", t.Header["alg"])
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		
		if err != nil {
			log.Printf("[DEBUG] Auth: token parse error: %v", err)
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		
		if !token.Valid {
			log.Printf("[DEBUG] Auth: token is invalid")
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Users with the "shared" role only get the /compartidos view: review, zen,
		// search and evaluation of the phrase playlists shared with them. Checked here so no route is left
		// open by forgetting to wrap it.
		if claims, ok := token.Claims.(jwt.MapClaims); ok &&
			hasRole(claims, "shared") && !hasRole(claims, "admin") &&
			!sharedAllowed(r.Method, r.URL.Path) {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
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
		if hasRole(claims, "admin") {
			next.ServeHTTP(w, r)
			return
		}
		log.Printf("[DEBUG] AdminOnly: 'admin' role not found in %v", claims["roles"])
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	})
}

// SharedOnly reports whether the request comes from a "shared" (non-admin) user, who
// only sees the playlists shared with them, not ones they own. Must run after Auth.
func SharedOnly(r *http.Request) bool {
	claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
	return ok && hasRole(claims, "shared") && !hasRole(claims, "admin")
}

func hasRole(claims jwt.MapClaims, name string) bool {
	roles, _ := claims["roles"].([]interface{})
	for _, role := range roles {
		if role == name {
			return true
		}
	}
	return false
}

// sharedRoutes is what a "shared" user may call; "*" matches one path segment.
var sharedRoutes = []struct{ method, path string }{
	{"GET", "/api/phrase-playlists"},
	{"GET", "/api/phrase-playlists/*"},
	{"GET", "/api/phrase-playlists/*/evaluation-stats"},
	{"GET", "/api/phrase-playlists/*/vocabulary"},
	{"POST", "/api/phrases/*/review"},
	{"POST", "/api/phrases/*/zen-listen"},
	{"POST", "/api/phrases/*/rate"},
	{"POST", "/api/phrases/*/evaluate"},
	{"POST", "/api/phrases/*/audio/generate"},
	{"POST", "/api/phrases/*/audio/polly"},
	{"GET", "/api/phrases/*/evaluation-history"},
	{"GET", "/api/phrase-stats/me"},
	{"GET", "/api/phrase-stats/me/detailed"},
	{"GET", "/api/phrase-zen-stats/me/detailed"},
	{"GET", "/api/search/sources"},
	{"GET", "/api/search"},
	{"PUT", "/api/user/language"},
}

func sharedAllowed(method, path string) bool {
	got := strings.Split(strings.TrimSuffix(path, "/"), "/")
	for _, rt := range sharedRoutes {
		if rt.method != method {
			continue
		}
		want := strings.Split(rt.path, "/")
		if len(want) != len(got) {
			continue
		}
		match := true
		for i := range want {
			if want[i] != "*" && want[i] != got[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
