CREATE TABLE IF NOT EXISTS phrase_reviews (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phrase_id INTEGER NOT NULL REFERENCES phrases(id) ON DELETE CASCADE,
    reviewed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_phrase_reviews_user_time ON phrase_reviews(user_id, reviewed_at DESC);
CREATE INDEX IF NOT EXISTS idx_phrase_reviews_time ON phrase_reviews(reviewed_at DESC);
