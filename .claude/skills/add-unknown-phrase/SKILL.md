---
name: add-unknown-phrase
description: Agrega al phrase playlist "desconocidas" una palabra o expresión en inglés que el usuario leyó/escuchó y no conoce. Por cada expresión genera 3 frases cortas y útiles que la usan (1 en B1, 1 en B2, 1 en C1) con traducción y pronunciación, las muestra para aprobación y solo entonces las inserta. Los grupos se llaman 1, 2, 3… y tienen exactamente 50 frases; al llenarse uno, las siguientes van al siguiente. Úsalo cuando el usuario pase una palabra/frase desconocida, diga "agrega esta frase a desconocidas", o pegue expresiones con `*`. Con el argumento `power-of-now` saca las expresiones de las capturas del libro en `backend/power-of-now/` y lleva el listado de marcadas en `backend/power-of-now/marcadas.tsv` para no repetirlas.
user-invocable: true
---

# Agregar expresiones desconocidas

Argumento recibido: `$ARGUMENTS`

```
/add-unknown-phrase how to tell at a glance
/add-unknown-phrase * how to tell at a glance * It is worth it
/add-unknown-phrase                      → pregunta qué expresión
```

Una o varias expresiones, separadas por `*`, `;` o saltos de línea. Quita viñetas
y espacios sobrantes. Si no llega nada, pregunta cuál es la expresión.

## Modo `power-of-now`

```
/add-unknown-phrase power-of-now
```

Si el argumento es exactamente `power-of-now`, las expresiones no vienen en el
mensaje: salen de las capturas del libro *The Power of Now* que el usuario dejó en
`backend/power-of-now/`. El playlist sigue
siendo `desconocidas`; solo cambia de dónde salen las expresiones.

| Qué | Dónde |
|---|---|
| Zips de Google Drive | `power-of-now/*.zip` |
| Capturas | `power-of-now/capturas/` (`.png`, `.jpg`) |
| Listado de expresiones marcadas | `power-of-now/marcadas.tsv` |

`marcadas.tsv` está separado por tabuladores, con una fila por expresión **marcada
en el libro** (no por frase generada):

```
expresion	estado	captura	contexto
farther afield	pendiente	IMG-20261016-WA0003.jpg	placing copies of the book in spiritual bookstores farther afield
```

- `expresion`: forma base en minúsculas (`nun`, no `nuns,`; `to give birth to`,
  no `to give birth to it`); sin puntuación del libro.
- `estado`: `pendiente`, `agregada AAAA-MM-DD` o `descartada`.
- `captura`: nombre del archivo donde aparece.
- `contexto`: la oración del libro, recortada, para entender el sentido.

Pasos previos (sustituyen al argumento):

1. **Extraer zips nuevos** sin sobrescribir:
   `unzip -n -q <zip> -d power-of-now/capturas`. No borres los zips. Zips y capturas están en `.gitignore` (copyright del libro); solo `marcadas.tsv` se sube a GitHub.
2. **Capturas nuevas** = las de `capturas/` cuyo nombre no aparece en la columna
   `captura` del TSV. Ábrelas con Read en orden de lectura (posición/porcentaje
   del pie de página) y saca los fragmentos resaltados (azul o amarillo). Un
   resaltado que corta una frase a la mitad (`handed over had`) se reduce a la
   expresión que tiene sentido (`handed over`).
3. **Deduplicar** cada expresión contra el TSV antes de agregarla:
   `awk -F'\t' 'tolower($1)=="<expresion>"' power-of-now/marcadas.tsv` (no uses
   `grep -P`: falla en el Git Bash de Windows). Si ya está, no se repite. Las nuevas se añaden al final con estado `pendiente`.
4. **Tanda**: toma hasta **5** expresiones `pendiente`, en el orden del archivo, y
   sigue con el procedimiento normal (pasos 1–7). Si el usuario pasa un número
   (`power-of-now 8`), usa ese tamaño de tanda. Cuando la expresión del libro es
   ambigua o probablemente se resaltó por error (p. ej. una palabra muy básica),
   pregúntalo en la tabla de aprobación.
5. **Tras insertar** (paso 6 verificado), cambia el estado de esas filas a
   `agregada AAAA-MM-DD`. Las que el usuario rechace quedan `descartada`. No
   marques nada si la inserción falló.
6. En el reporte final di cuántas siguen `pendiente`.

## Datos fijos

