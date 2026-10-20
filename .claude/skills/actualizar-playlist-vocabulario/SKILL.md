---
name: actualizar-playlist-vocabulario
description: Barre un playlist de vocabulario de frases ("📗 Vocab · …", p. ej. /phrases/27) y completa cada palabra pendiente con traducción, pronunciación y una ficha breve en español (significado, uso y curiosidades). Antes, sincroniza al vocabulario las expresiones que el skill add-unknown-phrase metió en el playlist padre, además de las que el usuario sombreó en la app. Usa la columna phrases.enriched_at como flag para saber cuáles ya están completas. Úsalo cuando el usuario pida actualizar, completar o enriquecer un playlist de vocabulario, o pase una ruta /phrases/<id> de vocabulario.
user-invocable: true
---

# Actualizar playlist de vocabulario

Argumento recibido: `$ARGUMENTS`

```
/actualizar-playlist-vocabulario /phrases/27
/actualizar-playlist-vocabulario 27
/actualizar-playlist-vocabulario 27 10      → tandas de 10 palabras
/actualizar-playlist-vocabulario            → pregunta qué playlist
```

El id es el último número de la ruta o URL (`http://localhost:5173/phrases/27` → 27).
Un segundo número opcional es el tamaño de la tanda (por defecto **15**).

## Qué hay en la base

- Un playlist de vocabulario es un `phrase_playlists` con `parent_playlist_id` (el
  playlist de frases del que salió). La app lo crea con el nombre `📗 Vocab · <padre>`
  y un único grupo `Words`.
- Cada palabra es una fila de `phrases` en ese grupo. Cuando el usuario sombrea un
  texto en la tarjeta y lo guarda, la app inserta la fila con `translation_es` y
  `pronunciation_es` vacíos.
- `phrases.vocab_notes` (mig 041): la ficha de la palabra. La tarjeta la muestra al
  revelar la respuesta, como texto plano que respeta los saltos de línea.
- `phrases.enriched_at` (mig 041): **el flag**. `NULL` = falta completar;
  con fecha = este skill ya la completó. Solo se procesan las filas con `NULL`.

## Herramienta de base de datos

`backend/cmd/dbq`, siempre desde `backend/` del proyecto listen-with-me-app.
Solo admite **una sentencia por llamada**.

```bash
go run ./cmd/dbq "SELECT ..."          # SQL inline
go run ./cmd/dbq "@/ruta/script.sql"   # SQL desde archivo (una sola sentencia)
```

**Paso 0: di a qué base apuntas** antes de escribir nada:

```bash
grep -oE '^DATABASE_URL=.*@[^/?]+' .env | sed 's/.*@//'
```

## Procedimiento

### 1. Validar el playlist

```sql
SELECT p.id, p.name, p.language, p.parent_playlist_id, parent.name AS padre,
       (SELECT id FROM phrase_groups WHERE phrase_playlist_id = p.id ORDER BY position, id LIMIT 1) AS grupo_words
FROM phrase_playlists p
LEFT JOIN phrase_playlists parent ON parent.id = p.parent_playlist_id
WHERE p.id = <id>;
```

- Si `parent_playlist_id` es `NULL`, no es un playlist de vocabulario. Busca su hijo
  (`SELECT id, name FROM phrase_playlists WHERE parent_playlist_id = <id>`) y
  pregunta si se refería a ese.
- Si `grupo_words` es `NULL` (vocabulario sin grupo), créalo al insertar en el paso 2:
  `INSERT INTO phrase_groups (phrase_playlist_id, name, position) VALUES (<id>, 'Words', 0)`.

### 2. Sincronizar las expresiones del playlist padre

Hoy el vocabulario solo recibe lo que el usuario sombrea en la app. Las expresiones que
**add-unknown-phrase** metió en el padre (las del `focus_text` de sus frases) también
deben estar.

**Regla de duplicados.** Cada palabra del vocabulario apunta a la frase de la que salió
(`phrases.vocab_source_phrase_id`, mig 042). La misma palabra **puede estar varias
veces si viene de frases distintas**: en otra frase puede significar otra cosa y cada
fila lleva su propio conteo de repasos. Lo único que no se repite es el mismo texto con
la misma frase de origen. La app sigue la misma regla al sombrear.

Lista lo que ya hay y las frases del padre en orden:

