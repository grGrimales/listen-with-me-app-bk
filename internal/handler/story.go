package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"listen-with-me/backend/internal/middleware"
	"listen-with-me/backend/internal/model"
	"listen-with-me/backend/internal/repository"
	"listen-with-me/backend/internal/storage"
)

type StoryHandler struct {
	stories *repository.StoryRepo
	storage storage.FileStorage
}

func NewStoryHandler(stories *repository.StoryRepo, storage storage.FileStorage) *StoryHandler {
	return &StoryHandler{stories: stories, storage: storage}
}

// GET /api/categories
func (h *StoryHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.stories.ListCategories()
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, cats)
}

// GET /api/stories
func (h *StoryHandler) ListStories(w http.ResponseWriter, r *http.Request) {
	playlistID, _ := strconv.Atoi(r.URL.Query().Get("playlist_id"))
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	stories, err := h.stories.List(false, playlistID, userID)
	if err != nil {
		log.Printf("Error listing stories: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	log.Printf("Found %d stories", len(stories))
	if stories == nil {
		stories = []model.Story{}
	}
	jsonOK(w, stories)
}

// GET /api/stories/deleted [admin]
func (h *StoryHandler) ListDeletedStories(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] ListDeletedStories handler entered")
	stories, err := h.stories.ListDeleted()
	if err != nil {
		log.Printf("[DEBUG] ListDeletedStories ERROR: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	log.Printf("[DEBUG] ListDeletedStories returning %d stories", len(stories))
	if stories == nil {
		stories = []model.Story{}
	}
	jsonOK(w, stories)
}

// POST /api/stories/{id}/restore [admin]
func (h *StoryHandler) RestoreStory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.stories.Restore(id); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "restored"})
}

// GET /api/stories/{id}
func (h *StoryHandler) GetStory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	story, err := h.stories.GetByID(id)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if story == nil {
		jsonError(w, "story not found", http.StatusNotFound)
		return
	}
	jsonOK(w, story)
}

// DELETE /api/stories/{id}  [admin]
func (h *StoryHandler) DeleteStory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.stories.Delete(id); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// PUT /api/stories/{id}  [admin]
func (h *StoryHandler) UpdateStory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req model.CreateFullStoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := h.stories.UpdateFull(id, &req); err != nil {
		log.Printf("Error updating story %d: %v", id, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "updated"})
}

// POST /api/stories/full  [admin]
func (h *StoryHandler) CreateFull(w http.ResponseWriter, r *http.Request) {
	var req model.CreateFullStoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.Level == "" || req.CategoryID == 0 {
		jsonError(w, "title, level and category_id are required", http.StatusBadRequest)
		return
	}
	story, err := h.stories.CreateFull(&req)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(story)
}

// POST /api/stories  [admin]
func (h *StoryHandler) CreateStory(w http.ResponseWriter, r *http.Request) {
	var req model.CreateStoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Title == "" || req.Level == "" || req.CategoryID == 0 {
		jsonError(w, "title, level and category_id are required", http.StatusBadRequest)
		return
	}
	story := &model.Story{
		Title:      req.Title,
		Level:      req.Level,
		CategoryID: req.CategoryID,
		CoverURL:   req.CoverURL,
		Author:     req.Author,
	}
	if err := h.stories.Create(story); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(story)
}

// POST /api/stories/{id}/publish  [admin]
func (h *StoryHandler) PublishStory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.stories.Publish(id); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "published"})
}

// POST /api/stories/{id}/paragraphs  [admin]
func (h *StoryHandler) AddParagraph(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req model.CreateParagraphRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		jsonError(w, "content is required", http.StatusBadRequest)
		return
	}
	p := &model.Paragraph{
		StoryID:  id,
		Position: req.Position,
		Content:  req.Content,
		AudioURL: req.AudioURL,
	}
	for i, url := range req.Images {
		p.Images = append(p.Images, model.ParagraphImage{
			ImageURL: url,
			Position: i,
		})
	}
	if err := h.stories.AddParagraph(p); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

// POST /api/paragraphs/{id}/translations  [admin]
func (h *StoryHandler) AddTranslation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req model.CreateTranslationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Language == "" || req.Content == "" {
		jsonError(w, "language and content are required", http.StatusBadRequest)
		return
	}
	t := &model.ParagraphTranslation{
		ParagraphID: id,
		Language:    req.Language,
		Content:     req.Content,
	}
	if err := h.stories.AddTranslation(t); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(t)
}

// POST /api/paragraphs/{id}/vocabulary  [admin]
func (h *StoryHandler) AddVocabulary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req model.CreateVocabularyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Word == "" || req.Definition == "" {
		jsonError(w, "word and definition are required", http.StatusBadRequest)
		return
	}
	v := &model.Vocabulary{
		ParagraphID: id,
		Word:        req.Word,
		Definition:  req.Definition,
	}
	if err := h.stories.AddVocabulary(v); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(v)
}

