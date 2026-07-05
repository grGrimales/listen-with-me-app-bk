-- Vocab is now a real playlist (child) linked to a parent phrase_playlist.
-- One child per (user, parent). Cascades: parent deleted -> all children deleted.
DROP TABLE IF EXISTS phrase_playlist_vocabulary;

ALTER TABLE phrase_playlists
    ADD COLUMN IF NOT EXISTS parent_playlist_id INTEGER
        REFERENCES phrase_playlists(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX IF NOT EXISTS ux_vocab_child_per_user_parent
    ON phrase_playlists(user_id, parent_playlist_id)
    WHERE parent_playlist_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_phrase_playlists_parent
    ON phrase_playlists(parent_playlist_id);
