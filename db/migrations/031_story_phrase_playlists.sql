-- Story-linked phrase playlists: created per (user, story) when the user saves a
-- word from a story. Their audio is a segment of the story's own audio (no Polly).
ALTER TABLE phrase_playlists
    ADD COLUMN IF NOT EXISTS story_id INT REFERENCES stories(id) ON DELETE CASCADE;

-- One story-phrase playlist per user per story.
CREATE UNIQUE INDEX IF NOT EXISTS ux_story_phrase_playlist
    ON phrase_playlists(user_id, story_id)
    WHERE story_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_phrase_playlists_story
    ON phrase_playlists(story_id);

-- A phrase can reference a segment of a story's audio instead of Polly audio.
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_audio_url TEXT;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_start_ms  INT;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS source_end_ms    INT;
