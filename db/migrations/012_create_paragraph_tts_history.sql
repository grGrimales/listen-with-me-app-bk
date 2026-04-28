CREATE TABLE paragraph_tts_history (
    id           UUID         DEFAULT gen_random_uuid() PRIMARY KEY,
    paragraph_id INTEGER      NOT NULL REFERENCES paragraphs(id) ON DELETE CASCADE,
    audio_url    TEXT         NOT NULL,
    voice_name   VARCHAR(255) NOT NULL DEFAULT '',
    model_id     VARCHAR(255) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX ON paragraph_tts_history(paragraph_id, created_at DESC);
