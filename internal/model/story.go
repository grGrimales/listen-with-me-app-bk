package model

import (
	"encoding/json"
	"time"
)

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Story struct {
	ID         int          `json:"id"`
	Title      string       `json:"title"`
	Level      string       `json:"level"`
	CategoryID int          `json:"category_id"`
	Category   *Category    `json:"category,omitempty"`
	CoverURL   string       `json:"cover_url"`
	Author     string       `json:"author"`
	Status     string       `json:"status"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	Paragraphs []Paragraph  `json:"paragraphs,omitempty"`
	Voices     []StoryVoice `json:"voices,omitempty"`
}

type Paragraph struct {
	ID           int                    `json:"id"`
	StoryID      int                    `json:"story_id"`
	Position     int                    `json:"position"`
	Content      string                 `json:"content"`
	Images       []ParagraphImage       `json:"images"`
	AudioURL     string                 `json:"audio_url"`
	Translations []ParagraphTranslation `json:"translations,omitempty"`
	Vocabulary   []Vocabulary           `json:"vocabulary,omitempty"`
}

type ParagraphImage struct {
	ID          int    `json:"id"`
	ParagraphID int    `json:"paragraph_id"`
	ImageURL    string `json:"image_url"`
	Position    int    `json:"position"`
}

type ParagraphTranslation struct {
	ID          int    `json:"id"`
	ParagraphID int    `json:"paragraph_id"`
	Language    string `json:"language"`
	Content     string `json:"content"`
}

type Vocabulary struct {
	ID          int    `json:"id"`
	ParagraphID int    `json:"paragraph_id"`
	Word        string `json:"word"`
	Definition  string `json:"definition"`
}

type VoiceTimestamp struct {
	ParagraphID int `json:"paragraph_id"`
	StartMs     int `json:"start_ms"`
	EndMs       int `json:"end_ms"`
}

type StoryVoice struct {
	ID         int              `json:"id"`
	StoryID    int              `json:"story_id"`
	Name       string           `json:"name"`
	AudioURL   string           `json:"audio_url"`
	Timestamps []VoiceTimestamp `json:"timestamps"`
}

type UserProgress struct {
	ID          int        `json:"id"`
	UserID      string     `json:"user_id"`
	StoryID     int        `json:"story_id"`
	VoiceID     *int       `json:"voice_id"`
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completed_at"`
}

// --- Request bodies ---

type CreateStoryRequest struct {
	Title      string `json:"title"`
	Level      string `json:"level"`
	CategoryID int    `json:"category_id"`
	CoverURL   string `json:"cover_url"`
	Author     string `json:"author"`
}

type CreateParagraphRequest struct {
	Position int      `json:"position"`
	Content  string   `json:"content"`
	Images   []string `json:"images"`
	AudioURL string   `json:"audio_url"`
}

type CreateTranslationRequest struct {
	Language string `json:"language"`
	Content  string `json:"content"`
}

type CreateVocabularyRequest struct {
	Word       string `json:"word"`
	Definition string `json:"definition"`
}

type CreateVoiceRequest struct {
	Name       string           `json:"name"`
	AudioURL   string           `json:"audio_url"`
	Timestamps []VoiceTimestamp `json:"timestamps"`
}

// --- Full story creation (single request) ---

type FullParagraph struct {
	Position     int                        `json:"position"`
	Content      string                     `json:"content"`
	Images       []string                   `json:"images"`
	AudioURL     string                     `json:"audio_url"`
	Translations []CreateTranslationRequest `json:"translations"`
	Vocabulary   []CreateVocabularyRequest  `json:"vocabulary"`
}

type CreateFullStoryRequest struct {
	Title      string          `json:"title"`
	Level      string          `json:"level"`
	CategoryID int             `json:"category_id"`
	CoverURL   string          `json:"cover_url"`
	Author     string          `json:"author"`
	Paragraphs []FullParagraph `json:"paragraphs"`
	Voices     []CreateVoiceRequest `json:"voices"`
}

// MarshalTimestamps serializes timestamps to JSON for DB storage.
func (v *StoryVoice) MarshalTimestamps() ([]byte, error) {
	if v.Timestamps == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(v.Timestamps)
}

type StoryReview struct {
	ID         int       `json:"id"`
	UserID     string    `json:"user_id"`
	StoryID    int       `json:"story_id"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

type UserVocabulary struct {
	ID        int       `json:"id"`
	UserID    string    `json:"user_id"`
	StoryID   int       `json:"story_id"`
	Phrase    string    `json:"phrase"`
	CreatedAt time.Time `json:"created_at"`
}

type AddUserVocabularyRequest struct {
	Phrase string `json:"phrase"`
}

type UserStats struct {
	TotalReviews    int            `json:"total_reviews"`
	DailyReviews    []StatPeriod   `json:"daily_reviews"`
	MonthlyReviews  []StatPeriod   `json:"monthly_reviews"`
	YearlyReviews   []StatPeriod   `json:"yearly_reviews"`
	HistorySummary  []StorySummary `json:"history_summary"`
}

type StatPeriod struct {
	Period string `json:"period"` // e.g. "2026-04-26", "2026-04", "2026"
	Count  int    `json:"count"`
}

type StorySummary struct {
	StoryID      int       `json:"story_id"`
	Title        string    `json:"title"`
	ReviewCount  int       `json:"review_count"`
	LastReviewed time.Time `json:"last_reviewed"`
}

type Playlist struct {
	ID          int       `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	StoryCount  int       `json:"story_count"`
}

type CreatePlaylistRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AddStoryToPlaylistRequest struct {
	StoryID int `json:"story_id"`
}
