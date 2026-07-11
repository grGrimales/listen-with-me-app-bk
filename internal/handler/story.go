package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"listen-with-me/backend/internal/gemini"
	"listen-with-me/backend/internal/middleware"
	"listen-with-me/backend/internal/model"
	"listen-with-me/backend/internal/repository"
	"listen-with-me/backend/internal/storage"
	"listen-with-me/backend/internal/tts"
	"listen-with-me/backend/internal/tts/elevenlabs"
)

type StoryHandler struct {
	stories    *repository.StoryRepo
	storage    storage.FileStorage
	gemini     *gemini.GeminiClient
	vocabTTS   tts.Provider
	elevenlabs *elevenlabs.Provider
	ttsRepo    *repository.TTSRepo
}

func NewStoryHandler(stories *repository.StoryRepo, storage storage.FileStorage, gemini *gemini.GeminiClient) *StoryHandler {
	return &StoryHandler{stories: stories, storage: storage, gemini: gemini}
}

func (h *StoryHandler) WithVocabTTS(p tts.Provider) *StoryHandler {
	h.vocabTTS = p
	return h
}

func (h *StoryHandler) WithElevenLabs(p *elevenlabs.Provider) *StoryHandler {
	h.elevenlabs = p
	return h
}

func (h *StoryHandler) WithTTSRepo(r *repository.TTSRepo) *StoryHandler {
	h.ttsRepo = r
	return h
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
	q := r.URL.Query()
	playlistID, _ := strconv.Atoi(q.Get("playlist_id"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	sortBy := q.Get("sort_by")
	if limit <= 0 {
		limit = 12
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	stories, hasMore, err := h.stories.List(false, playlistID, userID, sortBy, limit, offset)
	if err != nil {
		log.Printf("Error listing stories: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if stories == nil {
		stories = []model.Story{}
	}
	jsonOK(w, map[string]interface{}{"stories": stories, "has_more": hasMore})
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
	if lang := r.URL.Query().Get("lang"); lang != "" && lang != "en" {
		applyTargetLanguage(story, lang)
	}
	jsonOK(w, story)
}

func applyTargetLanguage(story *model.Story, lang string) {
	for i := range story.Paragraphs {
		p := &story.Paragraphs[i]
		for _, t := range p.Translations {
			if t.Language == lang {
				p.Content = t.Content
				if t.AudioURL != "" {
					p.AudioURL = t.AudioURL
				}
				break
			}
		}
	}
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

// DELETE /api/paragraph-images/{id} [admin]
func (h *StoryHandler) DeleteParagraphImage(w http.ResponseWriter, r *http.Request) {
	// Note: we use /api/paragraph-images/ prefix to get the image ID
	id, err := pathID(r, "/api/paragraph-images/")
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

type generateElevenLabsVoiceRequest struct {
	Name    string `json:"name"`     // optional; defaults to the tts_voices name
	VoiceID string `json:"voice_id"` // UUID from tts_voices (resolved to the raw provider id)
	ModelID string `json:"model_id"`
}

// paragraphSeparator joins paragraphs when sending to ElevenLabs. Two newlines create
// a clear pause and give us predictable characters to skip when mapping alignment back.
const paragraphSeparator = "\n\n"

// POST /api/stories/{id}/voices/generate-elevenlabs  [admin]
// Generates a full-story voice via ElevenLabs `with-timestamps`, saving audio + paragraph
// and per-word timestamps in a single call. Only used for new voices; existing voices
// (uploaded manually) remain untouched.
func (h *StoryHandler) GenerateElevenLabsVoice(w http.ResponseWriter, r *http.Request) {
	if h.elevenlabs == nil {
		jsonError(w, "ElevenLabs is not configured on the server", http.StatusServiceUnavailable)
		return
	}
	storyID, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid story id", http.StatusBadRequest)
		return
	}

	var req generateElevenLabsVoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.VoiceID == "" || req.ModelID == "" {
		jsonError(w, "voice_id and model_id are required", http.StatusBadRequest)
		return
	}
	if h.ttsRepo == nil {
		jsonError(w, "TTS voices are not configured on the server", http.StatusServiceUnavailable)
		return
	}

	voice, err := h.ttsRepo.GetVoiceByID(req.VoiceID)
	if err != nil {
		log.Printf("generate voice get tts voice error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if voice == nil || !voice.Enabled {
		jsonError(w, "voice not found", http.StatusNotFound)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = voice.Name
	}

	story, err := h.stories.GetByID(storyID)
	if err != nil {
		log.Printf("generate voice get story error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if story == nil {
		jsonError(w, "story not found", http.StatusNotFound)
		return
	}
	if len(story.Paragraphs) == 0 {
		jsonError(w, "story has no paragraphs to narrate", http.StatusBadRequest)
		return
	}

	// Build the input text and remember where each paragraph starts (in rune count),
	// since the ElevenLabs alignment array is 1 entry per character.
	var texts []string
	for _, p := range story.Paragraphs {
		texts = append(texts, p.Content)
	}
	fullText := strings.Join(texts, paragraphSeparator)

	result, err := h.elevenlabs.GenerateAudioWithTimestamps(r.Context(), fullText, voice.VoiceID, req.ModelID)
	if err != nil {
		log.Printf("elevenlabs with-timestamps error: %v", err)
		jsonError(w, "audio generation failed", http.StatusBadGateway)
		return
	}

	paraTS, wordTS := mapAlignmentToParagraphs(story.Paragraphs, result.Words, texts)

	filename := fmt.Sprintf("audio/voice_%d_%d.mp3", storyID, time.Now().UnixNano())
	audioURL, err := h.storage.Upload(r.Context(), filename, bytes.NewReader(result.Data), result.ContentType)
	if err != nil {
		log.Printf("upload voice audio error: %v", err)
		jsonError(w, "upload failed", http.StatusInternalServerError)
		return
	}

	savedVoice, err := h.stories.SaveVoiceWithTimestamps(storyID, name, audioURL, paraTS, wordTS)
	if err != nil {
		log.Printf("save voice error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(savedVoice)
}

// mapAlignmentToParagraphs walks the flat list of word-level timings ElevenLabs returned
// and buckets them per paragraph based on the words in each paragraph's source text.
// Also produces paragraph-level start/end timestamps (min/max of its words).
func mapAlignmentToParagraphs(paras []model.Paragraph, words []elevenlabs.AlignedWord, paraTexts []string) ([]model.VoiceTimestamp, []model.WordTimestamp) {
	var paraTS []model.VoiceTimestamp
	var wordTS []model.WordTimestamp
	if len(words) == 0 {
		return paraTS, wordTS
	}

	cursor := 0 // index into words
	for i, p := range paras {
		expectedWords := strings.Fields(paraTexts[i]) // count of words expected in this paragraph
		if len(expectedWords) == 0 {
			continue
		}

		take := len(expectedWords)
		if cursor+take > len(words) {
			take = len(words) - cursor
		}
		if take <= 0 {
			break
		}

		startMs := words[cursor].StartMs
		endMs := words[cursor+take-1].EndMs
		paraTS = append(paraTS, model.VoiceTimestamp{
			ParagraphID: p.ID,
			StartMs:     startMs,
			EndMs:       endMs,
		})

		for j := 0; j < take; j++ {
			wordTS = append(wordTS, model.WordTimestamp{
				ParagraphID: p.ID,
				WordIndex:   j,
				Word:        words[cursor+j].Word,
				StartMs:     words[cursor+j].StartMs,
				EndMs:       words[cursor+j].EndMs,
			})
		}
		cursor += take
	}
	return paraTS, wordTS
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
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

func (h *StoryHandler) SetPlaylistFavorite(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		IsFavorite bool `json:"is_favorite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	// Owners and users the playlist is shared with can each keep their own favorite.
	access, err := h.stories.HasPlaylistAccess(userID, id)
	if err != nil {
		log.Printf("Error checking playlist access %d: %v", id, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !access {
		jsonError(w, "playlist not found or no access", http.StatusForbidden)
		return
	}
	if err := h.stories.SetPlaylistFavorite(id, userID, req.IsFavorite); err != nil {
		log.Printf("Error setting favorite for playlist %d: %v", id, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"is_favorite": req.IsFavorite})
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

// --- Story playlist sharing ---

// requirePlaylistOwner writes an error response and returns false if the current
// user is not the owner of the playlist.
func (h *StoryHandler) requirePlaylistOwner(w http.ResponseWriter, playlistID int, userID string) bool {
	ownerID, err := h.stories.PlaylistOwner(playlistID)
	if err == sql.ErrNoRows {
		jsonError(w, "not found", http.StatusNotFound)
		return false
	}
	if err != nil {
		log.Printf("requirePlaylistOwner: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return false
	}
	if ownerID != userID {
		jsonError(w, "only the owner can perform this action", http.StatusForbidden)
		return false
	}
	return true
}

// GET /api/playlists/{id}/share-candidates?q=...
func (h *StoryHandler) PlaylistShareCandidates(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.requirePlaylistOwner(w, id, userID) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		jsonOK(w, []model.ShareCandidate{})
		return
	}
	items, err := h.stories.SearchPlaylistShareCandidates(id, userID, "%"+strings.ToLower(q)+"%")
	if err != nil {
		log.Printf("PlaylistShareCandidates: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, items)
}

// GET /api/playlists/{id}/shares
func (h *StoryHandler) ListPlaylistShares(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.requirePlaylistOwner(w, id, userID) {
		return
	}
	items, err := h.stories.ListPlaylistShares(id)
	if err != nil {
		log.Printf("ListPlaylistShares: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, items)
}

// POST /api/playlists/{id}/shares  body: {email, permission}
func (h *StoryHandler) AddPlaylistShare(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.requirePlaylistOwner(w, id, userID) {
		return
	}
	var body struct {
		Email      string `json:"email"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}
	if body.Permission != "read" && body.Permission != "editor" {
		jsonError(w, "permission must be 'read' or 'editor'", http.StatusBadRequest)
		return
	}
	targetID, err := h.stories.UserIDByEmail(body.Email)
	if err == sql.ErrNoRows {
		jsonError(w, "no user with that email", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("AddPlaylistShare user lookup: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if targetID == userID {
		jsonError(w, "you already own this playlist", http.StatusBadRequest)
		return
	}
	if err := h.stories.AddPlaylistShare(id, targetID, body.Permission); err != nil {
		log.Printf("AddPlaylistShare upsert: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	share, err := h.stories.GetPlaylistShare(id, targetID)
	if err != nil {
		log.Printf("AddPlaylistShare read-back: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, share)
}

// PATCH /api/playlists/{id}/shares/{userID}  body: {permission}
func (h *StoryHandler) UpdatePlaylistShare(w http.ResponseWriter, r *http.Request) {
	ownerID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	targetID := r.PathValue("userID")
	if targetID == "" {
		jsonError(w, "user id required", http.StatusBadRequest)
		return
	}
	if !h.requirePlaylistOwner(w, id, ownerID) {
		return
	}
	var body struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Permission != "read" && body.Permission != "editor" {
		jsonError(w, "permission must be 'read' or 'editor'", http.StatusBadRequest)
		return
	}
	n, err := h.stories.UpdatePlaylistShare(id, targetID, body.Permission)
	if err != nil {
		log.Printf("UpdatePlaylistShare: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if n == 0 {
		jsonError(w, "share not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{"permission": body.Permission})
}

// DELETE /api/playlists/{id}/shares/{userID}
func (h *StoryHandler) RemovePlaylistShare(w http.ResponseWriter, r *http.Request) {
	ownerID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	targetID := r.PathValue("userID")
	if targetID == "" {
		jsonError(w, "user id required", http.StatusBadRequest)
		return
	}
	if !h.requirePlaylistOwner(w, id, ownerID) {
		return
	}
	if err := h.stories.RemovePlaylistShare(id, targetID); err != nil {
		log.Printf("RemovePlaylistShare: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
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
	lang := req.Language
	if lang == "" {
		lang = "en"
	}

	// Always save the reader vocabulary (highlight / click-to-play).
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

	// Add the word to the word playlist of every story playlist that contains this
	// story. Its audio is the story's own segment (nil when the paragraph has no audio yet).
	seg, err := h.stories.FindStorySegment(storyID, req.Phrase)
	if err != nil {
		log.Printf("AddUserVocabulary segment lookup error: %v", err)
	}
	playlists, err := h.stories.PlaylistsContainingStory(userID, storyID)
	if err != nil {
		log.Printf("AddUserVocabulary playlists lookup error: %v", err)
	}
	addedTo := 0
	for _, pl := range playlists {
		if err := h.stories.UpsertStoryPlaylistPhrase(userID, pl.ID, pl.Name, lang, req.Phrase, storyID, seg); err != nil {
			log.Printf("AddUserVocabulary upsert story phrase (playlist %d): %v", pl.ID, err)
			continue
		}
		addedTo++
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"id":                 v.ID,
		"phrase":             v.Phrase,
		"story_id":           v.StoryID,
		"position":           v.Position,
		"audio_url":          v.AudioURL,
		"created_at":         v.CreatedAt,
		"in_story_playlists": len(playlists),
		"added_to_playlists": addedTo,
		"audio_missing":      seg == nil,
	})
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

// PATCH /api/stories/{id}/vocabulary/reorder
func (h *StoryHandler) ReorderUserVocabulary(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := h.stories.ReorderUserVocabulary(userID, storyID, req.IDs); err != nil {
		log.Printf("ReorderUserVocabulary error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
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
	// Look up the word first so we can also remove it from the story word playlists.
	vocab, _ := h.stories.GetUserVocabByID(id, userID)

	if err := h.stories.DeleteUserVocabulary(id, userID); err != nil {
		log.Printf("Error deleting user vocabulary %d: %v", id, err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also drop it from the user's story-linked word playlists (stats are discarded).
	if vocab != nil {
		if err := h.stories.RemoveStoryPlaylistPhrases(userID, vocab.StoryID, vocab.Phrase); err != nil {
			log.Printf("DeleteUserVocabulary remove story phrase: %v", err)
		}
	}

	jsonOK(w, map[string]string{"status": "deleted"})
}

// vocabVoiceID maps target language to Google Neural2 voice.
var vocabVoiceID = map[string]string{
	"en": "en-US-Neural2-C",
	"pt": "pt-BR-Neural2-A",
}

// POST /api/stories/vocabulary/{id}/audio?lang=en
func (h *StoryHandler) GenerateVocabAudio(w http.ResponseWriter, r *http.Request) {
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

	vocab, err := h.stories.GetUserVocabByID(id, userID)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	// Return cached audio immediately
	if vocab.AudioURL != "" {
		jsonOK(w, map[string]string{"audio_url": vocab.AudioURL})
		return
	}

	if h.vocabTTS == nil {
		jsonError(w, "TTS not configured", http.StatusServiceUnavailable)
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	voiceID, ok := vocabVoiceID[lang]
	if !ok {
		voiceID = vocabVoiceID["en"]
	}

	result, err := h.vocabTTS.GenerateAudio(r.Context(), vocab.Phrase, voiceID, "")
	if err != nil {
		log.Printf("GenerateVocabAudio TTS error: %v", err)
		jsonError(w, "tts error", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("audio/vocab_%d_%s.mp3", id, lang)
	audioURL, err := h.storage.Upload(r.Context(), filename, bytes.NewReader(result.Data), result.ContentType)
	if err != nil {
		log.Printf("GenerateVocabAudio upload error: %v", err)
		jsonError(w, "upload error", http.StatusInternalServerError)
		return
	}

	if err := h.stories.UpdateVocabAudioURL(id, userID, audioURL); err != nil {
		log.Printf("GenerateVocabAudio DB update error: %v", err)
	}

	jsonOK(w, map[string]string{"audio_url": audioURL})
}

// --- Zen Mode ---

// GET /api/zen/stories
func (h *StoryHandler) ListZenStories(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	playlistID, _ := strconv.Atoi(r.URL.Query().Get("playlist_id"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "random"
	}

	stories, err := h.stories.ListZen(userID, playlistID, limit, sort)
	if err != nil {
		log.Printf("Error listing zen stories: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if stories == nil {
		stories = []model.Story{}
	}
	jsonOK(w, stories)
}

// POST /api/zen/listen
func (h *StoryHandler) LogZenListen(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		StoryID int `json:"story_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.StoryID == 0 {
		jsonError(w, "story_id is required", http.StatusBadRequest)
		return
	}
	if err := h.stories.LogZenListen(userID, req.StoryID); err != nil {
		log.Printf("Error logging zen listen: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "logged"})
}

// --- Sentences & Evaluation ---

// POST /api/stories/{id}/sentences/preview [admin]
func (h *StoryHandler) PreviewSentences(w http.ResponseWriter, r *http.Request) {
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

	var fullContent strings.Builder
	for _, p := range story.Paragraphs {
		fullContent.WriteString(p.Content)
		fullContent.WriteString("\n\n")
	}

	geminiSentences, err := h.gemini.SplitStory(fullContent.String())
	if err != nil {
		log.Printf("Gemini SplitStory error: %v", err)
		jsonError(w, "failed to generate sentences: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, geminiSentences)
}

// POST /api/stories/{id}/sentences [admin]
func (h *StoryHandler) SaveSentences(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var reqSentences []struct {
		En string `json:"en"`
		Es string `json:"es"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqSentences); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	var sentences []model.StorySentence
	for i, s := range reqSentences {
		sentences = append(sentences, model.StorySentence{
			StoryID:  id,
			En:       s.En,
			Es:       s.Es,
			Position: i,
		})
	}

	if err := h.stories.AddSentences(sentences); err != nil {
		log.Printf("AddSentences error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, sentences)
}

// POST /api/stories/{id}/sentences/generate [admin]
func (h *StoryHandler) GenerateSentences(w http.ResponseWriter, r *http.Request) {
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

	var fullContent strings.Builder
	for _, p := range story.Paragraphs {
		fullContent.WriteString(p.Content)
		fullContent.WriteString("\n\n")
	}

	geminiSentences, err := h.gemini.SplitStory(fullContent.String())
	if err != nil {
		log.Printf("Gemini SplitStory error: %v", err)
		jsonError(w, "failed to generate sentences: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var sentences []model.StorySentence
	for i, s := range geminiSentences {
		sentences = append(sentences, model.StorySentence{
			StoryID:  id,
			En:       s.En,
			Es:       s.Es,
			Position: i,
		})
	}

	if err := h.stories.AddSentences(sentences); err != nil {
		log.Printf("AddSentences error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, sentences)
}

// GET /api/stories/{id}/sentences
func (h *StoryHandler) ListSentences(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/stories/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	list, err := h.stories.ListSentences(id)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []model.StorySentence{}
	}
	jsonOK(w, list)
}

// POST /api/sentences/{id}/evaluate
func (h *StoryHandler) EvaluateSentence(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/sentences/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req model.EvaluateSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	attempt := &model.UserSentenceAttempt{
		UserID:     userID,
		SentenceID: id,
		IsCorrect:  req.IsCorrect,
		UserAnswer: req.UserAnswer,
	}

	if err := h.stories.AddSentenceAttempt(attempt); err != nil {
		log.Printf("AddSentenceAttempt error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, attempt)
}

// GET /api/stories/{id}/sentences/stats
func (h *StoryHandler) GetStorySentenceStats(w http.ResponseWriter, r *http.Request) {
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

	stats, err := h.stories.GetSentenceStats(userID, id)
	if err != nil {
		log.Printf("GetSentenceStats error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if stats == nil {
		stats = []model.SentenceStats{}
	}
	jsonOK(w, stats)
}

// GET /api/sentences/{id}/history
func (h *StoryHandler) GetSentenceHistory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "/api/sentences/")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	history, err := h.stories.GetSentenceHistory(userID, id)
	if err != nil {
		log.Printf("GetSentenceHistory error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if history == nil {
		history = []model.UserSentenceAttempt{}
	}
	jsonOK(w, history)
}
