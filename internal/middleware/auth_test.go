package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func tokenWithRoles(t *testing.T, roles ...string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "00000000-0000-0000-0000-000000000000", "roles": roles,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAuthSharedRole(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	ok := Auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	cases := []struct {
		roles        []string
		method, path string
		want         int
	}{
		{[]string{"user", "shared"}, "GET", "/api/phrase-playlists", 200},
		{[]string{"user", "shared"}, "GET", "/api/phrase-playlists/28", 200},
		{[]string{"user", "shared"}, "GET", "/api/phrase-playlists/28/evaluation-stats", 200},
		{[]string{"user", "shared"}, "POST", "/api/phrases/5/review", 200},
		{[]string{"user", "shared"}, "POST", "/api/phrases/5/evaluate", 200},
		{[]string{"user", "shared"}, "POST", "/api/phrases/5/audio/polly", 200},
		{[]string{"user", "shared"}, "GET", "/api/search", 200},
		{[]string{"user", "shared"}, "GET", "/api/stories", 403},
		{[]string{"user", "shared"}, "GET", "/api/playlists", 403},
		{[]string{"user", "shared"}, "GET", "/api/story-phrase-playlists", 403},
		{[]string{"user", "shared"}, "GET", "/api/phrase-stats/me/detailed", 200},
		{[]string{"user", "shared"}, "GET", "/api/phrase-zen-stats/me/detailed", 200},
		{[]string{"user", "shared"}, "GET", "/api/phrase-stats/leaderboard", 403},
		{[]string{"user", "shared"}, "GET", "/api/phrase-zen-stats/leaderboard", 403},
		{[]string{"user", "shared"}, "GET", "/api/stats", 403},
		{[]string{"user", "shared"}, "PATCH", "/api/phrase-playlists/28", 403},
		{[]string{"user", "shared"}, "POST", "/api/phrase-playlists/28/vocabulary", 403},
		{[]string{"user", "shared"}, "POST", "/api/phrase-playlists/28/shares", 403},
		{[]string{"user", "shared"}, "PUT", "/api/phrases/5", 403},
		{[]string{"user", "shared"}, "GET", "/api/phrase-playlists/28/shares", 403},
		// Regular users and admins are not affected.
		{[]string{"user"}, "GET", "/api/stories", 200},
		{[]string{"user", "admin", "shared"}, "GET", "/api/stories", 200},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.Header.Set("Authorization", "Bearer "+tokenWithRoles(t, c.roles...))
		rec := httptest.NewRecorder()
		ok.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%v %s %s = %d, want %d", c.roles, c.method, c.path, rec.Code, c.want)
		}
	}
}
