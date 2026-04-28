package model

import "time"

type TTSVoice struct {
	ID          string    `json:"id"`
	Provider    string    `json:"provider"`
	VoiceID     string    `json:"voice_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TTSHistoryEntry struct {
	ID          string    `json:"id"`
	ParagraphID int       `json:"paragraph_id"`
	AudioURL    string    `json:"audio_url"`
	VoiceName   string    `json:"voice_name"`
	ModelID     string    `json:"model_id"`
	CreatedAt   time.Time `json:"created_at"`
}
