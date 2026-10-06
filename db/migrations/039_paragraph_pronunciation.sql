-- Pronunciación en español de un párrafo, igual que phrases.pronunciation_es.
-- Permite leer una historia en modo "español primero": ver la traducción, decir la
-- frase en inglés de memoria y recién después revelar el inglés con su pronunciación.
ALTER TABLE paragraphs ADD COLUMN IF NOT EXISTS pronunciation_es TEXT;
