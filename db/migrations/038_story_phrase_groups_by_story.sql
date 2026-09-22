-- Word playlists built from story playlists used a single default "Words" group.
-- Group those words by the story they came from instead, naming each group after
-- the story's title. Only story-derived word playlists are touched.

DO $$
DECLARE
    r         RECORD;
    group_id  INT;
    group_pos INT;
BEGIN
    FOR r IN
        SELECT DISTINCT pp.id AS playlist_id, s.id AS story_id, s.title AS title
        FROM phrase_playlists pp
        JOIN phrase_groups g ON g.phrase_playlist_id = pp.id
        JOIN phrases ph      ON ph.phrase_group_id = g.id
        JOIN stories s       ON s.id = ph.source_story_id
        WHERE pp.story_playlist_id IS NOT NULL
          AND TRIM(s.title) <> ''
        ORDER BY pp.id, s.title
    LOOP
        group_id := NULL;
        SELECT id INTO group_id
        FROM phrase_groups
        WHERE phrase_playlist_id = r.playlist_id AND lower(name) = lower(TRIM(r.title))
        ORDER BY position, id
        LIMIT 1;

        IF group_id IS NULL THEN
            SELECT COALESCE(MAX(position), -1) + 1 INTO group_pos
            FROM phrase_groups WHERE phrase_playlist_id = r.playlist_id;

            INSERT INTO phrase_groups (phrase_playlist_id, name, position)
            VALUES (r.playlist_id, TRIM(r.title), group_pos)
            RETURNING id INTO group_id;
        END IF;

        UPDATE phrases ph
        SET phrase_group_id = group_id
        FROM phrase_groups g
        WHERE ph.phrase_group_id = g.id
          AND g.phrase_playlist_id = r.playlist_id
          AND ph.source_story_id = r.story_id
          AND ph.phrase_group_id <> group_id;
    END LOOP;
END $$;

-- Drop the old default group when every word moved out of it.
DELETE FROM phrase_groups g
WHERE g.name = 'Words'
  AND EXISTS (
      SELECT 1 FROM phrase_playlists pp
      WHERE pp.id = g.phrase_playlist_id AND pp.story_playlist_id IS NOT NULL
  )
  AND NOT EXISTS (SELECT 1 FROM phrases ph WHERE ph.phrase_group_id = g.id);

-- Re-number phrase positions inside each regrouped playlist.
WITH ranked AS (
    SELECT ph.id,
           ROW_NUMBER() OVER (PARTITION BY ph.phrase_group_id ORDER BY ph.position, ph.id) - 1 AS rn
    FROM phrases ph
    JOIN phrase_groups g     ON g.id = ph.phrase_group_id
    JOIN phrase_playlists pp ON pp.id = g.phrase_playlist_id
    WHERE pp.story_playlist_id IS NOT NULL
)
UPDATE phrases ph
SET position = ranked.rn
FROM ranked
WHERE ranked.id = ph.id AND ph.position <> ranked.rn;
