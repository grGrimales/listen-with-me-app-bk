ALTER TABLE tts_voices ADD COLUMN IF NOT EXISTS language VARCHAR(10) NOT NULL DEFAULT 'en';

-- Mark Portuguese voices
UPDATE tts_voices SET language = 'pt' WHERE voice_id IN ('MZxV5lN3cv7hi1376O0m', '0YziWIrqiRTHCxeg1lyc');