```sql
SELECT ph.id, ph.text, ph.vocab_source_phrase_id AS frase, ph.enriched_at IS NOT NULL AS completa
FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
WHERE g.phrase_playlist_id = <vocab_id> ORDER BY g.position, ph.position;
```

```sql
SELECT ph.id, g.name AS grupo, ph.position, ph.focus_text, ph.text
FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
WHERE g.phrase_playlist_id = <padre_id> AND COALESCE(ph.focus_text, '') <> ''
ORDER BY g.position, ph.position;
```

add-unknown-phrase escribe 3 frases de ejemplo seguidas por expresión (algunas antiguas
tienen 4), todas con el mismo sentido. Por eso:

- **Una fila por expresión**, enlazada a la **primera** frase de su serie. El texto es
  el `focus_text` de esa frase, tal cual (en minúsculas salvo nombres propios), para
  que el resaltado y el click-para-escuchar funcionen en ella.
- **Una fila más** por cada variante de la serie que **cambia el sentido**, enlazada a
  su frase: `dawned on` (darse cuenta) en la serie de `dawn` (amanecer), `get up to`
  (andar haciendo) en la de `got up`. Las que solo cambian la forma gramatical
  (`handed over` / `hand over`, `foreseen`, `nuns`, `tenants`) no generan fila.
- Para saber dónde empieza cada serie sirve `backend/registro-desconocidas.tsv` (si el
  padre es `desconocidas`): sus filas `agregada …` están en el mismo orden. Las
  `descartada` no se sincronizan. Para leerlo usa `awk -F'	'` con los campos escritos
  como `$(1)`, `$(2)`, nunca con el dólar pegado al número (al cargar el skill esa forma
  se sustituye por las palabras del argumento).
- No añadas una fila si ya existe una con el mismo texto (sin importar mayúsculas) **y**
  la misma frase de origen. Una palabra que el usuario sombreó desde otra frase no
  impide la nueva fila.

Muestra la lista de filas nuevas (número, texto, frase de origen y, en las variantes, por
qué cambia el sentido) y pregunta si se insertan. **No insertes nada sin un "sí"
explícito**; si el usuario quita alguna, sácala. Inserta todo en una sola sentencia,
al final del grupo `Words`, con `enriched_at` en `NULL` para que el paso 3 las complete:

```sql
INSERT INTO phrases (phrase_group_id, text, translation_es, pronunciation_es, position, vocab_source_phrase_id)
SELECT <grupo_words>, v.txt, '', '',
       (SELECT COALESCE(MAX(position), -1) FROM phrases WHERE phrase_group_id = <grupo_words>) + v.ord,
       v.src
FROM (VALUES (1, 'foresee', 1536), (2, 'farther afield', 1533)) AS v(ord, txt, src)
WHERE NOT EXISTS (
  SELECT 1 FROM phrases x
  WHERE x.phrase_group_id = <grupo_words> AND lower(x.text) = lower(v.txt)
    AND x.vocab_source_phrase_id IS NOT DISTINCT FROM v.src
);
```

Si no hay nada nuevo, dilo y pasa al paso 3.

### 3. Completar las pendientes, por tandas

```sql
SELECT ph.id, ph.text, ph.translation_es, ph.pronunciation_es, src.text AS frase_origen
FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
LEFT JOIN phrases src ON src.id = ph.vocab_source_phrase_id
WHERE g.phrase_playlist_id = <vocab_id> AND ph.enriched_at IS NULL
ORDER BY g.position, ph.position;
```

Para entender en qué sentido se usó cada palabra, mira **su frase de origen**
(`vocab_source_phrase_id`) y, si el padre es `desconocidas`, la columna `contexto` del
registro. La ficha explica **ese** sentido primero. Si la palabra ya tiene otra fila
completada (desde otra frase), no copies su ficha: si el sentido es el mismo, dilo en
una línea y céntrate en lo nuevo de esta frase; si es otro, explica la diferencia.
Las filas antiguas sin frase de origen se buscan en el padre con
`ph.text ILIKE '%<palabra>%'`.

Por cada palabra de la tanda escribe:

- `translation_es`: la traducción más útil en español natural, en minúsculas y sin
  punto final. Si tiene dos sentidos frecuentes, sepáralos con ` · `
  (`terrible miedo · pavor`). Si la fila ya trae traducción, **se respeta**: no la
  cambies salvo que esté mal, y en ese caso pregúntalo.