// POST /api/stories/{id}/voices  [admin]
func (h *StoryHandler) AddVoice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req model.CreateVoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.AudioURL == "" {
		jsonError(w, "name and audio_url are required", http.StatusBadRequest)
		return
	}
	v := &model.StoryVoice{
		StoryID:    id,
		Name:       req.Name,
		AudioURL:   req.AudioURL,
		Timestamps: req.Timestamps,
	}
	if err := h.stories.AddVoice(v); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(v)
}

// POST /api/paragraphs/{id}/audio/upload  [admin]
func (h *StoryHandler) UploadParagraphAudio(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	const maxSize = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonError(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("audio/%d_%s%s", time.Now().UnixNano(), sanitizeFilename(strings.TrimSuffix(header.Filename, ext)), ext)

	url, err := h.storage.Upload(r.Context(), filename, file, header.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("paragraph audio upload error: %v", err)
		jsonError(w, "upload failed", http.StatusInternalServerError)
		return
	}

	if err := h.stories.SetParagraphAudio(id, url); err != nil {
		log.Printf("set paragraph audio error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"audio_url": url})
}

// DELETE /api/paragraphs/{id}/audio [admin]
func (h *StoryHandler) DeleteParagraphAudio(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	p, err := h.stories.GetParagraphByID(id)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		jsonError(w, "paragraph not found", http.StatusNotFound)
		return
	}

	if p.AudioURL != "" {
		if err := h.storage.Delete(r.Context(), p.AudioURL); err != nil {
			log.Printf("error deleting paragraph audio from storage: %v", err)
			// We continue even if storage delete fails to allow clearing the DB reference
		}
	}

	if err := h.stories.SetParagraphAudio(id, ""); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "audio deleted"})
}

// POST /api/paragraphs/{id}/images/upload [admin]
func (h *StoryHandler) UploadParagraphImage(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] UploadParagraphImage handler entered")
	id, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		log.Printf("[DEBUG] UploadParagraphImage: invalid id: %v", err)
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Check current image count
	p, err := h.stories.GetParagraphByID(id)
	if err != nil {
		log.Printf("[DEBUG] UploadParagraphImage: GetParagraphByID error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if p == nil {
		log.Printf("[DEBUG] UploadParagraphImage: paragraph %d not found", id)
		jsonError(w, "paragraph not found", http.StatusNotFound)
		return
	}
	if len(p.Images) >= 5 {
		log.Printf("[DEBUG] UploadParagraphImage: max images reached for paragraph %d", id)
		jsonError(w, "maximum 5 images per paragraph", http.StatusBadRequest)
		return
	}

	const maxSize = 10 << 20 // 10 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		log.Printf("[DEBUG] UploadParagraphImage: ParseMultipartForm error: %v", err)
		jsonError(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("[DEBUG] UploadParagraphImage: FormFile error: %v", err)
		jsonError(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("images/%d_%s%s", time.Now().UnixNano(), sanitizeFilename(strings.TrimSuffix(header.Filename, ext)), ext)

	log.Printf("[DEBUG] UploadParagraphImage: uploading file %s", filename)
	url, err := h.storage.Upload(r.Context(), filename, file, header.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("[DEBUG] UploadParagraphImage: storage upload error: %v", err)
		jsonError(w, "upload failed", http.StatusInternalServerError)
		return
	}

	img := &model.ParagraphImage{
		ParagraphID: id,
		ImageURL:    url,
		Position:    len(p.Images),
	}
	log.Printf("[DEBUG] UploadParagraphImage: adding image record to DB")
	if err := h.stories.AddParagraphImage(img); err != nil {
		log.Printf("[DEBUG] UploadParagraphImage: AddParagraphImage DB error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUG] UploadParagraphImage: success")
	jsonOK(w, img)
}

// DELETE /api/paragraphs/images/{id} [admin]
func (h *StoryHandler) DeleteParagraphImage(w http.ResponseWriter, r *http.Request) {
	// Note: we use /api/paragraphs/images/ prefix to get the image ID
	id, err := pathID(r, "/api/paragraphs/images/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Optional: we could fetch the image to delete it from storage too.
	// For simplicity, we'll just delete the record for now, 
	// but the architecture supports h.storage.Delete if we had the URL.

	if err := h.stories.DeleteParagraphImage(id); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"status": "image deleted"})
}

// POST /api/stories/{id}/voices/upload  [admin]
func (h *StoryHandler) UploadVoiceAudio(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	const maxSize = 100 << 20 // 100 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonError(w, "file too large or invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("audio/%d_%s%s", time.Now().UnixNano(), sanitizeFilename(strings.TrimSuffix(header.Filename, ext)), ext)

	url, err := h.storage.Upload(r.Context(), filename, file, header.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("audio upload error: %v", err)
		jsonError(w, "upload failed", http.StatusInternalServerError)
		return
	}

	v := &model.StoryVoice{
		StoryID:    id,
		Name:       name,
		AudioURL:   url,
		Timestamps: []model.VoiceTimestamp{},
	}
	if err := h.stories.AddVoice(v); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(v)
}

// helpers

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func pathID(r *http.Request, prefix string) (int, error) {
	raw := strings.TrimPrefix(r.URL.Path, prefix)
	raw = strings.Split(raw, "/")[0]
	return strconv.Atoi(raw)
}

func sanitizeFilename(name string) string {
	var buf []byte
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			buf = append(buf, c)
		} else {
			buf = append(buf, '_')
		}
	}
	if len(buf) == 0 {
		return "audio"
	}
	return string(buf)
}

