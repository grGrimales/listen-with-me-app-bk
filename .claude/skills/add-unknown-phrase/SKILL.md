---
name: add-unknown-phrase
description: Agrega al phrase playlist "desconocidas" una palabra o expresión en inglés que el usuario leyó/escuchó y no conoce. Por cada expresión genera 3 frases cortas y útiles que la usan (1 en B1, 1 en B2, 1 en C1) con traducción y pronunciación, las muestra para aprobación y solo entonces las inserta. Los grupos se llaman 1, 2, 3… y tienen exactamente 50 frases; al llenarse uno, las siguientes van al siguiente. Úsalo cuando el usuario pase una palabra/frase desconocida, diga "agrega esta frase a desconocidas", pegue expresiones con `*`, o apunte a un archivo de texto con expresiones (p. ej. `palabras-desconocidas.txt`, una por línea). Con el argumento `power-of-now` saca las expresiones de las capturas del libro en `backend/power-of-now/`. Toda expresión agregada o descartada queda en `backend/registro-desconocidas.tsv`, el registro único que evita repetirlas.
user-invocable: true
---

# Agregar expresiones desconocidas

Argumento recibido: `$ARGUMENTS`

```
/add-unknown-phrase how to tell at a glance
/add-unknown-phrase * how to tell at a glance * It is worth it
/add-unknown-phrase palabras-desconocidas.txt    → lee las expresiones del archivo
/add-unknown-phrase palabras-desconocidas.txt 8  → idem, tanda de 8
/add-unknown-phrase power-of-now                 → capturas del libro (ver abajo)
/add-unknown-phrase                              → pregunta qué expresión
```

Una o varias expresiones, separadas por `*`, `;` o saltos de línea. Quita viñetas
y espacios sobrantes. Si no llega nada, pregunta cuál es la expresión.

Cómo se interpreta el argumento, en este orden:

1. Exactamente `power-of-now` (más un número opcional) → **modo `power-of-now`**.
2. Apunta a un archivo que existe → **modo archivo**.
3. Cualquier otra cosa → son las expresiones mismas.

## Registro único: `registro-desconocidas.tsv`

Hay **un solo archivo** para llevar la cuenta de todas las expresiones, vengan de
donde vengan (mensaje, archivo de texto o capturas de `power-of-now`):
`backend/registro-desconocidas.tsv`. Es lo que evita repetir una expresión por
error. No hay ningún otro listado; no crees TSV paralelos.

```
expresion	estado	fuente	captura	contexto
farther afield	agregada 2026-10-16	power-of-now	IMG-20261016-WA0003.jpg	placing copies of the book in spiritual bookstores farther afield
spotlight	pendiente	power-of-now	Screenshot_20261016-143359_Kindle.png	any teaching that puts the spotlight of attention on the workings of the ego
tenant	agregada 2026-10-17	palabras-desconocidas.txt		
```

- `expresion`: forma base en minúsculas, sin puntuación (`nun`, no `nuns,`;
  `to give birth to`, no `gave birth to it`). Es la clave de la deduplicación.
- `estado`:
  - `pendiente`: detectada pero aún sin frases. Solo la usa `power-of-now`, porque
    leer las capturas es caro y no conviene repetirlo.
  - `agregada AAAA-MM-DD`: sus 3 frases ya están en la base.
  - `descartada AAAA-MM-DD`: el usuario la rechazó.
- `fuente`: `mensaje`, `power-of-now` o el nombre del archivo de donde salió.
- `captura`: solo en `power-of-now`, la imagen donde aparece. Vacío en el resto.
- `contexto`: opcional; la oración original si se conoce.

Las columnas van separadas por tabuladores reales; las vacías se dejan vacías (no
se omiten). Siempre 5 columnas.

**Antes de proponer frases** (en cualquier modo), comprueba cada expresión contra
el registro:

```bash
awk -F'\t' -v e="<expresion en minúsculas>" 'NR>1 && tolower($(1))==e' registro-desconocidas.tsv
```

