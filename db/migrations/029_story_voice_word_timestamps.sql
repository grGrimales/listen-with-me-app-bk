-- Word-level timestamps for story voices, enabling click-to-play on a specific word.
-- One row per word per paragraph per voice. Cascades from voice or paragraph deletion.
CREATE TABLE IF NOT EXISTS story_voice_word_timestamps (
    id           SERIAL PRIMARY KEY,
    voice_id     INT  NOT NULL REFERENCES story_voices(id) ON DELETE CASCADE,
    paragraph_id INT  NOT NULL REFERENCES paragraphs(id)   ON DELETE CASCADE,
    word_index   INT  NOT NULL,
    word         TEXT NOT NULL,
    start_ms     INT  NOT NULL,
    end_ms       INT  NOT NULL,
    UNIQUE (voice_id, paragraph_id, word_index)
);

CREATE INDEX IF NOT EXISTS idx_word_ts_voice_para
    ON story_voice_word_timestamps (voice_id, paragraph_id);
