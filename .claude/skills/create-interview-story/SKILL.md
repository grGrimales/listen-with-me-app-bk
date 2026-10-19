---
name: create-interview-story
description: Redacta una historia nueva para practicar entrevistas de trabajo en inglés — un diálogo o respuesta a una pregunta de entrevista — y la inserta en un story playlist. Pregunta la pregunta de entrevista, el nivel CEFR, cuántos párrafos, cuántas palabras por párrafo, a qué playlist va y si hay algún comentario extra; muestra la historia completa en inglés con su traducción al español para que el usuario la apruebe o pida ajustes, y recién entonces la inserta. Úsalo cuando el usuario pida crear/armar una historia de entrevista, preparar una respuesta para una entrevista, o agregar una historia a un playlist de entrevistas.
user-invocable: true
---

# Crear una historia para practicar entrevistas

Argumento recibido: `$ARGUMENTS`

```
/create-interview-story                                              → pregunta todo
/create-interview-story What's your experience with Kafka?           → la pregunta ya viene dada
/create-interview-story Háblame de un conflicto con un compañero     → idem, en español
```

Si `$ARGUMENTS` trae texto, tómalo como la **pregunta de entrevista** y no la
vuelvas a preguntar. Lo demás siempre se pregunta.

El resultado es una historia normal de la app: párrafos en inglés, cada uno con su
traducción al español y su pronunciación, metida en un story playlist. Se lee en
`/stories/{id}`, entra en el Modo Zen y funciona con el modo recall (español
primero, revelar el inglés párrafo por párrafo).

## Paso 0 — a qué base apuntas

El `.env` alterna entre test y producción. Compruébalo **antes** de escribir nada
y dilo en tu respuesta:

```bash
grep -oE '^DATABASE_URL=.*@[^/?]+' backend/.env | sed 's/.*@//'
```

> Las rutas de este documento son relativas a la raíz del proyecto (la carpeta con
> `backend/` y `frontend/`). Si tu sesión ya arrancó dentro de `backend/`, quítales
> ese prefijo. Todos los `go run` van desde `backend/`.

## Paso 1 — los playlists disponibles

Necesitas la lista real para poder ofrecerla como opción:

```bash
go run ./cmd/dbq "SELECT p.id, p.name, count(ps.story_id) AS historias FROM playlists p LEFT JOIN playlist_stories ps ON ps.playlist_id = p.id GROUP BY p.id ORDER BY p.id"
```

## Paso 2 — preguntar

Usa **AskUserQuestion**, en dos tandas (el máximo es 4 preguntas por llamada).
Ofrece opciones concretas; el usuario siempre puede escribir otra cosa.

**Tanda 1**

| Pregunta | Header | Opciones sugeridas |
|---|---|---|
| ¿A qué playlist de historias la agrego? | Playlist | los 3-4 playlists más plausibles del paso 1, con su nº de historias. Pon primero el de entrevistas si lo hay |
| ¿En qué nivel de inglés la redacto? | Nivel | `B1 — intermedio`, `B2 — intermedio alto (recomendado)`, `C1 — avanzado`, `A2 — básico`. Describe qué cambia: vocabulario, subordinadas, phrasal verbs |
| ¿Cuántos párrafos? | Párrafos | `4`, `6`, `8`. Explica que el primero suele ser la pregunta del entrevistador |
| ¿Cuántas palabras por párrafo, aproximadamente? | Longitud | `12-20 (frases cortas, fáciles de repetir)`, `20-35 (recomendado)`, `35-50 (respuestas largas)` |

**Tanda 2** — y solo si falta:

- La **pregunta de entrevista**, si `$ARGUMENTS` vino vacío. Es texto libre:
  pregúntala en el chat, no con AskUserQuestion.
- Un **comentario abierto** para la redacción: pregunta siempre, en el chat,
  dejando claro que puede quedar vacío. Ejemplos de lo que suele poner el usuario:
  qué tecnologías mencionar, qué empresa o proyecto usar como referencia, qué tono
  (más formal, más conversacional), qué quiere practicar.

No inventes las respuestas ni asumas valores por defecto: **espera** a que conteste.

## Paso 3 — redactar la historia

Formato: **un diálogo de entrevista**, igual que el resto del playlist. El párrafo
1 es la pregunta del entrevistador; los demás son la respuesta del candidato,
partida en trozos del largo pedido. Cada párrafo es una unidad que se pueda
escuchar y repetir suelta.

Reglas de redacción:

- **Inglés hablado, no escrito.** Contracciones (`I've`, `that's`, `we were`),
  conectores de conversación (`So`, `Actually`, `In my case`). Nada de prosa de
  ensayo.
- **Primera persona y creíble.** Si el usuario dio contexto en el comentario
  abierto (empresa, stack, proyecto), úsalo; si no, mantenlo genérico pero
  concreto — un ejemplo real vale más que una generalidad.
- **Respeta el nivel.** A2/B1: frases cortas, presente y pasado simple, vocabulario
  común. B2: subordinadas, pasado perfecto, phrasal verbs, algún término técnico.
  C1: matices, voz pasiva, idioms moderados.
- **Respeta la longitud** pedida por párrafo; una diferencia de ±20 % está bien,
  el doble no.
- **Estructura la respuesta.** Para preguntas de comportamiento, el arco
  situación → tarea → acción → resultado, sin nombrarlo. Para preguntas técnicas,
  definición → cómo lo usaste → un detalle concreto → qué aprendiste.
