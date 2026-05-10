package server

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib"

	"listen-with-me/backend/internal/handler"
	"listen-with-me/backend/internal/middleware"
	"listen-with-me/backend/internal/repository"
	"listen-with-me/backend/internal/storage"
	"listen-with-me/backend/internal/tts/elevenlabs"
	"listen-with-me/backend/internal/gemini"
)

var (
	handlerInstance http.Handler
	initOnce        sync.Once
)

func Setup() http.Handler {
	initOnce.Do(func() {
		_ = godotenv.Load()

		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("DATABASE_URL not set")
		}

		jwtSecret := os.Getenv("JWT_SECRET")
		if jwtSecret == "" {
			log.Fatal("JWT_SECRET not set")
		}

		db, err := sql.Open("pgx", dsn)
		if err != nil {
			log.Fatal("failed to open database:", err)
		}

		if err := db.Ping(); err != nil {
			log.Printf("Warning: initial ping failed: %v", err)
		}

		uploadDir := os.Getenv("UPLOAD_DIR")
		if uploadDir == "" {
			uploadDir = "./uploads"
		}
		
		var audioStorage storage.FileStorage
		cloudinaryURL := os.Getenv("CLOUDINARY_URL")
		
		if cloudinaryURL != "" {
			var err error
			audioStorage, err = storage.NewCloudinaryStorage(cloudinaryURL)
			if err != nil {
				log.Fatal("failed to initialize cloudinary storage:", err)
			}
		} else {
			serverBaseURL := os.Getenv("SERVER_BASE_URL")
			if serverBaseURL == "" {
				serverBaseURL = "http://localhost:8082"
			}
			var err error
			audioStorage, err = storage.NewLocalStorage(uploadDir, serverBaseURL)
			if err != nil {
				log.Fatal("failed to initialize local storage:", err)
			}
		}

		userRepo := repository.NewUserRepo(db)
		storyRepo := repository.NewStoryRepo(db)
		ttsRepo := repository.NewTTSRepo(db)

		elevenlabsAPIKey := os.Getenv("ELEVENLABS_API_KEY")
		ttsProvider := elevenlabs.New(elevenlabsAPIKey)

		geminiAPIKey := os.Getenv("GEMINI_API_KEY")
		geminiClient := gemini.NewClient(geminiAPIKey)

		authH := handler.NewAuthHandler(userRepo)
		storyH := handler.NewStoryHandler(storyRepo, audioStorage, geminiClient)
		ttsH := handler.NewTTSHandler(ttsRepo, storyRepo, audioStorage, ttsProvider)
		userH := handler.NewUserHandler(userRepo)

		mux := http.NewServeMux()

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

		// Authenticated
		mux.Handle("GET /api/stats", middleware.Auth(http.HandlerFunc(storyH.GetUserStats)))
		mux.Handle("GET /api/playlists", middleware.Auth(http.HandlerFunc(storyH.ListPlaylists)))
		mux.Handle("GET /api/admin/stories/trash-items", admin(http.HandlerFunc(storyH.ListDeletedStories)))
		mux.Handle("GET /api/categories", middleware.Auth(http.HandlerFunc(storyH.ListCategories)))
		mux.Handle("GET /api/stories", middleware.Auth(http.HandlerFunc(storyH.ListStories)))
		
		// Story CRUD and sub-resources
		mux.Handle("POST /api/stories", admin(storyH.CreateStory))
		mux.Handle("POST /api/stories/full", admin(storyH.CreateFull))
		mux.Handle("GET /api/stories/{id}", middleware.Auth(http.HandlerFunc(storyH.GetStory)))
		mux.Handle("PUT /api/stories/{id}", admin(storyH.UpdateStory))
		mux.Handle("DELETE /api/stories/{id}", admin(storyH.DeleteStory))
		
		mux.Handle("POST /api/stories/{id}/publish", admin(storyH.PublishStory))
		mux.Handle("POST /api/stories/{id}/restore", admin(storyH.RestoreStory))
		mux.Handle("POST /api/stories/{id}/review", middleware.Auth(http.HandlerFunc(storyH.MarkAsReviewed)))
		
		// Sentences
		mux.Handle("POST /api/stories/{id}/sentences/generate", admin(storyH.GenerateSentences))
		mux.Handle("POST /api/stories/{id}/sentences/preview", admin(storyH.PreviewSentences))
		mux.Handle("POST /api/stories/{id}/sentences", admin(storyH.SaveSentences))
		mux.Handle("GET /api/stories/{id}/sentences", middleware.Auth(http.HandlerFunc(storyH.ListSentences)))
		mux.Handle("GET /api/stories/{id}/sentences/stats", middleware.Auth(http.HandlerFunc(storyH.GetStorySentenceStats)))
		
		// Vocabulary
		mux.Handle("POST /api/stories/{id}/vocabulary", middleware.Auth(http.HandlerFunc(storyH.AddUserVocabulary)))
		mux.Handle("GET /api/stories/{id}/vocabulary", middleware.Auth(http.HandlerFunc(storyH.ListUserVocabulary)))
		mux.Handle("DELETE /api/stories/vocabulary/{id}", middleware.Auth(http.HandlerFunc(storyH.DeleteUserVocabulary)))
		
		// Voices
		mux.Handle("POST /api/stories/{id}/voices", admin(storyH.AddVoice))
		mux.Handle("POST /api/stories/{id}/voices/upload", admin(storyH.UploadVoiceAudio))
		
		// Paragraphs
		mux.Handle("POST /api/stories/{id}/paragraphs", admin(storyH.AddParagraph))
		mux.Handle("POST /api/paragraphs/{id}/translations", admin(storyH.AddTranslation))
		mux.Handle("POST /api/paragraphs/{id}/vocabulary", admin(storyH.AddVocabulary))
		mux.Handle("POST /api/paragraphs/{id}/audio/upload", admin(storyH.UploadParagraphAudio))
		mux.Handle("POST /api/paragraphs/{id}/audio/generate", admin(http.HandlerFunc(ttsH.GenerateParagraphAudio)))
		mux.Handle("POST /api/paragraphs/{id}/audio/restore", admin(http.HandlerFunc(ttsH.RestoreParagraphAudio)))
		mux.Handle("POST /api/paragraphs/{id}/images/upload", admin(storyH.UploadParagraphImage))
		mux.Handle("GET /api/paragraphs/{id}/audio/history", admin(http.HandlerFunc(ttsH.ListParagraphAudioHistory)))
		mux.Handle("DELETE /api/paragraphs/{id}/audio", admin(storyH.DeleteParagraphAudio))
		mux.Handle("DELETE /api/paragraph-images/{id}", admin(storyH.DeleteParagraphImage))

		// Playlists
		mux.Handle("POST /api/playlists", middleware.Auth(http.HandlerFunc(storyH.CreatePlaylist)))
		mux.Handle("PUT /api/playlists/{id}", middleware.Auth(http.HandlerFunc(storyH.UpdatePlaylist)))
		mux.Handle("DELETE /api/playlists/{id}", middleware.Auth(http.HandlerFunc(storyH.DeletePlaylist)))
		mux.Handle("PATCH /api/playlists/{id}", middleware.Auth(http.HandlerFunc(storyH.SetPlaylistFavorite)))
		mux.Handle("POST /api/playlists/{id}/stories", middleware.Auth(http.HandlerFunc(storyH.AddStoryToPlaylist)))
		mux.Handle("DELETE /api/playlists/{id}/stories/{storyID}", middleware.Auth(http.HandlerFunc(storyH.RemoveStoryFromPlaylist)))

		mux.Handle("POST /api/sentences/{id}/evaluate", middleware.Auth(http.HandlerFunc(storyH.EvaluateSentence)))
		mux.Handle("GET /api/sentences/{id}/history", middleware.Auth(http.HandlerFunc(storyH.GetSentenceHistory)))

		// Zen Mode
		mux.Handle("GET /api/zen/stories", middleware.Auth(http.HandlerFunc(storyH.ListZenStories)))
		mux.Handle("POST /api/zen/listen", middleware.Auth(http.HandlerFunc(storyH.LogZenListen)))

		// User preferences
		mux.Handle("PUT /api/user/language", middleware.Auth(http.HandlerFunc(userH.UpdateLanguage)))

		// TTS configuration (admin)
		mux.Handle("GET /api/tts/voices", admin(http.HandlerFunc(ttsH.ListVoices)))
		mux.Handle("GET /api/tts/models", admin(http.HandlerFunc(ttsH.ListModels)))

		handlerInstance = recoveryMiddleware(securityMiddleware(corsMiddleware(mux)))
	})
	return handlerInstance
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		origin := os.Getenv("ALLOWED_ORIGIN")
		if origin == "" {
			origin = "http://localhost:5173"
		}
		// Remove trailing slash if present
		if len(origin) > 0 && origin[len(origin)-1] == '/' {
			origin = origin[:len(origin)-1]
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
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
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
