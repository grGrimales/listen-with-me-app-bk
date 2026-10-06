// Comando de un solo uso: convierte un playlist de FRASES en historias.
//
// Cada grupo del playlist de frases se vuelve una historia (el título es el nombre
// del grupo) y cada frase del grupo se vuelve un párrafo de esa historia, reutilizando
// el MP3 que la frase ya tiene y copiando sus word timestamps. Las historias se meten
// en un story playlist para poder leerlas y escucharlas como cualquier otra.
//
// No se crea ninguna fila en story_voices a propósito: una voz de historia completa
// haría que el lector y el modo Zen la eligieran por defecto, desactivando el modo
// por párrafo y con él el resaltado por palabra y los segmentos de vocabulario.
//
//	go run ./cmd/phrases_to_stories -playlist 21 -name "Entrevista en inglés"
//	go run ./cmd/phrases_to_stories -playlist 21 -name "Entrevista en inglés" -apply
//
// Con -sync no crea nada: refresca el audio, los word timestamps y la pronunciación
// de los párrafos de las historias ya creadas a partir del estado actual de las frases
// (útil cuando se genera después el audio de las frases que no lo tenían).
//
//	go run ./cmd/phrases_to_stories -playlist 21 -name "Entrevista en inglés" -sync -apply
//
// Sin -apply solo informa: no escribe nada.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"listen-with-me/backend/internal/model"
	"listen-with-me/backend/internal/repository"
)

// phrase es una frase de origen con todo lo que se necesita para armar un párrafo.
type phrase struct {
	id            int
	position      int
	text          string
	translation   string
	pronunciation string
	audioURL      string
	words         []model.WordTimestamp
}

// group es un grupo de frases, que se convierte en una historia.
type group struct {
	id      int
	name    string
	phrases []phrase
}

func (g group) withAudio() int {
	n := 0
	for _, p := range g.phrases {
		if p.audioURL != "" {
			n++
		}
	}
	return n
}

func (g group) withWords() int {
	n := 0
	for _, p := range g.phrases {
		if len(p.words) > 0 {
			n++
		}
	}
	return n
}

