CREATE TABLE IF NOT EXISTS story_sentences (
    id SERIAL PRIMARY KEY,
    story_id INTEGER NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    paragraph_id INTEGER REFERENCES paragraphs(id) ON DELETE CASCADE,
    en TEXT NOT NULL,
    es TEXT NOT NULL,
    position INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_sentence_attempts (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sentence_id INTEGER NOT NULL REFERENCES story_sentences(id) ON DELETE CASCADE,
    is_correct BOOLEAN NOT NULL,
    user_answer TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for performance
CREATE INDEX idx_user_sentence_attempts_user_id ON user_sentence_attempts(user_id);
CREATE INDEX idx_user_sentence_attempts_sentence_id ON user_sentence_attempts(sentence_id);
CREATE INDEX idx_story_sentences_story_id ON story_sentences(story_id);
