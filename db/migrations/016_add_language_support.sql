-- Add target_language preference to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS target_language VARCHAR(10) NOT NULL DEFAULT 'en';

-- Add audio_url to paragraph_translations for language-specific audio
ALTER TABLE paragraph_translations ADD COLUMN IF NOT EXISTS audio_url TEXT;

-- Insert Portuguese voices for ElevenLabs TTS
INSERT INTO tts_voices (provider, voice_id, name, description, enabled)
SELECT 'elevenlabs', 'MZxV5lN3cv7hi1376O0m', 'Ana Dias', 'Voz femenina en portugués', true
WHERE NOT EXISTS (SELECT 1 FROM tts_voices WHERE voice_id = 'MZxV5lN3cv7hi1376O0m');

INSERT INTO tts_voices (provider, voice_id, name, description, enabled)
SELECT 'elevenlabs', '0YziWIrqiRTHCxeg1lyc', 'Will', 'Voz masculina en portugués', true
WHERE NOT EXISTS (SELECT 1 FROM tts_voices WHERE voice_id = '0YziWIrqiRTHCxeg1lyc');