// POST /api/stories/{id}/review
func (h *StoryHandler) MarkAsReviewed(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.stories.AddReview(userID, id); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "reviewed"})
}

// GET /api/stats
func (h *StoryHandler) GetUserStats(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	stats, err := h.stories.GetUserStats(userID)
	if err != nil {
		log.Printf("Error getting user stats: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, stats)
}

func (h *StoryHandler) userIDFromContext(r *http.Request) (string, error) {
	claims, ok := r.Context().Value(middleware.ClaimsKey).(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("no claims")
	}
	sub, ok := claims["sub"]
	if !ok {
		return "", fmt.Errorf("no sub in claims")
	}
	switch v := sub.(type) {
	case string:
		return v, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// --- Playlists ---

func (h *StoryHandler) ListPlaylists(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	list, err := h.stories.ListPlaylists(userID)
	if err != nil {
		log.Printf("Error listing playlists for user %s: %v", userID, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, list)
}

func (h *StoryHandler) CreatePlaylist(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req model.CreatePlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	p := &model.Playlist{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
	}
	if err := h.stories.CreatePlaylist(p); err != nil {
		if strings.Contains(err.Error(), "unique_user_playlist_name") {
			jsonError(w, "a playlist with this name already exists", http.StatusConflict)
			return
		}
		log.Printf("Error creating playlist: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, p)
}

func (h *StoryHandler) UpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/playlists/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req model.CreatePlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	p := &model.Playlist{
		ID:          id,
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
	}
	if err := h.stories.UpdatePlaylist(p); err != nil {
		if strings.Contains(err.Error(), "unique_user_playlist_name") {
			jsonError(w, "a playlist with this name already exists", http.StatusConflict)
			return
		}
		log.Printf("Error updating playlist %d: %v", id, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "updated"})
}

func (h *StoryHandler) DeletePlaylist(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/playlists/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.stories.DeletePlaylist(id, userID); err != nil {
		log.Printf("Error deleting playlist %d: %v", id, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func (h *StoryHandler) AddStoryToPlaylist(w http.ResponseWriter, r *http.Request) {
	pID, err := pathID(r, "/api/playlists/")
	if err != nil {
		jsonError(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	var req model.AddStoryToPlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := h.stories.AddStoryToPlaylist(pID, req.StoryID); err != nil {
		log.Printf("Error adding story %d to playlist %d: %v", req.StoryID, pID, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "added"})
}

func (h *StoryHandler) RemoveStoryFromPlaylist(w http.ResponseWriter, r *http.Request) {
	// Pattern: /api/playlists/{id}/stories/{storyID}
	pID, err := pathID(r, "/api/playlists/")
	if err != nil {
		jsonError(w, "invalid playlist id", http.StatusBadRequest)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/api/playlists/%d/stories/", pID))
	sID, err := strconv.Atoi(raw)
	if err != nil {
		jsonError(w, "invalid story id", http.StatusBadRequest)
		return
	}
	if err := h.stories.RemoveStoryFromPlaylist(pID, sID); err != nil {
		log.Printf("Error removing story %d from playlist %d: %v", sID, pID, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

// --- User Vocabulary ---

// POST /api/stories/{id}/vocabulary
func (h *StoryHandler) AddUserVocabulary(w http.ResponseWriter, r *http.Request) {
	storyID, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req model.AddUserVocabularyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if req.Phrase == "" {
		jsonError(w, "phrase is required", http.StatusBadRequest)
		return
	}
	v := &model.UserVocabulary{
		UserID:  userID,
		StoryID: storyID,
		Phrase:  req.Phrase,
	}
	if err := h.stories.AddUserVocabulary(v); err != nil {
		log.Printf("AddUserVocabulary error: userID=%s storyID=%d phrase=%q err=%v", userID, storyID, req.Phrase, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, v)
}

// GET /api/stories/{id}/vocabulary
func (h *StoryHandler) ListUserVocabulary(w http.ResponseWriter, r *http.Request) {
	storyID, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	list, err := h.stories.ListUserVocabulary(userID, storyID)
	if err != nil {
		log.Printf("Error listing user vocabulary: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, list)
}

// DELETE /api/stories/vocabulary/{id}
func (h *StoryHandler) DeleteUserVocabulary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/vocabulary/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.stories.DeleteUserVocabulary(id, userID); err != nil {
		log.Printf("Error deleting user vocabulary %d: %v", id, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}