- **`translation_es`**: español natural y completo, no literal. Mayúscula inicial y
  punto final. Los términos técnicos que un hispanohablante diría en inglés se
  dejan en inglés (`pipeline`, `cold start`, `deploy`).
- **`pronunciation_es`**: fonética aproximada al español, en minúsculas, **con la
  misma puntuación que el inglés** (`?`, `.`, `,`) — se lee en voz alta y los signos
  marcan la entonación. Convenciones del resto de la base: `th`→`d`/`z`, `h`→`j`,
  `w`→`u`, `oo`→`u`, `-tion`→`-shon`, `v` se mantiene; siglas deletreadas (`AWS` →
  `ei dábliu es`). Se genera siempre aunque no se muestre en la revisión: el lector
  la pone justo debajo del inglés al revelar un párrafo.
- **`title`**: la pregunta de entrevista en inglés, en una línea corta y sin signos
  de interrogación de más. Es lo que se ve en la biblioteca y en el playlist.

## Paso 4 — mostrar para aprobación

Antes de tocar la base, muestra la historia **completa** en el chat:

```
**Título:** Tell me about a time you improved a slow pipeline
**Playlist:** 18 · Entrevista en inglés   **Nivel:** B2   **Párrafos:** 6 (~28 palabras)

**01** Can you tell me about a time you had to improve the performance of a pipeline?
   ¿Puedes contarme de alguna vez que tuviste que mejorar el rendimiento de un pipeline?

**02** Yes, that happened quite often at my last job, where some jobs were taking hours.
   Sí, eso pasaba bastante seguido en mi último trabajo, donde algunos jobs tardaban horas.
```

Inglés y español, párrafo por párrafo. **La pronunciación no se muestra** — existe
en el JSON, pero ensucia la revisión.

Después pregunta explícitamente si la aprueba o quiere ajustes. Si pide cambios:
reescribe, **vuelve a mostrar la historia entera** y vuelve a preguntar. Repite las
veces que haga falta. No insertes nada hasta un sí claro.

## Paso 5 — insertar

Escribe el JSON en el scratchpad (nunca dentro del proyecto) y pásalo por
`cmd/create_story`, que valida contra la base y hace la inserción en una sola
transacción (`CreateFull` + `AddStoryToPlaylist`):

```json
{
  "title": "Tell me about a time you improved a slow pipeline",
  "level": "B2",
  "category_id": 1,
  "author": "<nombre del playlist destino>",
  "paragraphs": [
    {
      "content": "Can you tell me about a time you had to improve the performance of a pipeline?",
      "pronunciation_es": "kan yu tel mi abáut a táim yu jad tu imprúv de perfórmans of a páiplain?",
      "translations": [{"language": "es", "content": "¿Puedes contarme de alguna vez que tuviste que mejorar el rendimiento de un pipeline?"}]
    }
  ]
}
```

`category_id` es `1` (General) salvo que el usuario pida otra; `author` es el
nombre del playlist destino. No pongas `position` (lo renumera el comando), ni
`audio_url`, ni `voices`.

```bash
go run ./cmd/create_story -file <scratchpad>/historia.json -playlist <PL>          # valida
go run ./cmd/create_story -file <scratchpad>/historia.json -playlist <PL> -apply   # inserta
```

El dry-run es obligatorio: imprime título, nivel, categoría, nº de párrafos,
palabras, playlist destino y dueño, y falla con la lista de problemas si algo no
cuadra (nivel inválido, categoría inexistente, párrafo sin traducción `es`,
título repetido en ese playlist). Arregla el JSON y repite hasta que valide.

## Paso 6 — verificar y reportar

```bash
go run ./cmd/dbq "SELECT s.id, s.title, s.level, s.status, count(p.id) AS parrafos, count(NULLIF(p.pronunciation_es,'')) AS con_pron, count(t.id) AS traducciones FROM stories s LEFT JOIN paragraphs p ON p.story_id = s.id LEFT JOIN paragraph_translations t ON t.paragraph_id = p.id AND t.language = 'es' WHERE s.id = <ID> GROUP BY s.id"
```

`parrafos`, `con_pron` y `traducciones` deben coincidir con lo que aprobó el
usuario. Repórtale: base de datos usada, id de la historia, playlist, y el enlace
`/stories/<ID>`. Recuérdale que **la historia nace sin audio**: se lee y se usa en
modo recall, pero para escucharla hay que generar el audio de cada párrafo desde
`/admin/stories/<ID>/assets` (ElevenLabs, consume créditos).

## Lo que este skill no hace

- **No genera audio ni word timestamps.** Es otro flujo
  (`POST /api/paragraphs/{id}/audio/generate`) y consume créditos de ElevenLabs:
  solo si el usuario lo pide expresamente.
- **No crea playlists.** Si el que quiere no existe, dilo y pregunta; crearlo es
  `POST /api/playlists` desde la app.
- **No escribe en `story_voices`.** Una voz de historia completa haría que el
  lector y el Modo Zen la eligieran por defecto, desactivando el modo por párrafo
  y con él el resaltado por palabra. El comando lo rechaza si lo intentas.
- **No edita historias existentes.** `PUT /api/stories/{id}` borra y recrea los
  párrafos, y la cascada se lleva los word timestamps. Para cambiar una historia ya
  creada, pregunta antes.
