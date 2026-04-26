CREATE TABLE IF NOT EXISTS user_story_reviews (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id INTEGER NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    reviewed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_user_story_reviews_user_id ON user_story_reviews(user_id);
CREATE INDEX idx_user_story_reviews_reviewed_at ON user_story_reviews(reviewed_at);
