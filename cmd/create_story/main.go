// Inserta una historia completa desde un archivo JSON y la mete en un story playlist.
//
// El JSON es exactamente el cuerpo de POST /api/stories/full (model.CreateFullStoryRequest),
// así que esto es ese endpoint sin servidor ni login de por medio:
//
//	{
//	  "title": "Tell me about a time you improved a slow pipeline",
//	  "level": "B2",
//	  "category_id": 1,
//	  "author": "Entrevista en inglés",
//	  "paragraphs": [
//	    {
//	      "content": "Sure, that happened at my last job.",
//	      "pronunciation_es": "shur dat jápend at mai last yob",
//	      "translations": [{"language": "es", "content": "Claro, eso pasó en mi último trabajo."}]
//	    }
//	  ]
//	}
//
//	go run ./cmd/create_story -file historia.json -playlist 18
//	go run ./cmd/create_story -file historia.json -playlist 18 -apply
//
// Sin -apply solo valida e informa: no escribe nada. `position` se renumera por el
// orden del array, y no se escribe ninguna fila en story_voices (ver cmd/phrases_to_stories).
package main

import (
	"database/sql"
	"encoding/json"
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

var cefrLevels = map[string]bool{"A1": true, "A2": true, "B1": true, "B2": true, "C1": true, "C2": true}

func main() {
	file := flag.String("file", "", "archivo JSON con la historia (formato CreateFullStoryRequest)")
	playlistID := flag.Int("playlist", 0, "id del story playlist destino (tabla playlists)")
	allowDuplicate := flag.Bool("allow-duplicate", false, "permite crear una historia cuyo título ya existe en el playlist")
	apply := flag.Bool("apply", false, "inserta la historia; sin este flag solo valida")
	flag.Parse()

	if *file == "" || *playlistID == 0 {
		flag.Usage()
		os.Exit(2)
	}

	raw, err := os.ReadFile(*file)
	if err != nil {
		log.Fatalf("leer %s: %v", *file, err)
	}
	var req model.CreateFullStoryRequest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		log.Fatalf("JSON inválido en %s: %v", *file, err)
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

	// --- Validación: lo que la base exige, comprobado antes de intentar escribir ---

	var problems []string
	if strings.TrimSpace(req.Title) == "" {
		problems = append(problems, "falta title")
	}
	if !cefrLevels[req.Level] {
		problems = append(problems, fmt.Sprintf("level %q no es un nivel CEFR (A1..C2)", req.Level))
	}
	if len(req.Paragraphs) == 0 {
		problems = append(problems, "la historia no tiene párrafos")
	}
	if len(req.Voices) > 0 {
		problems = append(problems, "voices debe ir vacío: una voz de historia completa desactiva el modo por párrafo en el lector")
	}

	categoryName := "(no existe)"
	if err := db.QueryRow(`SELECT name FROM categories WHERE id = $1`, req.CategoryID).Scan(&categoryName); err == sql.ErrNoRows {
		problems = append(problems, fmt.Sprintf("no existe la categoría %d", req.CategoryID))
	} else if err != nil {
		log.Fatal(err)
	}

	totalWords := 0
	for i := range req.Paragraphs {
		p := &req.Paragraphs[i]
		// La posición la manda el orden del array: así el JSON no puede traer huecos
		// ni repetidos, que chocarían con UNIQUE (story_id, position).
		p.Position = i
		if strings.TrimSpace(p.Content) == "" {
			problems = append(problems, fmt.Sprintf("párrafo %d sin content", i+1))
		}
		if p.AudioURL != "" {
			problems = append(problems, fmt.Sprintf("párrafo %d trae audio_url: este comando no adjunta audio", i+1))
		}
		hasES := false
		for _, t := range p.Translations {
			if t.Language == "es" && strings.TrimSpace(t.Content) != "" {
				hasES = true
			}
		}
		if !hasES {
			problems = append(problems, fmt.Sprintf("párrafo %d sin traducción es", i+1))
		}
		totalWords += len(strings.Fields(p.Content))
	}

	playlistName, ownerEmail := "(no existe)", "?"
	if err := db.QueryRow(
		`SELECT p.name, u.email FROM playlists p JOIN users u ON u.id = p.user_id WHERE p.id = $1`, *playlistID,
	).Scan(&playlistName, &ownerEmail); err == sql.ErrNoRows {
		problems = append(problems, fmt.Sprintf("no existe el story playlist %d", *playlistID))
	} else if err != nil {
		log.Fatal(err)
	}

	var duplicateID int
	err = db.QueryRow(
		`SELECT s.id FROM playlist_stories ps JOIN stories s ON s.id = ps.story_id
		 WHERE ps.playlist_id = $1 AND lower(trim(s.title)) = lower(trim($2)) AND s.status != 'deleted'
		 LIMIT 1`, *playlistID, req.Title,
	).Scan(&duplicateID)
	if err != nil && err != sql.ErrNoRows {
		log.Fatal(err)
	}
	if duplicateID != 0 && !*allowDuplicate {
		problems = append(problems,
			fmt.Sprintf("el playlist ya tiene una historia titulada %q (id %d); usa -allow-duplicate si es a propósito", req.Title, duplicateID))
	}

	// --- Informe ---

	fmt.Printf("Historia:  %s\n", req.Title)
	fmt.Printf("Nivel:     %s   Categoría: %s (%d)   Autor: %s\n", req.Level, categoryName, req.CategoryID, req.Author)
	fmt.Printf("Párrafos:  %d (%d palabras en total, %.0f de media)\n",
		len(req.Paragraphs), totalWords, float64(totalWords)/float64(max(len(req.Paragraphs), 1)))
	fmt.Printf("Destino:   playlist %d %q (de %s)\n", *playlistID, playlistName, ownerEmail)
	pron := 0
	for _, p := range req.Paragraphs {
		if strings.TrimSpace(p.PronunciationES) != "" {
			pron++
		}
	}
	fmt.Printf("Extras:    %d/%d párrafos con pronunciación, sin audio, sin voces\n\n", pron, len(req.Paragraphs))

	if len(problems) > 0 {
		fmt.Println("No se puede insertar:")
		for _, p := range problems {
			fmt.Printf("  ✗ %s\n", p)
		}
		os.Exit(1)
	}

	if !*apply {
		fmt.Println("Validación OK. Volvé a correrlo con -apply para insertar.")
		return
	}

	// --- Inserción ---

	story, err := repo.CreateFull(&req)
	if err != nil {
		log.Fatalf("crear la historia: %v", err)
	}
	if err := repo.AddStoryToPlaylist(*playlistID, story.ID); err != nil {
		log.Fatalf("meter la historia %d en el playlist %d: %v", story.ID, *playlistID, err)
	}

	fmt.Printf("Listo: historia %d creada (status %s) y añadida al playlist %d.\n", story.ID, story.Status, *playlistID)
	fmt.Printf("Leela en /stories/%d\n", story.ID)
}
