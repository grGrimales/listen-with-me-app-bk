-- Word-level timestamps for a phrase's generated audio (cached in polly_audio_url_female).
-- Stored for potential future features (click-to-play / karaoke on phrases). Not used in
-- the UI yet — mirrors paragraph_word_timestamps for stories.
CREATE TABLE IF NOT EXISTS phrase_word_timestamps (
    id         SERIAL PRIMARY KEY,
    phrase_id  INT  NOT NULL REFERENCES phrases(id) ON DELETE CASCADE,
    word_index INT  NOT NULL,
    word       TEXT NOT NULL,
    start_ms   INT  NOT NULL,
    end_ms     INT  NOT NULL,
    UNIQUE (phrase_id, word_index)
);

CREATE INDEX IF NOT EXISTS idx_phrase_word_ts_phrase
    ON phrase_word_timestamps (phrase_id);

-- Provenance / metadata captured from the ElevenLabs generation response.
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS audio_model       TEXT;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS audio_voice_id    TEXT;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS audio_duration_ms INT;