| Qué | Valor |
|---|---|
| Playlist | `phrase_playlists.name = 'desconocidas'`, `language = 'en'` |
| Dueño | usuario con email `carlos.guzman.0515@gmail.com` |
| Tipo | playlist normal: `story_playlist_id IS NULL AND parent_playlist_id IS NULL` |
| Grupos | `name` = `'1'`, `'2'`, … (solo el número); `position` = número − 1 |
| Tamaño de grupo | **exactamente 50** frases; la frase 51 abre el grupo siguiente |

Una tanda puede partirse entre dos grupos (p. ej. 2 frases cierran el grupo 1 y
2 abren el grupo 2). Eso es correcto.

## Herramienta de base de datos

`backend/cmd/dbq`, siempre desde `backend/`:

```bash
go run ./cmd/dbq "SELECT ..."          # SQL inline
go run ./cmd/dbq "@/ruta/script.sql"   # SQL desde archivo
```

**Paso 0 — di a qué base apuntas** antes de escribir nada:

```bash
grep -oE '^DATABASE_URL=.*@[^/?]+' .env | sed 's/.*@//'
```

## Procedimiento

### 1. Estado actual del playlist

```sql
SELECT p.id AS playlist_id, g.id AS group_id, g.name, g.position, count(ph.id) AS frases
FROM phrase_playlists p
JOIN users u ON u.id = p.user_id
LEFT JOIN phrase_groups g ON g.phrase_playlist_id = p.id
LEFT JOIN phrases ph      ON ph.phrase_group_id   = g.id
WHERE u.email = 'carlos.guzman.0515@gmail.com' AND p.name = 'desconocidas'
  AND p.story_playlist_id IS NULL AND p.parent_playlist_id IS NULL
GROUP BY p.id, g.id, g.name, g.position
ORDER BY g.position;
```

0 filas = el playlist aún no existe; se crea en el paso 5. Con esto calcula en
qué grupo y posición caerá cada frase nueva.

### 2. Duplicados

Por cada expresión, mira si ya está en el playlist:

```sql
SELECT ph.id, g.name AS grupo, ph.text
FROM phrases ph
JOIN phrase_groups g    ON g.id = ph.phrase_group_id
JOIN phrase_playlists p ON p.id = g.phrase_playlist_id
JOIN users u            ON u.id = p.user_id
WHERE u.email = 'carlos.guzman.0515@gmail.com' AND p.name = 'desconocidas'
  AND ph.text ILIKE '%<expresión>%';
```

Si hay coincidencias, muéstralas y pregunta si igual quiere 3 frases más.

### 3. Escribir las 3 frases por expresión

Orden fijo: **B1, B2, C1**.

- **Cortas**: idealmente 6–12 palabras; que se lean y escuchen de un vistazo.
- **Contienen la expresión** literal, o con la conjugación/forma natural que pida
  la oración (`it's worth it`, `was it worth it?`). Debe ser lo que el usuario
  aprende, no un sinónimo.
- **Útiles y naturales**: situaciones cotidianas o de trabajo que de verdad dirías.
  Nada rebuscado, ni literario, ni forzado para meter vocabulario difícil.
- **Lógica**: cada frase tiene sentido sola, sin contexto previo.
- **Variedad**: cada una muestra un uso distinto (afirmación, pregunta, negación,
  otro tiempo verbal, otro registro). El nivel sube por estructura y vocabulario
  alrededor, no por longitud.
  - B1: estructura simple, vocabulario frecuente.
  - B2: subordinada, condicional, matiz de opinión.
  - C1: registro más preciso o idiomático, pero igual de corta y natural.
- `translation_es`: español natural (no calco), mayúscula inicial y puntuación final.
- `pronunciation_es`: fonética aproximada al español, minúsculas, con tilde en la
  sílaba fuerte, conservando `?`/`!`. Estilo del playlist existente:
  `"How's everything with you and your family?"` → `jáus évrizing wid yu and yor fámili?`
  Convenciones: `th`→`d`/`z`, `h`→`j`, `w`→`u`, `-tion`→`-shon`.
