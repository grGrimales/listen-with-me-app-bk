package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"listen-with-me/backend/internal/model"
	"listen-with-me/backend/internal/repository"
	"listen-with-me/backend/internal/storage"
	"listen-with-me/backend/internal/tts"
)

type TTSHandler struct {
	ttsRepo  *repository.TTSRepo
	stories  *repository.StoryRepo
	storage  storage.FileStorage
	provider tts.Provider
}

func NewTTSHandler(ttsRepo *repository.TTSRepo, stories *repository.StoryRepo, store storage.FileStorage, provider tts.Provider) *TTSHandler {
	return &TTSHandler{
		ttsRepo:  ttsRepo,
		stories:  stories,
		storage:  store,
		provider: provider,
	}
}

// GET /api/tts/voices [admin]
func (h *TTSHandler) ListVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := h.ttsRepo.ListEnabledVoices()
	if err != nil {
		log.Printf("tts list voices error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if voices == nil {
		voices = []model.TTSVoice{}
	}
	jsonOK(w, voices)
}

// GET /api/tts/models [admin]
func (h *TTSHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.provider.ListModels(r.Context())
	if err != nil {
		log.Printf("tts list models error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, models)
}

type generateAudioRequest struct {
	VoiceID string `json:"voice_id"` // UUID from tts_voices table
	ModelID string `json:"model_id"` // provider model ID
}

// POST /api/paragraphs/{id}/audio/generate [admin]
func (h *TTSHandler) GenerateParagraphAudio(w http.ResponseWriter, r *http.Request) {
	paragraphID, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		jsonError(w, "invalid paragraph id", http.StatusBadRequest)
		return
	}

	var req generateAudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.VoiceID == "" || req.ModelID == "" {
		jsonError(w, "voice_id and model_id are required", http.StatusBadRequest)
		return
	}

	voice, err := h.ttsRepo.GetVoiceByID(req.VoiceID)
	if err != nil {
		log.Printf("tts get voice error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if voice == nil || !voice.Enabled {
		jsonError(w, "voice not found", http.StatusNotFound)
		return
	}

	paragraph, err := h.stories.GetParagraphByID(paragraphID)
	if err != nil {
		log.Printf("tts get paragraph error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if paragraph == nil {
		jsonError(w, "paragraph not found", http.StatusNotFound)
		return
	}

	result, err := h.provider.GenerateAudio(r.Context(), paragraph.Content, voice.VoiceID, req.ModelID)
	if err != nil {
		log.Printf("tts generate audio error: %v", err)
		jsonError(w, "audio generation failed", http.StatusBadGateway)
		return
	}

	filename := fmt.Sprintf("audio/tts_%d_%d.mp3", paragraphID, time.Now().UnixNano())
	audioURL, err := h.storage.Upload(r.Context(), filename, bytes.NewReader(result.Data), result.ContentType)
	if err != nil {
		log.Printf("tts upload audio error: %v", err)
		jsonError(w, "upload failed", http.StatusInternalServerError)
		return
	}

	if err := h.ttsRepo.InsertHistory(paragraphID, audioURL, voice.Name, req.ModelID); err != nil {
		log.Printf("tts insert history error: %v", err)
	}

	if err := h.stories.SetParagraphAudio(paragraphID, audioURL); err != nil {
		log.Printf("tts set paragraph audio error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"audio_url": audioURL})
}

// GET /api/paragraphs/{id}/audio/history [admin]
func (h *TTSHandler) ListParagraphAudioHistory(w http.ResponseWriter, r *http.Request) {
	paragraphID, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		jsonError(w, "invalid paragraph id", http.StatusBadRequest)
		return
	}

	entries, err := h.ttsRepo.ListHistory(paragraphID)
	if err != nil {
		log.Printf("tts list history error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []model.TTSHistoryEntry{}
	}
	jsonOK(w, entries)
}

type restoreAudioRequest struct {
	HistoryID string `json:"history_id"`
}

// POST /api/paragraphs/{id}/audio/restore [admin]
func (h *TTSHandler) RestoreParagraphAudio(w http.ResponseWriter, r *http.Request) {
	paragraphID, err := pathID(r, "/api/paragraphs/")
	if err != nil {
		jsonError(w, "invalid paragraph id", http.StatusBadRequest)
		return
	}

	var req restoreAudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.HistoryID == "" {
		jsonError(w, "history_id is required", http.StatusBadRequest)
		return
	}

	entry, err := h.ttsRepo.GetHistoryEntry(req.HistoryID)
	if err != nil {
		log.Printf("tts get history entry error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if entry == nil || entry.ParagraphID != paragraphID {
		jsonError(w, "history entry not found", http.StatusNotFound)
		return
	}

	if err := h.stories.SetParagraphAudio(paragraphID, entry.AudioURL); err != nil {
		log.Printf("tts restore audio error: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"audio_url": entry.AudioURL})
}
