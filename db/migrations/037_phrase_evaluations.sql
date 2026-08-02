-- Writing evaluation for phrases: the user reads the Spanish translation and types
-- the phrase in the target language. One row per attempt — both hits and misses —
-- so the user can review what they wrote and where they failed in the past.
CREATE TABLE IF NOT EXISTS phrase_evaluations (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID    NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    phrase_id   INTEGER NOT NULL REFERENCES phrases(id) ON DELETE CASCADE,
    playlist_id INTEGER REFERENCES phrase_playlists(id) ON DELETE SET NULL,
    is_correct  BOOLEAN NOT NULL,
    user_answer TEXT NOT NULL DEFAULT '',
    -- Snapshot of the expected text: phrases can be edited later, the history should
    -- still show what was being asked at the time of the attempt.
    expected    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_phrase_eval_user_phrase   ON phrase_evaluations(user_id, phrase_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_phrase_eval_user_playlist ON phrase_evaluations(user_id, playlist_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_phrase_eval_time          ON phrase_evaluations(created_at DESC);
