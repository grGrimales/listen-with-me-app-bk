CREATE TABLE tts_voices (
    id          UUID         DEFAULT gen_random_uuid() PRIMARY KEY,
    provider    VARCHAR(50)  NOT NULL DEFAULT 'elevenlabs',
    voice_id    VARCHAR(255) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description TEXT         NOT NULL DEFAULT '',
    enabled     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

INSERT INTO tts_voices (provider, voice_id, name, description) VALUES
    ('elevenlabs', 'tnSpp4vdxKPjI9w0GnoV', 'Voice 1', ''),
    ('elevenlabs', 'UgBBYS2sOqTuMpoF3BR0', 'Voice 2', '');
