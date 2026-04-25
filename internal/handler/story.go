package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"listen-with-me/backend/internal/model"
	"listen-with-me/backend/internal/repository"
)

type StoryHandler struct {
	stories *repository.StoryRepo
}

func NewStoryHandler(stories *repository.StoryRepo) *StoryHandler {
	return &StoryHandler{stories: stories}
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
	stories, err := h.stories.List(false)
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
		ImageURL: req.ImageURL,
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