func main() {
	phrasePlaylistID := flag.Int("playlist", 0, "id del playlist de frases de origen (tabla phrase_playlists)")
	name := flag.String("name", "", "nombre del story playlist destino (se crea si no existe)")
	storyPlaylistID := flag.Int("story-playlist", 0, "id de un story playlist existente; si se pasa, ignora -name")
	level := flag.String("level", "B2", "nivel CEFR de las historias (A1..C2)")
	categoryID := flag.Int("category", 1, "id de la categoría de las historias")
	author := flag.String("author", "", "autor de las historias (por defecto, el nombre del story playlist)")
	sync := flag.Bool("sync", false, "no crea historias: refresca audio, timestamps y pronunciación de las ya creadas")
	apply := flag.Bool("apply", false, "ejecuta los cambios; sin este flag solo simula")
	flag.Parse()

	if *phrasePlaylistID == 0 || (*name == "" && *storyPlaylistID == 0) {
		flag.Usage()
		os.Exit(2)
	}

	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	repo := repository.NewStoryRepo(db)

	// --- Origen: el playlist de frases y sus grupos ---

	var srcName, ownerID, language string
	if err := db.QueryRow(
		`SELECT name, user_id, language FROM phrase_playlists WHERE id = $1`, *phrasePlaylistID,
	).Scan(&srcName, &ownerID, &language); err == sql.ErrNoRows {
		log.Fatalf("no existe el playlist de frases %d", *phrasePlaylistID)
	} else if err != nil {
		log.Fatal(err)
	}

	groups, err := loadGroups(db, *phrasePlaylistID)
	if err != nil {
		log.Fatal(err)
	}
	if len(groups) == 0 {
		log.Fatalf("el playlist de frases %d (%q) no tiene grupos con frases", *phrasePlaylistID, srcName)
	}

	fmt.Printf("Origen: playlist de frases %d %q (%s), %d grupos\n", *phrasePlaylistID, srcName, language, len(groups))

	// --- Destino: el story playlist ---

	targetID, targetName, created, err := resolveStoryPlaylist(db, repo, ownerID, srcName, *name, *storyPlaylistID, *apply)
	if err != nil {
		log.Fatal(err)
	}
	switch {
	case *storyPlaylistID > 0:
		fmt.Printf("Destino: story playlist %d %q (existente)\n", targetID, targetName)
	case created:
		fmt.Printf("Destino: story playlist %d %q (creado)\n", targetID, targetName)
	case targetID > 0:
		fmt.Printf("Destino: story playlist %d %q (ya existía, se reutiliza)\n", targetID, targetName)
	default:
		fmt.Printf("Destino: story playlist %q (se crearía)\n", targetName)
	}

	// Las historias de este playlist, por título, para saltar lo ya convertido.
	existing, err := storiesByTitle(db, targetID)
	if err != nil {
		log.Fatal(err)
	}

	if *sync {
		syncMode(db, repo, groups, existing, *apply)
		return
	}

	storyAuthor := *author
	if storyAuthor == "" {
		storyAuthor = targetName
	}

	// --- Creación ---

	totalParas, totalAudio, totalWords, createdStories, skipped := 0, 0, 0, 0, 0
	for _, g := range groups {
		if storyID, ok := existing[strings.ToLower(g.name)]; ok {
			fmt.Printf("  skip  %-45s ya existe como historia %d\n", g.name, storyID)
			skipped++
			continue
		}

		fmt.Printf("  %-45s %d párrafos (%d con audio, %d con word-ts)\n",
			g.name, len(g.phrases), g.withAudio(), g.withWords())
		totalParas += len(g.phrases)
		totalAudio += g.withAudio()
		totalWords += g.withWords()

		if !*apply {
			continue
		}

		req := &model.CreateFullStoryRequest{
			Title:      g.name,
			Level:      *level,
			CategoryID: *categoryID,
			Author:     storyAuthor,
			Paragraphs: make([]model.FullParagraph, 0, len(g.phrases)),
			// Voices vacío: ver la nota de cabecera.
		}
		for i, p := range g.phrases {
			req.Paragraphs = append(req.Paragraphs, model.FullParagraph{
				Position:        i,
				Content:         p.text,
				PronunciationES: p.pronunciation,
				AudioURL:        p.audioURL,
				Translations: []model.CreateTranslationRequest{
					{Language: "es", Content: p.translation},
				},
			})
		}

		story, err := repo.CreateFull(req)
		if err != nil {
			log.Fatalf("crear la historia %q: %v", g.name, err)
		}

		// Los word timestamps no pasan por CreateFull: se escriben por párrafo, y la
		// posición es lo que une cada párrafo recién creado con su frase de origen.
		paraIDByPos := map[int]int{}
		for _, p := range story.Paragraphs {
			paraIDByPos[p.Position] = p.ID
		}
		for i, p := range g.phrases {
			if len(p.words) == 0 {
				continue
			}
			paraID, ok := paraIDByPos[i]
			if !ok {
				continue
			}
			if err := repo.SaveParagraphWordTimestamps(paraID, p.words); err != nil {
				log.Fatalf("timestamps del párrafo %d (historia %q): %v", paraID, g.name, err)
			}
		}

		if err := repo.AddStoryToPlaylist(targetID, story.ID); err != nil {
			log.Fatalf("meter la historia %d en el playlist %d: %v", story.ID, targetID, err)
		}
		fmt.Printf("        → historia %d creada y añadida al playlist\n", story.ID)
		createdStories++
	}

	fmt.Println()
	if *apply {
		fmt.Printf("Listo: %d historias creadas, %d saltadas, en el story playlist %d %q.\n",
			createdStories, skipped, targetID, targetName)
		return
	}
	fmt.Printf("Simulación: se crearían %d historias (%d saltadas) con %d párrafos, %d con audio y %d con word-ts.\n",
		len(groups)-skipped, skipped, totalParas, totalAudio, totalWords)
	fmt.Println("Volvé a correrlo con -apply para escribir.")
}

