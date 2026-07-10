-- The word playlist is now per story-PLAYLIST (a user's collection of stories),
-- not per individual story. Re-key phrase_playlists from story_id to story_playlist_id.
DELETE FROM phrase_playlists WHERE story_id IS NOT NULL;
ALTER TABLE phrase_playlists DROP COLUMN IF EXISTS story_id;

ALTER TABLE phrase_playlists
    ADD COLUMN IF NOT EXISTS story_playlist_id INT REFERENCES playlists(id) ON DELETE CASCADE;

-- One word playlist per user per story-playlist.
CREATE UNIQUE INDEX IF NOT EXISTS ux_storyplaylist_phrase_playlist
    ON phrase_playlists(user_id, story_playlist_id)
    WHERE story_playlist_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_phrase_playlists_story_playlist
    ON phrase_playlists(story_playlist_id);

-- Remember which story a saved word came from (to reuse that story's audio segment).
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_story_id INT REFERENCES stories(id) ON DELETE SET NULL;
