---
name: translate-story-phrases
description: Rellena translation_es y pronunciation_es vacíos en las frases de los phrase playlists generados desde historias ("from stories"). Acepta como argumento la URL o el id de un playlist para trabajar solo ese; sin argumento, inventaría todos los que tengan huecos. Genera un script SQL, valida que solo toque esas filas, corrige y revalida hasta que el alcance esté limpio, y recién entonces lo ejecuta. Además reordena las frases de cada grupo para que sigan el orden del texto de la historia. Úsalo cuando el usuario pida traducir/completar frases guardadas desde historias, o cuando aparezcan tarjetas sin traducción ni pronunciación en /phrases/from-stories.
user-invocable: true
---

# Traducir frases guardadas desde historias

Argumento recibido: `$ARGUMENTS`

```
/translate-story-phrases https://listen-with-me-app-fe.vercel.app/phrases/10
/translate-story-phrases /phrases/10
/translate-story-phrases 10
/translate-story-phrases                 → inventaría todos y pregunta cuál
```

Al guardar una frase desde una historia, `UpsertStoryPlaylistPhrase`
(`backend/internal/repository/story.go`) inserta `translation_es` y `pronunciation_es`
como `''` a propósito: solo persiste el texto y el recorte de audio de la historia.
Este skill rellena esos huecos.

## Alcance: solo playlists "from stories"

| Tipo de phrase playlist | Cómo se reconoce | ¿Entra? |
|---|---|---|
| Generado desde historias | `story_playlist_id IS NOT NULL` | **Sí** |
| Normal (importado a mano) | `story_playlist_id IS NULL AND parent_playlist_id IS NULL` | No |
| Hijo de vocabulario (`📗 Vocab · …`) | `parent_playlist_id IS NOT NULL` | No |

Son los que el usuario ve en `/phrases/from-stories` (endpoint
`GET /api/story-phrase-playlists`). Sus frases además traen
`source_story_id` y `source_audio_url` poblados.

**Nunca** ensanches el alcance a los otros dos tipos, aunque tengan huecos: sus
traducciones las escribe el usuario al importar.

## Herramienta de base de datos

`backend/cmd/dbq` ejecuta SQL ad-hoc. Siempre desde `backend/`:

```bash
go run ./cmd/dbq "SELECT ..."          # SQL inline
go run ./cmd/dbq "@/ruta/script.sql"   # SQL desde archivo
```

Usa `DATABASE_URL` de `backend/.env`. Con `DB_TARGET=prod` lee la línea
`#DATABASE_URL=` comentada. Los `UPDATE/INSERT/DELETE` reportan
`(N rows affected)`; los `SELECT` devuelven una línea JSON por fila.

> Las rutas de este documento (`backend/.env`, `backend/internal/…`) son relativas
> a la raíz del proyecto, la carpeta que contiene `backend/` y `frontend/`. Si tu
> sesión ya arrancó dentro de `backend/`, quítales ese prefijo.

**Paso 0 — confirma a qué base apuntas antes de escribir nada** y dilo en tu
respuesta (el `.env` alterna entre test y producción):

```bash
grep -oE '^DATABASE_URL=.*@[^/?]+' backend/.env | sed 's/.*@//'
```

## Procedimiento

Procesa **un playlist por ejecución**: mantiene el alcance estrecho y las
traducciones coherentes con su historia.

### 1. Determinar el playlist (`<PL>`)

**Con argumento** (`/translate-story-phrases https://…/phrases/10`, `/phrases/10`,
o simplemente `10`) — saca el número con `/phrases/(\d+)`; si no hay `/phrases/`,
usa el argumento entero si es un número. Sirve igual una URL de subpágina
(`/phrases/10/manage`, `/phrases/10/vocabulary`, `/phrases/10/evaluation`): el id
es el mismo. Si no logras extraer un número — p. ej. `/phrases/zen`,
`/phrases/from-stories` — **detente y pregunta**; no adivines.

Con `<PL>` en mano, resuélvelo antes de tocar nada:

```sql
SELECT p.id, p.name, p.language, p.story_playlist_id, p.parent_playlist_id,
       count(ph.id)                                                       AS frases,
       count(*) FILTER (WHERE COALESCE(ph.translation_es,'')   = '')      AS sin_traduccion,
       count(*) FILTER (WHERE COALESCE(ph.pronunciation_es,'') = '')      AS sin_pronunciacion
FROM phrase_playlists p
LEFT JOIN phrase_groups g ON g.phrase_playlist_id = p.id
LEFT JOIN phrases ph      ON ph.phrase_group_id   = g.id
WHERE p.id = <PL>
GROUP BY p.id;
```

| Resultado | Qué hacer |
|---|---|
| 0 filas | El playlist no existe. Dilo y detente. |
| `story_playlist_id IS NULL` | **No es de historias.** Detente y dilo: es un playlist normal o un hijo de vocabulario, y sus traducciones las escribe el usuario. No lo rellenes aunque tenga huecos. |
| Sin huecos | Traducciones completas. Salta al **paso 8** (el orden puede seguir mal). |
| De historias y con huecos | Adelante al paso 2. |

**Sin argumento** — inventaría todos y pregunta cuál:

### 1b. Inventario de huecos (solo sin argumento)

```sql
SELECT p.id AS playlist_id, p.name, p.language,
       count(*)                                                            AS frases,
       count(*) FILTER (WHERE COALESCE(ph.translation_es,'')   = '')       AS sin_traduccion,
       count(*) FILTER (WHERE COALESCE(ph.pronunciation_es,'') = '')       AS sin_pronunciacion
FROM phrase_playlists p
JOIN phrase_groups g ON g.phrase_playlist_id = p.id
JOIN phrases ph      ON ph.phrase_group_id   = g.id
WHERE p.story_playlist_id IS NOT NULL
GROUP BY p.id, p.name, p.language
HAVING count(*) FILTER (WHERE COALESCE(ph.translation_es,'') = ''
                           OR COALESCE(ph.pronunciation_es,'') = '') > 0
ORDER BY p.id;
```

Muestra la tabla al usuario. Si hay varios playlists con huecos, pregunta cuál
(o confirma que los quiere todos, uno tras otro).

### 2. Filas a rellenar

Con `<PL>` = el playlist elegido:

```sql
SELECT ph.id, ph.text,
       COALESCE(ph.translation_es,'')   AS translation_es,
       COALESCE(ph.pronunciation_es,'') AS pronunciation_es,
       s.title AS historia
FROM phrases ph
JOIN phrase_groups g    ON g.id = ph.phrase_group_id
JOIN phrase_playlists p ON p.id = g.phrase_playlist_id
LEFT JOIN stories s     ON s.id = ph.source_story_id
WHERE p.story_playlist_id IS NOT NULL
  AND p.id = <PL>
  AND (COALESCE(ph.translation_es,'') = '' OR COALESCE(ph.pronunciation_es,'') = '')
ORDER BY g.position, ph.position;
```

Ojo: una fila puede tener **uno** de los dos campos ya escrito. El script de
abajo respeta lo que ya existe y solo llena lo vacío.

### 3. Escribir las traducciones

Estilo, calcado del contenido que ya hay en la base:

- **Oraciones completas** → español natural, mayúscula inicial y punto final.
  `"Lambda has to create one from scratch"` → `"Lambda tiene que crear uno desde cero."`
- **Fragmentos anidados** (sub-selecciones de otra frase; son comunes porque el
  lector deja guardar trozos solapados) → minúscula y sin punto, para que se lean
  como el trozo que son. `"through it"` → `"a través de ello"`
- **Términos técnicos** que un hispanohablante diría en inglés (handler, runtime,
  cold start, warm start) → tradúcelos y aclara entre paréntesis:
  `"función handler (la función manejadora)"`, `"un arranque en frío"`.
- **`pronunciation_es`** → fonética aproximada al español, minúsculas, **sin
  acentos ni puntuación**. Referencia real del playlist 1:
  `"There is a lamp on the table."` → `der is a lamp on de teibol`.
  Convenciones: `th`→`d`/`z`, `h`→`j`, `w`→`u`, `oo`→`u`, `-tion`→`-shon`,
  `v` se mantiene. Siglas deletreadas: `AWS` → `ei dabliu es`.
