-- Independent listening log for phrase Zen mode (separate from phrase_reviews).
CREATE TABLE IF NOT EXISTS phrase_zen_listens (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phrase_id   INTEGER NOT NULL REFERENCES phrases(id) ON DELETE CASCADE,
    playlist_id INTEGER REFERENCES phrase_playlists(id) ON DELETE SET NULL,
    listened_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_phrase_zen_user_time   ON phrase_zen_listens(user_id, listened_at DESC);
CREATE INDEX IF NOT EXISTS idx_phrase_zen_user_phrase ON phrase_zen_listens(user_id, phrase_id);
CREATE INDEX IF NOT EXISTS idx_phrase_zen_time        ON phrase_zen_listens(listened_at DESC);
