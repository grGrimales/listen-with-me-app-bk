-- Categories
CREATE TABLE IF NOT EXISTS categories (
    id   SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    slug VARCHAR(100) NOT NULL UNIQUE
);

-- Stories
CREATE TABLE IF NOT EXISTS stories (
    id          SERIAL PRIMARY KEY,
    title       VARCHAR(255) NOT NULL,
    level       VARCHAR(5)   NOT NULL CHECK (level IN ('A1','A2','B1','B2','C1','C2')),
    category_id INT          NOT NULL REFERENCES categories(id),
    cover_url   TEXT,
    author      VARCHAR(255),
    status      VARCHAR(20)  NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Paragraphs
CREATE TABLE IF NOT EXISTS paragraphs (
    id        SERIAL PRIMARY KEY,
    story_id  INT  NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    position  INT  NOT NULL,
    content   TEXT NOT NULL,
    image_url TEXT,
    UNIQUE (story_id, position)
);

-- Paragraph translations
CREATE TABLE IF NOT EXISTS paragraph_translations (
    id           SERIAL PRIMARY KEY,
    paragraph_id INT         NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    language     VARCHAR(10) NOT NULL,
    content      TEXT        NOT NULL,
    UNIQUE (paragraph_id, language)
);

-- Vocabulary per paragraph
CREATE TABLE IF NOT EXISTS vocabulary (
    id           SERIAL PRIMARY KEY,
    paragraph_id INT          NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    word         VARCHAR(100) NOT NULL,
    definition   TEXT         NOT NULL
);

-- Story voices (one audio file per voice)
CREATE TABLE IF NOT EXISTS story_voices (
    id         SERIAL       PRIMARY KEY,
    story_id   INT          NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    audio_url  TEXT         NOT NULL,
    timestamps JSONB
);

-- User progress
CREATE TABLE IF NOT EXISTS user_progress (
    id           SERIAL      PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id     INT         NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    voice_id     INT         REFERENCES story_voices(id),
    completed    BOOLEAN     NOT NULL DEFAULT FALSE,
    completed_at TIMESTAMPTZ,
    UNIQUE (user_id, story_id)
);

-- Seed: basic categories
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
