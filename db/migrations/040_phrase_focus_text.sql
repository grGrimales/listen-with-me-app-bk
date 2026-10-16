-- Expresión que la frase enseña (p. ej. "farther afield" en "Their customers now come
-- from much farther afield."), copiada tal como aparece en phrases.text. La tarjeta la
-- resalta y, al hacer click, reproduce solo ese tramo del audio usando
-- phrase_word_timestamps. NULL = la frase no tiene expresión destacada.
ALTER TABLE phrases ADD COLUMN IF NOT EXISTS focus_text TEXT;
