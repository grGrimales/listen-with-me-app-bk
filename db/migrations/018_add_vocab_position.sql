ALTER TABLE user_story_vocabulary
ADD COLUMN IF NOT EXISTS position INTEGER NOT NULL DEFAULT 0;

WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY user_id, story_id ORDER BY created_at DESC) - 1 AS rn
  FROM user_story_vocabulary
)
UPDATE user_story_vocabulary
SET position = ranked.rn
FROM ranked
WHERE user_story_vocabulary.id = ranked.id;