- Comillas simples dentro del texto se escapan duplicándolas (`''`).

### 4. Generar el script

Escribe el bloque de valores **una sola vez** en `values.txt` (en el scratchpad),
una fila por línea, sin coma en la última:

```
  (922,  'Claro, déjame explicártelo paso a paso.', 'shur let mi uok yu zru it'),
  (923,  'Un entorno de ejecución de Lambda pasa por tres fases.', 'a lambda eksekiushon enviroment gous zru zri feises')
```

Y genera los dos `.sql` a partir de él, para que no puedan divergir:

```bash
cat > head_update.sql <<'EOF'
UPDATE phrases ph
SET translation_es   = CASE WHEN COALESCE(ph.translation_es,'')   = '' THEN v.tr ELSE ph.translation_es   END,
    pronunciation_es = CASE WHEN COALESCE(ph.pronunciation_es,'') = '' THEN v.pr ELSE ph.pronunciation_es END,
    updated_at       = NOW()
FROM (VALUES
EOF

cat > head_dryrun.sql <<'EOF'
SELECT count(*) AS filas_que_se_actualizarian
FROM phrases ph, (VALUES
EOF

cat > tail.sql <<'EOF'
) AS v(id, tr, pr)
WHERE ph.id = v.id
  AND ph.phrase_group_id IN (SELECT id FROM phrase_groups WHERE phrase_playlist_id = <PL>)
  AND (COALESCE(ph.translation_es,'') = '' OR COALESCE(ph.pronunciation_es,'') = '');
EOF

cat head_update.sql values.txt tail.sql > fill.sql
cat head_dryrun.sql values.txt tail.sql > dryrun.sql
```

El `WHERE` es el mismo en los dos, así que **el dry-run cuenta exactamente las
filas que tocará el `UPDATE`**. Los tres candados:

1. `ph.id = v.id` — lista explícita de ids
2. `phrase_group_id ∈ (grupos de <PL>)` — nada fuera del playlist
3. al menos un campo vacío — no pisa trabajo ya hecho

Y el `CASE` preserva el campo que ya tuviera contenido. Solo se escriben
`translation_es`, `pronunciation_es` y `updated_at`: **nunca** `text`, `position`,
`source_*` ni las columnas de audio.

### 5. Validar (obligatorio, antes de ejecutar)

```bash
IDS=$(grep -oE '^ *\([0-9]+,' values.txt | tr -d ' (,' | paste -sd, -)
N_VALUES=$(grep -cE '^ *\([0-9]+,' values.txt)
DUPES=$(grep -oE '^ *\([0-9]+,' values.txt | tr -d ' (,' | sort | uniq -d)
```

```sql
SELECT
  (SELECT count(*) FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
     WHERE g.phrase_playlist_id = <PL>
       AND (COALESCE(ph.translation_es,'') = '' OR COALESCE(ph.pronunciation_es,'') = ''))  AS huecos_del_playlist,
  (SELECT count(*) FROM phrases WHERE id IN (<IDS>))                                        AS ids_que_existen,
  (SELECT count(*) FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
     WHERE ph.id IN (<IDS>) AND g.phrase_playlist_id <> <PL>)                               AS ids_fuera_del_playlist,
  (SELECT count(*) FROM phrases ph
     JOIN phrase_groups g    ON g.id = ph.phrase_group_id
     JOIN phrase_playlists p ON p.id = g.phrase_playlist_id
     WHERE ph.id IN (<IDS>) AND p.story_playlist_id IS NULL)                                AS ids_en_playlist_no_de_historia,
  (SELECT count(*) FROM phrases WHERE id IN (<IDS>)
     AND COALESCE(translation_es,'') <> '' AND COALESCE(pronunciation_es,'') <> '')          AS ids_ya_completos,
  (SELECT count(*) FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
     WHERE g.phrase_playlist_id = <PL>
       AND (COALESCE(ph.translation_es,'') = '' OR COALESCE(ph.pronunciation_es,'') = '')
       AND ph.id NOT IN (<IDS>))                                                            AS huecos_no_cubiertos;
```

