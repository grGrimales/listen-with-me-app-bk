-- Spaced repetition state per (user, phrase) — SM-2 style
CREATE TABLE IF NOT EXISTS user_phrase_srs (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phrase_id INTEGER NOT NULL REFERENCES phrases(id) ON DELETE CASCADE,
    ease_factor REAL NOT NULL DEFAULT 2.5,
    interval_days REAL NOT NULL DEFAULT 0,
    repetitions INTEGER NOT NULL DEFAULT 0,
    lapses INTEGER NOT NULL DEFAULT 0,
    last_reviewed_at TIMESTAMPTZ,
    next_review_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, phrase_id)
);
CREATE INDEX IF NOT EXISTS idx_user_phrase_srs_next ON user_phrase_srs(user_id, next_review_at);
