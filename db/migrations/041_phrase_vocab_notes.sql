-- vocab_notes: ficha de una palabra guardada (playlists "📗 Vocab · …"), explicación breve
-- en español de qué significa, cómo se usa y alguna curiosidad. La tarjeta la muestra al
-- revelar la respuesta.
-- enriched_at: flag del skill actualizar-playlist-vocabulario, cuándo se completaron
-- traducción, pronunciación y vocab_notes. NULL = la palabra aún está pendiente.
ALTER TABLE phrases
  ADD COLUMN IF NOT EXISTS vocab_notes TEXT,
  ADD COLUMN IF NOT EXISTS enriched_at TIMESTAMPTZ;
