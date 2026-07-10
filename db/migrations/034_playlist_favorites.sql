-- Per-user favorites for story playlists, so a user (including someone a playlist
-- was shared with) can favorite it independently of the owner.
CREATE TABLE IF NOT EXISTS playlist_favorites (
    playlist_id INT  NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (playlist_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_playlist_favorites_user ON playlist_favorites(user_id);

-- Backfill existing owner favorites from the legacy column.
INSERT INTO playlist_favorites (playlist_id, user_id)
    SELECT id, user_id FROM playlists WHERE is_favorite = TRUE
    ON CONFLICT DO NOTHING;
