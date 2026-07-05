-- Split cached Polly audio into female / male voice URLs.
ALTER TABLE phrases RENAME COLUMN polly_audio_url TO polly_audio_url_female;
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS polly_audio_url_male TEXT;