(no uses `grep -P`: falla en el Git Bash de Windows). En `awk` escribe los campos
como `$(1)`, `$(2)`, nunca con el dólar pegado al número: al cargar el skill esa
forma se sustituye por las palabras del argumento y el comando llega roto. Si
editas este archivo, mantén esa forma (tampoco la escribas en el texto
explicativo). La comparación es literal,
así que además fíjate a ojo en variantes de la misma expresión (`hand over` /
`handed over`, `worth it` / `it is worth it`) mirando el registro entero.

| Si ya está como… | Qué hacer |
|---|---|
| `agregada` | sácala de la tanda y dilo en la respuesta |
| `descartada` | pregunta si ahora sí la quiere |
| `pendiente` | es la misma fila: úsala, no añadas otra |

El paso 2 (duplicados en la base) sigue haciéndose: cubre lo que se insertó sin
pasar por el registro.

**Después de insertar** (paso 6 verificado):

- Las filas que ya existían (`pendiente`) cambian de estado en su sitio a
  `agregada <hoy>` o `descartada <hoy>`.
- Las expresiones nuevas (mensaje o archivo) se añaden al final, una fila por
  expresión: `agregada <hoy>` las insertadas y `descartada <hoy>` las que el
  usuario rechazó en la tabla de aprobación.
- Si la inserción falló, no escribas nada.

```bash
# añadir una fila nueva (5 columnas, \n como fin de línea)
printf '%s\t%s\t%s\t%s\t%s\n' "tenant" "agregada 2026-10-17" palabras-desconocidas.txt "" "" >> registro-desconocidas.tsv

# cambiar de estado filas pendientes
awk -F'\t' -v OFS='\t' -v hoy="2026-10-17" '
  BEGIN { split("relative|meaningless", a, "|"); for (i in a) s[a[i]] }
  NR>1 && ($(1) in s) && $(2)=="pendiente" { $(2) = "agregada " hoy } { print }
' registro-desconocidas.tsv > registro.tmp && mv registro.tmp registro-desconocidas.tsv
```

## Modo archivo

```
/add-unknown-phrase palabras-desconocidas.txt
/add-unknown-phrase palabras-desconocidas.txt 8
```

Si el argumento (sin comillas y sin un número final opcional) es la ruta de un
archivo que existe, las expresiones salen de ese archivo. La ruta se resuelve
desde `backend/` o como absoluta; si no existe tal cual, prueba añadiéndole
`.txt` (`palabras-desconocidas` → `palabras-desconocidas.txt`). Si el argumento
parece un nombre de archivo pero no existe, **pregunta** en vez de tomarlo como
expresión.

Formato del archivo, el que el usuario escribe a mano:

- Una expresión por línea; también vale separar con `*` o `;`.
- Se ignoran líneas vacías y las que empiezan por `#`.
- Quita `\r` (el archivo suele venir con saltos CRLF de Windows), viñetas
  (`-`, `*`, `•`, `1.`) y espacios sobrantes.
- Si una línea trae contexto tras ` | ` (`tenant | the tenant didn't pay`), lo de
  la izquierda es la expresión y lo de la derecha va a la columna `contexto` del
  registro.
- Las líneas que empiezan por `listo - ` o `descartada - ` ya están procesadas:
  las marca este skill (ver paso 4) y se saltan.

```
listo - tenant
listo - haven't accounted for
descartada - nun
spotlight
```

Leer solo las pendientes, sin que el CRLF ensucie la última palabra:

```bash
tr -d '\r' < palabras-desconocidas.txt | sed 's/^[[:space:]]*//; s/[[:space:]]*$//' | grep -v '^#' | grep -v '^$' | grep -viE '^(listo|descartada) - '
```

Pasos:

1. **Pendientes** = las líneas sin marca, en su orden, que **no** están en el
   registro. Si una línea sin marca ya figura en el registro (agregada o
   descartada por otra vía), no la propongas: márcala como en el paso 4 y dilo.
