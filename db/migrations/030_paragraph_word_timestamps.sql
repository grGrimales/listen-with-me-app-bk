-- Word-level timestamps for a paragraph's own audio (paragraphs.audio_url).
-- Enables click-to-play per paragraph: click a word to jump that paragraph's audio.
-- One set of words per paragraph; regenerating audio replaces them.
CREATE TABLE IF NOT EXISTS paragraph_word_timestamps (
    id           SERIAL PRIMARY KEY,
    paragraph_id INT  NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    word_index   INT  NOT NULL,
    word         TEXT NOT NULL,
    start_ms     INT  NOT NULL,
    end_ms       INT  NOT NULL,
    UNIQUE (paragraph_id, word_index)
);

CREATE INDEX IF NOT EXISTS idx_para_word_ts_paragraph
    ON paragraph_word_timestamps (paragraph_id);