Más el dry-run: `go run ./cmd/dbq "@dryrun.sql"`.

| Comprobación | Valor esperado |
|---|---|
| `$DUPES` | vacío |
| `filas_que_se_actualizarian` (dry-run) | = `$N_VALUES` |
| `ids_que_existen` | = `$N_VALUES` |
| `huecos_del_playlist` | = `$N_VALUES` |
| `ids_fuera_del_playlist` | **0** |
| `ids_en_playlist_no_de_historia` | **0** |
| `ids_ya_completos` | **0** |
| `huecos_no_cubiertos` | **0** |

Presenta esta tabla al usuario con los valores reales.

### 6. Si algo no cuadra: corregir y revalidar

**No ejecutes con una comprobación en rojo.** Diagnostica, arregla `values.txt`,
regenera los `.sql` y vuelve al paso 5. Repite hasta que todo esté en verde.

| Síntoma | Causa habitual | Arreglo |
|---|---|---|
| `dry-run > N_VALUES` | id repetido en `values.txt` | quita el duplicado |
| `ids_fuera_del_playlist > 0` | id copiado de otro playlist | bórralo de `values.txt` |
| `ids_en_playlist_no_de_historia > 0` | se coló un playlist normal o de vocabulario | bórralo; ese tipo no entra |
| `huecos_no_cubiertos > 0` | faltan filas por traducir | añádelas |
| `ids_que_existen < N_VALUES` | id inexistente (frase borrada) | quítalo |
| `ids_ya_completos > 0` | alguien la completó desde la app mientras trabajabas | quítala y reinventaría |
| error de sintaxis al parsear | comilla simple sin escapar | duplícala (`''`) |
| `dry-run = 0` con huecos pendientes | `<PL>` mal sustituido en `tail.sql` | corrige el número |

### 7. Respaldo y ejecución

Con todo en verde, guarda primero las filas tal como están (permite revertir):

```bash
go run ./cmd/dbq "SELECT id, text, COALESCE(translation_es,'') AS translation_es, COALESCE(pronunciation_es,'') AS pronunciation_es FROM phrases WHERE id IN (<IDS>)" > backup_pl<PL>.jsonl
```

Luego ejecuta y verifica:

```bash
go run ./cmd/dbq "@fill.sql"     # espera (N rows affected) = dry-run
```

```sql
-- huecos restantes en el playlist: debe dar 0
SELECT count(*) FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
WHERE g.phrase_playlist_id = <PL>
  AND (COALESCE(ph.translation_es,'') = '' OR COALESCE(ph.pronunciation_es,'') = '');

-- daño colateral: debe dar 0
SELECT count(*) FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id
WHERE g.phrase_playlist_id <> <PL> AND ph.updated_at > NOW() - INTERVAL '10 minutes';
```

Si `rows affected` no coincide con el dry-run, **dilo** y revisa antes de seguir
con otro playlist.

### 8. Ordenar las frases como en la historia

`UpsertStoryPlaylistPhrase` asigna `position` en el orden en que el usuario va
guardando las frases, así que una frase guardada más tarde queda al final de su
grupo aunque esté al principio del texto. En `/phrases/<PL>` cada grupo debe leerse
como la historia, de principio a fin. Este paso corre **siempre**, aunque no haya
habido huecos que traducir.

Clave de orden dentro de cada grupo (`phrase_group_id`):

1. `paragraphs.position` del párrafo donde aparece el texto (si el texto aparece en
   varios párrafos, gana el que tiene el mismo `audio_url` que `ph.source_audio_url`)
2. `strpos` del texto dentro de ese párrafo
3. `source_start_ms` (desempata repeticiones)
4. longitud **descendente**: la frase que contiene a otra va antes que su fragmento
   anidado (`"because in my experience the slow part is…"` → `"…the slow part"` →
   `"the slow part"`)
5. `position` anterior e `id`: estabilidad

