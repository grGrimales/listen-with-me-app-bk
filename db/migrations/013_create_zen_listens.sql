CREATE TABLE IF NOT EXISTS zen_listens (
  id        BIGSERIAL    PRIMARY KEY,
  user_id   UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  story_id  INTEGER      NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
  listened_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_zen_listens_user_id  ON zen_listens(user_id);
CREATE INDEX IF NOT EXISTS idx_zen_listens_story_id ON zen_listens(story_id);