- `focus_text`: la expresión **tal como aparece en esa frase** (`hand over` en
  "I'll hand over…", `handed over` en "I handed over…", `foreseen` en "could have
  foreseen"). Tiene que ser un trozo literal de `text` (sin importar mayúsculas):
  la tarjeta lo resalta y, al hacer click, reproduce solo ese tramo del audio.
  Sin puntuación en los extremos.

### 4. Mostrar y esperar aprobación (obligatorio)

Una tabla por expresión:

| # | Nivel | Frase | Traducción | Pronunciación | Grupo / pos. |
|---|---|---|---|---|---|

En la columna Frase, marca el `focus_text` en **negrita** para que se vea qué parte
quedará resaltada.

Termina preguntando si las aprueba. **No insertes nada sin un "sí" explícito.**
Si pide cambios, rehace solo las frases señaladas y vuelve a mostrar la tabla
completa.

### 5. Insertar

Escribe el script en el scratchpad (`insert_unknown.sql`). Es un único bloque `DO`,
así que es atómico: o entra todo o nada. Crea el playlist si falta y decide el grupo
frase por frase, abriendo uno nuevo cuando el actual llega a 50. Las comillas
simples del texto se escapan duplicándolas (`''`). `ord` fija el orden de inserción
(por expresión: B1, B2, C1).

```sql
DO $$
DECLARE
  uid uuid; pl int; gid int; gpos int; n int; r record;
BEGIN
  SELECT id INTO uid FROM users WHERE email = 'carlos.guzman.0515@gmail.com';
  IF uid IS NULL THEN RAISE EXCEPTION 'usuario no encontrado'; END IF;

  SELECT id INTO pl FROM phrase_playlists
  WHERE user_id = uid AND name = 'desconocidas'
    AND story_playlist_id IS NULL AND parent_playlist_id IS NULL;
  IF pl IS NULL THEN
    INSERT INTO phrase_playlists (user_id, name, description, language)
    VALUES (uid, 'desconocidas', 'Expresiones que leí o escuché y no conocía: 3 frases de uso por cada una (B1, B2, C1).', 'en')
    RETURNING id INTO pl;
  END IF;

  FOR r IN
    SELECT * FROM (VALUES
      (1, 'English text', 'Traducción.', 'pronunsiéishon', 'text'),
      (2, '...', '...', '...', '...')
    ) AS v(ord, txt, tr, pr, focus) ORDER BY ord
  LOOP
    SELECT id, position INTO gid, gpos FROM phrase_groups
    WHERE phrase_playlist_id = pl ORDER BY position DESC LIMIT 1;

    IF gid IS NULL THEN
      INSERT INTO phrase_groups (phrase_playlist_id, name, position)
      VALUES (pl, '1', 0) RETURNING id INTO gid;
      n := 0;
    ELSE
      SELECT count(*) INTO n FROM phrases WHERE phrase_group_id = gid;
      IF n >= 50 THEN
        INSERT INTO phrase_groups (phrase_playlist_id, name, position)
        VALUES (pl, (gpos + 2)::text, gpos + 1) RETURNING id INTO gid;
        n := 0;
      END IF;
    END IF;

    IF position(lower(r.focus) IN lower(r.txt)) = 0 THEN
      RAISE EXCEPTION 'focus_text "%" no aparece en "%"', r.focus, r.txt;
    END IF;

    INSERT INTO phrases (phrase_group_id, text, translation_es, pronunciation_es, position, focus_text)
    VALUES (gid, r.txt, r.tr, r.pr, n, r.focus);
  END LOOP;
END $$;
```

```bash
go run ./cmd/dbq "@<scratchpad>/insert_unknown.sql"
```

Solo se insertan filas en `phrase_playlists` (si falta), `phrase_groups` y
`phrases` de este playlist. Nunca se actualiza ni borra nada.

### 6. Verificar

Repite la consulta del paso 1 y comprueba:

| Comprobación | Esperado |
|---|---|
| Frases totales | anterior + 3 × nº de expresiones |
| Cada grupo salvo el último | exactamente 50 |
| Último grupo | entre 1 y 50 |
| Nombres de grupo | `1`, `2`, … consecutivos con `position` = nombre − 1 |

Y las frases recién insertadas:

```sql
SELECT ph.id, g.name AS grupo, ph.position, ph.text
FROM phrases ph
JOIN phrase_groups g    ON g.id = ph.phrase_group_id
JOIN phrase_playlists p ON p.id = g.phrase_playlist_id
JOIN users u            ON u.id = p.user_id
WHERE u.email = 'carlos.guzman.0515@gmail.com' AND p.name = 'desconocidas'
  AND ph.created_at > NOW() - INTERVAL '10 minutes'
ORDER BY g.position, ph.position;
```

Si algo no cuadra, dilo antes de seguir.

### 7. Reportar

Base de datos usada, cuántas frases entraron y en qué grupo(s), ids, y el enlace
`/phrases/<playlist_id>`.

## Lo que este skill no hace

- **No genera audio.** `polly_audio_url_female` queda en `NULL`; la app genera el
  audio ElevenLabs al reproducir la tarjeta (`POST /api/phrases/{id}/audio/generate`)
  y consume créditos solo entonces.
- No toca otros playlists ni reordena frases existentes.
