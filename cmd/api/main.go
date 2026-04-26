package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"listen-with-me/backend/internal/handler"
	"listen-with-me/backend/internal/middleware"
	"listen-with-me/backend/internal/repository"
	"listen-with-me/backend/internal/storage"
)

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET not set")
	}
	if len(jwtSecret) < 32 {
		log.Println("WARNING: JWT_SECRET is too short. Use at least 32 characters for better security.")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("failed to open database:", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("failed to connect to database:", err)
	}
	log.Println("connected to database")

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	serverBaseURL := os.Getenv("SERVER_BASE_URL")
	if serverBaseURL == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8082"
		}
		serverBaseURL = "http://localhost:" + port
	}
	audioStorage, err := storage.NewLocalStorage(uploadDir, serverBaseURL)
	if err != nil {
		log.Fatal("failed to initialize audio storage:", err)
	}
	log.Printf("audio storage: local dir=%s base=%s", uploadDir, serverBaseURL)

	userRepo := repository.NewUserRepo(db)
	storyRepo := repository.NewStoryRepo(db)

	authH := handler.NewAuthHandler(userRepo)
	storyH := handler.NewStoryHandler(storyRepo, audioStorage)

	mux := http.NewServeMux()

	// Static — uploaded audio files
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir("./uploads"))))

	// Public
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /api/auth/register", authH.Register)
	mux.HandleFunc("POST /api/auth/login", authH.Login)

	// Admin helper
	admin := func(h http.HandlerFunc) http.Handler {
		return middleware.Auth(middleware.AdminOnly(h))
	}

	// Authenticated — read
	mux.Handle("GET /api/admin/stories/trash-items", admin(http.HandlerFunc(storyH.ListDeletedStories)))
	mux.Handle("GET /api/categories", middleware.Auth(http.HandlerFunc(storyH.ListCategories)))
	mux.Handle("GET /api/stories", middleware.Auth(http.HandlerFunc(storyH.ListStories)))
	mux.Handle("GET /api/stories/", middleware.Auth(http.HandlerFunc(storyH.GetStory)))
	mux.Handle("PUT /api/stories/", admin(storyH.UpdateStory))
	mux.Handle("DELETE /api/stories/", admin(storyH.DeleteStory))

	// Admin — write
	mux.Handle("POST /api/stories", admin(storyH.CreateStory))
	mux.Handle("POST /api/stories/full", admin(storyH.CreateFull))
	mux.Handle("POST /api/stories/", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case hasSegment(r.URL.Path, "voices") && hasSegment(r.URL.Path, "upload"):
			storyH.UploadVoiceAudio(w, r)
		case hasSegment(r.URL.Path, "paragraphs"):
			storyH.AddParagraph(w, r)
		case hasSegment(r.URL.Path, "voices"):
			storyH.AddVoice(w, r)
		case hasSegment(r.URL.Path, "publish"):
			storyH.PublishStory(w, r)
		case hasSegment(r.URL.Path, "restore"):
			storyH.RestoreStory(w, r)
		default:
			http.NotFound(w, r)
		}
	})))
	mux.Handle("POST /api/paragraphs/", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[DEBUG] POST /api/paragraphs/ hit: %s", r.URL.Path)
		switch {
		case hasSegment(r.URL.Path, "audio") && hasSegment(r.URL.Path, "upload"):
			storyH.UploadParagraphAudio(w, r)
		case hasSegment(r.URL.Path, "images") && hasSegment(r.URL.Path, "upload"):
			storyH.UploadParagraphImage(w, r)
		case hasSegment(r.URL.Path, "translations"):
			storyH.AddTranslation(w, r)
		case hasSegment(r.URL.Path, "vocabulary"):
			storyH.AddVocabulary(w, r)
		default:
			http.NotFound(w, r)
		}
	})))

	mux.Handle("DELETE /api/paragraphs/", admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case hasSegment(r.URL.Path, "audio"):
			storyH.DeleteParagraphAudio(w, r)
		case hasSegment(r.URL.Path, "images"):
			storyH.DeleteParagraphImage(w, r)
		default:
			http.NotFound(w, r)
		}
	})))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	h := recoveryMiddleware(securityMiddleware(corsMiddleware(mux)))

	log.Printf("server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, h); err != nil {
		log.Fatal(err)
	}
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[REQ] %s %s", r.Method, r.URL.Path)
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[PANIC] %v", err)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func hasSegment(path, segment string) bool {
	for _, s := range splitPath(path) {
		if s == segment {
			return true
		}
	}
	return false
}

func splitPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '/' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	return parts
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[CORS] %s %s", r.Method, r.URL.Path)
		origin := os.Getenv("ALLOWED_ORIGIN")
		if origin == "" {
			origin = "http://localhost:5173"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