2. **Tanda**: hasta **5** pendientes; un número tras el nombre del archivo cambia
   el tamaño. Sigue con el procedimiento normal (pasos 1–7).
3. **Registro**: en este modo la columna `expresion` lleva la línea **tal como está
   en el archivo** (en minúsculas, sin la marca), aunque no sea la forma base
   (`haven't accounted for`, no `to account for`), para que las dos cosas
   coincidan.
4. **Marcar el archivo** tras insertar (paso 6 verificado) y anotar el registro:
   antepón `listo - ` a cada línea insertada y `descartada - ` a cada una que el
   usuario rechazó. Solo cambia esas líneas: no reordenes, no borres y no toques
   el resto del texto. Si la inserción falló, no marques nada.

   ```bash
   sed -i -E "s/^([[:space:]]*)(tenant|haven't accounted for)([[:space:]]*)$/listo - \\2/" palabras-desconocidas.txt
   ```

   La expresión va tal cual en el patrón; si lleva caracteres especiales de regex
   (`.`, `(`, `?`, `*`, `/`…), escápalos. Después vuelve a leer el archivo y
   comprueba que esas líneas, y solo esas, quedaron marcadas.
5. En el reporte final di cuántas líneas del archivo siguen sin marcar.

## Modo `power-of-now`

```
/add-unknown-phrase power-of-now
/add-unknown-phrase power-of-now 8
```

Si el argumento es `power-of-now`, las expresiones salen de las capturas del libro
*The Power of Now* que el usuario dejó en `backend/power-of-now/`. El playlist sigue
siendo `desconocidas` y el listado es el mismo registro único, con
`fuente = power-of-now`.

| Qué | Dónde |
|---|---|
| Zips de Google Drive | `power-of-now/*.zip` |
| Capturas | `power-of-now/capturas/` (`.png`, `.jpg`) |
| Expresiones marcadas | `registro-desconocidas.tsv`, filas con `fuente = power-of-now` |

Zips y capturas están en `.gitignore` (copyright del libro); el registro sí se sube
a GitHub.

Pasos previos:

1. **Extraer zips nuevos** sin sobrescribir:
   `unzip -n -q <zip> -d power-of-now/capturas`. No borres los zips.
2. **Capturas nuevas** = las de `capturas/` cuyo nombre no aparece en la columna
   `captura` del registro. Ábrelas con Read en orden de lectura (posición/porcentaje
   del pie de página) y saca los fragmentos resaltados (azul o amarillo). Un
   resaltado que corta una frase a la mitad (`handed over had`) se reduce a la
   expresión que tiene sentido (`handed over`).
3. **Deduplicar** cada expresión contra el registro entero (cualquier fuente). Si
   ya está, no se añade. Las nuevas van al final como
   `<expresion>	pendiente	power-of-now	<captura>	<contexto>`.
4. **Tanda**: hasta **5** filas `pendiente` con `fuente = power-of-now`, en el
   orden del archivo; un número tras `power-of-now` cambia el tamaño. Sigue con el
   procedimiento normal (pasos 1–7). Cuando la expresión del libro es ambigua o
   probablemente se resaltó por error (p. ej. una palabra muy básica), pregúntalo
   en la tabla de aprobación.
5. **Tras insertar**, cambia el estado de esas filas como dice el registro único.
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
`/phrases/<playlist_id>`. Confirma qué filas añadiste a `registro-desconocidas.tsv`
y, si las expresiones salieron de un archivo o de `power-of-now`, cuántas quedan
pendientes allí.

## Lo que este skill no hace

- **No genera audio.** `polly_audio_url_female` queda en `NULL`; la app genera el
  audio ElevenLabs al reproducir la tarjeta (`POST /api/phrases/{id}/audio/generate`)
  y consume créditos solo entonces.
- No toca otros playlists ni reordena frases existentes.
