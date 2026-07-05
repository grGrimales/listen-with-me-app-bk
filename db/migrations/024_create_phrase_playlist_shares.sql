CREATE TABLE IF NOT EXISTS phrase_playlist_shares (
    playlist_id INTEGER NOT NULL REFERENCES phrase_playlists(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission VARCHAR(20) NOT NULL CHECK (permission IN ('read', 'editor')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (playlist_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_phrase_playlist_shares_user ON phrase_playlist_shares(user_id);