- `pronunciation_es`: fonética aproximada al español, minúsculas, tilde en la sílaba
  fuerte, como en el resto de la app: `dread` → `dred`, `foresee` → `forsí`,
  `farther afield` → `fárder afíld`. Convenciones: `th`→`d`/`z`, `h`→`j`, `w`→`u`,
  `-tion`→`-shon`. También se respeta si ya existe.
- `vocab_notes`: la ficha, en español, **texto plano** (sin markdown: la tarjeta no lo
  interpreta) y en 3 líneas como máximo, 40–80 palabras en total:

  ```
  Significado: sensación intensa de miedo ante algo que va a pasar.
  Uso: "a feeling of dread", "I dread Mondays" (verbo: temer que llegue algo). Más fuerte que fear.
  Curiosidad: dreadlocks viene de aquí; el pelo "temible" de los rastas.
  ```

  - `Significado:` qué es, en el sentido del contexto.
  - `Uso:` con qué palabras suele ir, registro (formal, coloquial, literario),
    diferencias con sinónimos o falsos amigos, y un ejemplo corto entre comillas.
  - `Curiosidad:` solo si hay algo que de verdad ayude a recordarla (etimología,
    falso amigo, error típico de hispanohablantes, pronunciación engañosa). Si no hay
    nada interesante, omite la línea; no la rellenes.
  - Nada inventado: si no estás seguro de una etimología, no la pongas.

Muestra una tabla por tanda y espera aprobación:

| # | Palabra | Frase de origen | Traducción | Pronunciación | Ficha |
|---|---|---|---|---|---|

**No escribas nada sin un "sí" explícito.** Si pide cambios, rehace solo esas filas y
vuelve a mostrar la tabla completa.

### 4. Guardar la tanda

Un script en el scratchpad (`enrich_vocab.sql`), una sola sentencia `UPDATE`. Las
comillas simples se escapan duplicándolas (`''`); los saltos de línea de la ficha van
tal cual dentro del literal. Escribe el archivo con saltos `
` (en Windows, Python con
`newline=''`): un `
` quedaría guardado dentro de la ficha. Tras guardar, comprueba que
ninguna ficha tenga `chr(13)`. El filtro por playlist y `enriched_at IS NULL` impide
tocar filas ajenas o ya completadas:

```sql
UPDATE phrases ph SET
  translation_es   = CASE WHEN COALESCE(ph.translation_es, '')   = '' THEN v.tr ELSE ph.translation_es END,
  pronunciation_es = CASE WHEN COALESCE(ph.pronunciation_es, '') = '' THEN v.pr ELSE ph.pronunciation_es END,
  vocab_notes      = v.notes,
  enriched_at      = NOW(),
  updated_at       = NOW()
FROM (VALUES
  (1720, 'apenas disimulado', 'wid bérli konsíld', 'Significado: ...
Uso: ...'),
  (1545, '...', '...', '...')
) AS v(id, tr, pr, notes), phrase_groups g
WHERE ph.id = v.id AND g.id = ph.phrase_group_id
  AND g.phrase_playlist_id = <vocab_id> AND ph.enriched_at IS NULL;
```

```bash
go run ./cmd/dbq "@<scratchpad>/enrich_vocab.sql"
```

Verifica que se actualizaron tantas filas como palabras tenía la tanda:

```sql
SELECT count(*) FROM phrases WHERE id IN (<ids>) AND enriched_at IS NOT NULL AND COALESCE(vocab_notes, '') <> '';
```

Si quedan pendientes, di cuántas faltan y ofrece la siguiente tanda (vuelve al paso 3).

### 5. Reportar

Base de datos usada, cuántas expresiones se sincronizaron desde el padre, cuántas
palabras se completaron, cuántas siguen con `enriched_at` en `NULL`, y el enlace
`/phrases/<vocab_id>`.

## Lo que este skill no hace

- No genera audio: la app genera el Polly de cada palabra al reproducirla.
- No borra ni reordena palabras, ni toca el playlist padre.
- No vuelve a procesar palabras ya completadas. Para rehacer una, el usuario debe
  pedirlo explícitamente; entonces pon su `enriched_at` en `NULL` (solo esa fila) y
  procésala como pendiente.
