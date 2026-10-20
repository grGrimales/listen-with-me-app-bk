-- Frase de la que se sacó una palabra del vocabulario (playlists "📗 Vocab · …"). La misma
-- palabra puede guardarse varias veces, una por frase donde resultó desconocida: cada fila
-- tiene su propio sentido, ficha y conteo de repasos. NULL = palabra guardada antes de
-- esta migración, sin frase conocida.
ALTER TABLE phrases
  ADD COLUMN IF NOT EXISTS vocab_source_phrase_id INT REFERENCES phrases(id) ON DELETE SET NULL;
