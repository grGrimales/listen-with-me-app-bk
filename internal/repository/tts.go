package repository

import (
	"database/sql"

	"listen-with-me/backend/internal/model"
)

type TTSRepo struct {
	db *sql.DB
}

func NewTTSRepo(db *sql.DB) *TTSRepo {
	return &TTSRepo{db: db}
}

func (r *TTSRepo) ListEnabledVoices() ([]model.TTSVoice, error) {
	rows, err := r.db.Query(`
		SELECT id, provider, voice_id, name, description, COALESCE(language,'en'), enabled, created_at, updated_at
		FROM tts_voices
		WHERE enabled = TRUE
		ORDER BY language, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var voices []model.TTSVoice
	for rows.Next() {
		var v model.TTSVoice
		if err := rows.Scan(&v.ID, &v.Provider, &v.VoiceID, &v.Name, &v.Description, &v.Language, &v.Enabled, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		voices = append(voices, v)
	}
	return voices, rows.Err()
}

func (r *TTSRepo) GetVoiceByID(id string) (*model.TTSVoice, error) {
	var v model.TTSVoice
	err := r.db.QueryRow(`
		SELECT id, provider, voice_id, name, description, COALESCE(language,'en'), enabled, created_at, updated_at
		FROM tts_voices WHERE id = $1
	`, id).Scan(&v.ID, &v.Provider, &v.VoiceID, &v.Name, &v.Description, &v.Language, &v.Enabled, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *TTSRepo) InsertHistory(paragraphID int, audioURL, voiceName, modelID string) error {
	_, err := r.db.Exec(`
		INSERT INTO paragraph_tts_history (paragraph_id, audio_url, voice_name, model_id)
		VALUES ($1, $2, $3, $4)
	`, paragraphID, audioURL, voiceName, modelID)
	return err
}

func (r *TTSRepo) ListHistory(paragraphID int) ([]model.TTSHistoryEntry, error) {
	rows, err := r.db.Query(`
		SELECT id, paragraph_id, audio_url, voice_name, model_id, created_at
		FROM paragraph_tts_history
		WHERE paragraph_id = $1
		ORDER BY created_at DESC
	`, paragraphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.TTSHistoryEntry
	for rows.Next() {
		var e model.TTSHistoryEntry
		if err := rows.Scan(&e.ID, &e.ParagraphID, &e.AudioURL, &e.VoiceName, &e.ModelID, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *TTSRepo) GetHistoryEntry(id string) (*model.TTSHistoryEntry, error) {
	var e model.TTSHistoryEntry
	err := r.db.QueryRow(`
		SELECT id, paragraph_id, audio_url, voice_name, model_id, created_at
		FROM paragraph_tts_history WHERE id = $1
	`, id).Scan(&e.ID, &e.ParagraphID, &e.AudioURL, &e.VoiceName, &e.ModelID, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
