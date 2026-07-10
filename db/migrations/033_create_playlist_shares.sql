-- Sharing for story playlists: the owner grants read/editor access to other users.
CREATE TABLE IF NOT EXISTS playlist_shares (
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    user_id     UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission  VARCHAR(20) NOT NULL CHECK (permission IN ('read', 'editor')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (playlist_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_playlist_shares_user ON playlist_shares(user_id);
