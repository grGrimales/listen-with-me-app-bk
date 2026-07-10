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
	"listen-with-me/backend/internal/tts/elevenlabs"
)

type TTSHandler struct {
	ttsRepo    *repository.TTSRepo
	stories    *repository.StoryRepo
	storage    storage.FileStorage
	provider   tts.Provider
	elevenlabs *elevenlabs.Provider
}

func NewTTSHandler(ttsRepo *repository.TTSRepo, stories *repository.StoryRepo, store storage.FileStorage, provider tts.Provider) *TTSHandler {
	return &TTSHandler{
		ttsRepo:  ttsRepo,
		stories:  stories,
		storage:  store,
		provider: provider,
	}
}

// WithElevenLabs enables word-level timestamps on paragraph audio generation.
func (h *TTSHandler) WithElevenLabs(p *elevenlabs.Provider) *TTSHandler {
	h.elevenlabs = p
	return h
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
	VoiceID  string `json:"voice_id"` // UUID from tts_voices table
	ModelID  string `json:"model_id"` // provider model ID
	Language string `json:"language"` // "en" (default) or "pt"
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
	if req.Language == "" {
		req.Language = "en"
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

	var textToGenerate string
	if req.Language == "en" {
		textToGenerate = paragraph.Content
	} else {
		trans, err := h.stories.GetTranslationByLang(paragraphID, req.Language)
		if err != nil {
			log.Printf("tts get translation error: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if trans == nil {
			jsonError(w, "no translation found for language: "+req.Language, http.StatusNotFound)
			return
		}
		textToGenerate = trans.Content
	}

	// For the main (English) paragraph audio we request word-level timestamps so the
	// reader's click-to-play works. Translations keep the plain generation.
	var audioData []byte
	var contentType string
	var words []model.WordTimestamp

	if req.Language == "en" && h.elevenlabs != nil {
		res, err := h.elevenlabs.GenerateAudioWithTimestamps(r.Context(), textToGenerate, voice.VoiceID, req.ModelID)
		if err != nil {
			log.Printf("tts generate audio (with-timestamps) error: %v", err)
			jsonError(w, "audio generation failed", http.StatusBadGateway)
			return
		}
		audioData = res.Data
		contentType = res.ContentType
		for i, aw := range res.Words {
			words = append(words, model.WordTimestamp{
				ParagraphID: paragraphID,
				WordIndex:   i,
				Word:        aw.Word,
				StartMs:     aw.StartMs,
				EndMs:       aw.EndMs,
			})
		}
	} else {
		res, err := h.provider.GenerateAudio(r.Context(), textToGenerate, voice.VoiceID, req.ModelID)
		if err != nil {
			log.Printf("tts generate audio error: %v", err)
			jsonError(w, "audio generation failed", http.StatusBadGateway)
			return
		}
		audioData = res.Data
		contentType = res.ContentType
	}

	filename := fmt.Sprintf("audio/tts_%d_%s_%d.mp3", paragraphID, req.Language, time.Now().UnixNano())
	audioURL, err := h.storage.Upload(r.Context(), filename, bytes.NewReader(audioData), contentType)
	if err != nil {
		log.Printf("tts upload audio error: %v", err)
		jsonError(w, "upload failed", http.StatusInternalServerError)
		return
	}

	if err := h.ttsRepo.InsertHistory(paragraphID, audioURL, voice.Name, req.ModelID); err != nil {
		log.Printf("tts insert history error: %v", err)
	}

	if req.Language == "en" {
		if err := h.stories.SetParagraphAudio(paragraphID, audioURL); err != nil {
			log.Printf("tts set paragraph audio error: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Replace the paragraph's word timestamps (empty slice clears stale ones).
		if err := h.stories.SaveParagraphWordTimestamps(paragraphID, words); err != nil {
			log.Printf("tts save word timestamps error: %v", err)
		}
	} else {
		if err := h.stories.SetTranslationAudio(paragraphID, req.Language, audioURL); err != nil {
			log.Printf("tts set translation audio error: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	jsonOK(w, map[string]any{"audio_url": audioURL, "word_count": len(words)})
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