// loadGroups lee los grupos del playlist de frases con sus frases y word timestamps,
// en el orden en que se muestran en la app. Los grupos sin frases se descartan.
func loadGroups(db *sql.DB, phrasePlaylistID int) ([]group, error) {
	rows, err := db.Query(
		`SELECT g.id, g.name,
		        ph.id, ph.position, ph.text, ph.translation_es,
		        COALESCE(ph.pronunciation_es, ''), COALESCE(ph.polly_audio_url_female, '')
		 FROM phrase_groups g
		 JOIN phrases ph ON ph.phrase_group_id = g.id
		 WHERE g.phrase_playlist_id = $1
		 ORDER BY g.position, g.id, ph.position, ph.id`, phrasePlaylistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []group
	byID := map[int]int{} // group id → índice en groups
	for rows.Next() {
		var gID int
		var gName string
		var p phrase
		if err := rows.Scan(&gID, &gName, &p.id, &p.position, &p.text, &p.translation, &p.pronunciation, &p.audioURL); err != nil {
			return nil, err
		}
		idx, ok := byID[gID]
		if !ok {
			groups = append(groups, group{id: gID, name: strings.TrimSpace(gName)})
			idx = len(groups) - 1
			byID[gID] = idx
		}
		groups[idx].phrases = append(groups[idx].phrases, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for gi := range groups {
		for pi := range groups[gi].phrases {
			words, err := loadPhraseWords(db, groups[gi].phrases[pi].id)
			if err != nil {
				return nil, err
			}
			groups[gi].phrases[pi].words = words
		}
	}
	return groups, nil
}

func loadPhraseWords(db *sql.DB, phraseID int) ([]model.WordTimestamp, error) {
	rows, err := db.Query(
		`SELECT word_index, word, start_ms, end_ms
		 FROM phrase_word_timestamps WHERE phrase_id = $1 ORDER BY word_index`, phraseID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.WordTimestamp
	for rows.Next() {
		var w model.WordTimestamp
		if err := rows.Scan(&w.WordIndex, &w.Word, &w.StartMs, &w.EndMs); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// resolveStoryPlaylist devuelve el story playlist destino: el indicado por -story-playlist,
// uno del mismo dueño que ya se llame igual, o uno nuevo. En simulación devuelve id 0
// cuando habría que crearlo.
func resolveStoryPlaylist(db *sql.DB, repo *repository.StoryRepo, ownerID, srcName, name string, explicitID int, apply bool) (int, string, bool, error) {
	if explicitID > 0 {
		var plName, plOwner string
		if err := db.QueryRow(`SELECT name, user_id FROM playlists WHERE id = $1`, explicitID).Scan(&plName, &plOwner); err == sql.ErrNoRows {
			return 0, "", false, fmt.Errorf("no existe el story playlist %d", explicitID)
		} else if err != nil {
			return 0, "", false, err
		}
		if plOwner != ownerID {
			return 0, "", false, fmt.Errorf("el story playlist %d es de otro usuario que el playlist de frases", explicitID)
		}
		return explicitID, plName, false, nil
	}

	if name == "" {
		name = srcName
	}

	var id int
	err := db.QueryRow(
		`SELECT id FROM playlists WHERE user_id = $1 AND lower(name) = lower($2)`, ownerID, name,
	).Scan(&id)
	switch {
	case err == nil:
		return id, name, false, nil
	case err != sql.ErrNoRows:
		return 0, "", false, err
	}

	if !apply {
		return 0, name, false, nil
	}
	pl := &model.Playlist{
		UserID:      ownerID,
		Name:        name,
		Description: "Historias armadas desde el playlist de frases '" + srcName + "'",
	}
	if err := repo.CreatePlaylist(pl); err != nil {
		return 0, "", false, err
	}
	return pl.ID, pl.Name, true, nil
}

// storiesByTitle mapea lower(title) → story id para las historias del playlist.
func storiesByTitle(db *sql.DB, storyPlaylistID int) (map[string]int, error) {
	out := map[string]int{}
	if storyPlaylistID == 0 {
		return out, nil
	}
	rows, err := db.Query(
		`SELECT s.id, s.title
		 FROM playlist_stories ps
		 JOIN stories s ON s.id = ps.story_id
		 WHERE ps.playlist_id = $1 AND s.status != 'deleted'`, storyPlaylistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out[strings.ToLower(strings.TrimSpace(title))] = id
	}
	return out, rows.Err()
}

// syncMode refresca el audio, los word timestamps y la pronunciación de los párrafos
// de historias ya creadas a partir del estado actual de las frases. Empareja párrafo y
// frase por posición, que es como las creó este mismo comando.
func syncMode(db *sql.DB, repo *repository.StoryRepo, groups []group, existing map[string]int, apply bool) {
	fmt.Println("\nModo -sync: no se crean historias.")
	updatedAudio, updatedWords, updatedPron := 0, 0, 0

	for _, g := range groups {
		storyID, ok := existing[strings.ToLower(g.name)]
		if !ok {
			fmt.Printf("  skip  %-45s no hay historia con ese título en el playlist\n", g.name)
			continue
		}

		paras, err := paragraphsByPosition(db, storyID)
		if err != nil {
			log.Fatal(err)
		}

		for i, p := range g.phrases {
			para, ok := paras[i]
			if !ok {
				continue
			}

			if p.audioURL != "" && p.audioURL != para.audioURL {
				fmt.Printf("  %-45s párrafo %d ← audio de la frase %d\n", g.name, i, p.id)
				updatedAudio++
				if len(p.words) > 0 {
					updatedWords++
				}
				if apply {
					if _, err := db.Exec(`UPDATE paragraphs SET audio_url = $1 WHERE id = $2`, p.audioURL, para.id); err != nil {
						log.Fatalf("actualizar el audio del párrafo %d: %v", para.id, err)
					}
					if len(p.words) > 0 {
						if err := repo.SaveParagraphWordTimestamps(para.id, p.words); err != nil {
							log.Fatalf("timestamps del párrafo %d: %v", para.id, err)
						}
					}
				}
			}

			if p.pronunciation != "" && p.pronunciation != para.pronunciation {
				fmt.Printf("  %-45s párrafo %d ← pronunciación de la frase %d\n", g.name, i, p.id)
				updatedPron++
				if apply {
					if _, err := db.Exec(`UPDATE paragraphs SET pronunciation_es = $1 WHERE id = $2`, p.pronunciation, para.id); err != nil {
						log.Fatalf("actualizar la pronunciación del párrafo %d: %v", para.id, err)
					}
				}
			}
		}
	}

	fmt.Println()
	if apply {
		fmt.Printf("Listo: %d párrafos con audio actualizado, %d con word-ts reescritos, %d con pronunciación.\n",
			updatedAudio, updatedWords, updatedPron)
		return
	}
	fmt.Printf("Simulación: se actualizarían %d párrafos de audio (%d con word-ts) y %d de pronunciación. Corré con -apply para escribir.\n",
		updatedAudio, updatedWords, updatedPron)
}

type paragraphRef struct {
	id            int
	audioURL      string
	pronunciation string
}

func paragraphsByPosition(db *sql.DB, storyID int) (map[int]paragraphRef, error) {
	rows, err := db.Query(
		`SELECT id, position, COALESCE(audio_url, ''), COALESCE(pronunciation_es, '')
		 FROM paragraphs WHERE story_id = $1`, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]paragraphRef{}
	for rows.Next() {
		var id, pos int
		var audio, pron string
		if err := rows.Scan(&id, &pos, &audio, &pron); err != nil {
			return nil, err
		}
		out[pos] = paragraphRef{id: id, audioURL: audio, pronunciation: pron}
	}
	return out, rows.Err()
}
