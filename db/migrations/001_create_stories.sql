CREATE TABLE IF NOT EXISTS stories (
    id              SERIAL PRIMARY KEY,
    title           VARCHAR(255) NOT NULL,
    description     TEXT,
    audio_url       TEXT NOT NULL,
    level           VARCHAR(20) NOT NULL DEFAULT 'beginner',
    duration_seconds INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