Las frases cuyo texto no aparece en ningún párrafo (historia editada después de
guardarla) van al final de su grupo, en el orden que tenían. **No** se reordenan los
grupos: `playlist_stories` no tiene un orden propio de historias.

Escribe el CTE una vez en `reorder_cte.sql` (scratchpad) y genera desde él el
dry-run y el `UPDATE`, igual que en el paso 4:

```sql
WITH located AS (
  SELECT ph.id, ph.phrase_group_id, ph.position AS old_pos,
         loc.par_pos, loc.off, ph.source_start_ms, length(ph.text) AS len
  FROM phrases ph
  JOIN phrase_groups g ON g.id = ph.phrase_group_id
  LEFT JOIN LATERAL (
    SELECT pa.position AS par_pos, strpos(lower(pa.content), lower(ph.text)) AS off
    FROM paragraphs pa
    WHERE pa.story_id = ph.source_story_id
      AND strpos(lower(pa.content), lower(ph.text)) > 0
    ORDER BY (pa.audio_url IS NOT DISTINCT FROM ph.source_audio_url) DESC, pa.position
    LIMIT 1
  ) loc ON true
  WHERE g.phrase_playlist_id = <PL>
),
ordered AS (
  SELECT id, old_pos,
         ROW_NUMBER() OVER (
           PARTITION BY phrase_group_id
           ORDER BY par_pos NULLS LAST, off, source_start_ms NULLS LAST, len DESC, old_pos, id
         ) - 1 AS new_pos
  FROM located
)
```

```bash
PL=10                                            # el id real del playlist
sed "s/<PL>/$PL/" reorder_cte.sql > cte.sql
{ cat cte.sql; echo "SELECT count(*) AS total, count(*) FILTER (WHERE old_pos <> new_pos) AS cambian FROM ordered;"; } > reorder_dry.sql
{ cat cte.sql; echo "UPDATE phrases ph SET position = o.new_pos FROM ordered o WHERE ph.id = o.id AND ph.position <> o.new_pos;"; } > reorder_fill.sql
```

1. `go run ./cmd/dbq "@reorder_dry.sql"`. Si `cambian = 0`, el orden ya está bien:
   dilo y no escribas nada. `total` debe ser igual a `frases` del paso 1.
2. Respaldo:
   `go run ./cmd/dbq "SELECT ph.id, ph.phrase_group_id, ph.position FROM phrases ph JOIN phrase_groups g ON g.id = ph.phrase_group_id WHERE g.phrase_playlist_id = <PL>" > backup_pl<PL>_positions.jsonl`
3. `go run ./cmd/dbq "@reorder_fill.sql"`. Como la sentencia empieza por `WITH`, dbq
   la trata como consulta e imprime `(0 rows)` en vez de `rows affected`; eso es
   normal.
4. Vuelve a correr el dry-run: `cambian` debe dar **0**.

Solo se escribe `position`, y únicamente dentro de `<PL>`. Tampoco se toca
`updated_at`, así que la comprobación de daño colateral del paso 7 sigue siendo
válida. No hay un índice único sobre `(phrase_group_id, position)`, así que basta con
un solo `UPDATE`.

### 9. Reportar

Al usuario: base de datos usada, playlist e historia, filas rellenadas, ruta del
respaldo y el resultado de las dos verificaciones finales. Menciona una muestra
de 3-5 traducciones para que pueda juzgar el criterio. Del paso 8: cuántas
posiciones cambiaron, ruta del respaldo de posiciones y cuántas frases no se
encontraron en el texto, si hubo alguna.

## Lo que este skill no hace

- **No genera audio.** `polly_audio_url_female/_male` en `NULL` es normal en estas
  frases: su audio es el recorte de la historia (`source_audio_url` +
  `source_start_ms/_end_ms`), que ya funciona. Generar voz ElevenLabs es otro flujo
  (`POST /api/phrases/{id}/audio/generate`) y consume créditos: solo si lo piden.
- **No arregla el origen.** El `INSERT` con `''` en
  `backend/internal/repository/story.go` sigue igual; este skill limpia después.
