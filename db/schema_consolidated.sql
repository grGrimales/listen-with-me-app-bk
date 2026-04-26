-- Consolidation of all migrations in correct order

-- 1. Extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 2. Users (from 003)
CREATE TABLE IF NOT EXISTS users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    "fullName" VARCHAR(255) NOT NULL,
    email      VARCHAR(255) NOT NULL UNIQUE,
    password   VARCHAR(255) NOT NULL,
    roles      VARCHAR(20)[] NOT NULL DEFAULT '{user}',
    "isActive" BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 3. Categories (from 002)
CREATE TABLE IF NOT EXISTS categories (
    id   SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    slug VARCHAR(100) NOT NULL UNIQUE
);

-- 4. Stories (from 002 + 005)
CREATE TABLE IF NOT EXISTS stories (
    id          SERIAL PRIMARY KEY,
    title       VARCHAR(255) NOT NULL,
    level       VARCHAR(5)   NOT NULL CHECK (level IN ('A1','A2','B1','B2','C1','C2')),
    category_id INT          NOT NULL REFERENCES categories(id),
    cover_url   TEXT,
    author      VARCHAR(255),
    status      VARCHAR(20)  NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','deleted')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 5. Paragraphs (from 002 + 004)
CREATE TABLE IF NOT EXISTS paragraphs (
    id        SERIAL PRIMARY KEY,
    story_id  INT  NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    position  INT  NOT NULL,
    content   TEXT NOT NULL,
    image_url TEXT,
    audio_url TEXT, -- from 004
    UNIQUE (story_id, position)
);

-- 6. Paragraph translations (from 002)
CREATE TABLE IF NOT EXISTS paragraph_translations (
    id           SERIAL PRIMARY KEY,
    paragraph_id INT         NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    language     VARCHAR(10) NOT NULL,
    content      TEXT        NOT NULL,
    UNIQUE (paragraph_id, language)
);

-- 7. Vocabulary (from 002)
CREATE TABLE IF NOT EXISTS vocabulary (
    id           SERIAL PRIMARY KEY,
    paragraph_id INT          NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    word         VARCHAR(100) NOT NULL,
    definition   TEXT         NOT NULL
);

-- 8. Story voices (from 002)
CREATE TABLE IF NOT EXISTS story_voices (
    id         SERIAL       PRIMARY KEY,
    story_id   INT          NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    audio_url  TEXT         NOT NULL,
    timestamps JSONB
);

-- 9. User progress (from 002)
CREATE TABLE IF NOT EXISTS user_progress (
    id           SERIAL      PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id     INT         NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    voice_id     INT         REFERENCES story_voices(id),
    completed    BOOLEAN     NOT NULL DEFAULT FALSE,
    completed_at TIMESTAMPTZ,
    UNIQUE (user_id, story_id)
);

-- 10. Paragraph Images (from 006)
CREATE TABLE IF NOT EXISTS paragraph_images (
    id           SERIAL PRIMARY KEY,
    paragraph_id INTEGER NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    image_url    TEXT NOT NULL,
    position     INTEGER NOT NULL DEFAULT 0,
    created_at   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 11. User story reviews (from 007)
CREATE TABLE IF NOT EXISTS user_story_reviews (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id INTEGER NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    reviewed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 12. Playlists (from 008 + 009)
CREATE TABLE IF NOT EXISTS playlists (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_user_playlist_name UNIQUE (user_id, name) -- from 009
);

CREATE TABLE IF NOT EXISTS playlist_stories (
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    story_id INTEGER NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    added_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (playlist_id, story_id)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_user_story_reviews_user_id ON user_story_reviews(user_id);
CREATE INDEX IF NOT EXISTS idx_user_story_reviews_reviewed_at ON user_story_reviews(reviewed_at);
CREATE INDEX IF NOT EXISTS idx_playlists_user_id ON playlists(user_id);

-- Seeds
INSERT INTO categories (name, slug) VALUES
    ('General',      'general'),
    ('Sports',       'sports'),
    ('Science',      'science'),
    ('History',      'history'),
    ('Technology',   'technology'),
    ('Culture',      'culture'),
    ('Politics',     'politics'),
    ('Travel',       'travel')
ON CONFLICT DO NOTHING;
