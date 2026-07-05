CREATE TABLE IF NOT EXISTS phrase_playlist_vocabulary (
    id SERIAL PRIMARY KEY,
    phrase_playlist_id INTEGER NOT NULL REFERENCES phrase_playlists(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phrase_id INTEGER REFERENCES phrases(id) ON DELETE SET NULL,
    text TEXT NOT NULL,
    translation_es TEXT,
    notes TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ppv_user_playlist ON phrase_playlist_vocabulary(user_id, phrase_playlist_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ppv_user_playlist_text
    ON phrase_playlist_vocabulary(user_id, phrase_playlist_id, lower(text));
