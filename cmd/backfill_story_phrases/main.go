// Comando de un solo uso: recupera las palabras que un usuario guardó de una historia
// ANTES de meter esa historia en un story playlist, y que por eso nunca llegaron al
// playlist de palabras. Desde el arreglo en AddStoryToPlaylist esto ya no vuelve a
// pasar; esto repara lo que quedó suelto.
//
//	go run ./cmd/backfill_story_phrases -email alguien@x.com -playlist 16 -story 67
//	go run ./cmd/backfill_story_phrases -email alguien@x.com -playlist 16 -story 67 -apply
//
// Sin -apply solo informa: no escribe nada.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"listen-with-me/backend/internal/repository"
)

func main() {
	email := flag.String("email", "", "correo del usuario dueño de las palabras")
	playlistID := flag.Int("playlist", 0, "id del story playlist (tabla playlists)")
	storyID := flag.Int("story", 0, "id de la historia")
	apply := flag.Bool("apply", false, "ejecuta el backfill; sin este flag solo simula")
	flag.Parse()

	if *email == "" || *playlistID == 0 || *storyID == 0 {
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

	// --- Validaciones: replican las condiciones bajo las que la app misma habría
	// creado este playlist de palabras. Si alguna falla, no hay nada que reparar. ---

	var userID, fullName, language string
	if err := db.QueryRow(
		`SELECT id, "fullName", COALESCE(target_language, 'en') FROM users WHERE email = $1`, *email,
	).Scan(&userID, &fullName, &language); err == sql.ErrNoRows {
		log.Fatalf("no existe un usuario con el correo %s", *email)
	} else if err != nil {
		log.Fatal(err)
	}

	var playlistName, ownerID string
	if err := db.QueryRow(
		`SELECT name, user_id FROM playlists WHERE id = $1`, *playlistID,
	).Scan(&playlistName, &ownerID); err == sql.ErrNoRows {
		log.Fatalf("no existe el story playlist %d", *playlistID)
	} else if err != nil {
		log.Fatal(err)
	}

	var storyTitle string
	if err := db.QueryRow(
		`SELECT title FROM stories WHERE id = $1`, *storyID,
	).Scan(&storyTitle); err == sql.ErrNoRows {
		log.Fatalf("no existe la historia %d", *storyID)
	} else if err != nil {
		log.Fatal(err)
	}

	var inPlaylist bool
	if err := db.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM playlist_stories WHERE playlist_id = $1 AND story_id = $2)`,
		*playlistID, *storyID,
	).Scan(&inPlaylist); err != nil {
		log.Fatal(err)
	}
	if !inPlaylist {
		log.Fatalf("la historia %d no está en el playlist %d: nada que recuperar", *storyID, *playlistID)
	}

	// El usuario debe poder ver el playlist: dueño o con share. Es la misma condición
	// de PlaylistsContainingStory, la que decide a qué playlists llega una palabra.
	var hasAccess bool
	if err := db.QueryRow(
		`SELECT $2 = (SELECT user_id::text FROM playlists WHERE id = $1)
		     OR EXISTS (SELECT 1 FROM playlist_shares WHERE playlist_id = $1 AND user_id = $2::uuid)`,
		*playlistID, userID,
	).Scan(&hasAccess); err != nil {
		log.Fatal(err)
	}
	if !hasAccess {
		log.Fatalf("%s no es dueño del playlist %d ni lo tiene compartido", *email, *playlistID)
	}

	var vocabRows, vocabDistinct int
	if err := db.QueryRow(
		`SELECT count(*), count(DISTINCT lower(phrase)) FROM user_story_vocabulary
		 WHERE user_id = $1::uuid AND story_id = $2`, userID, *storyID,
	).Scan(&vocabRows, &vocabDistinct); err != nil {
		log.Fatal(err)
	}

	phrasePlaylistID, phrasesBefore := wordPlaylistState(db, userID, *playlistID)

	fmt.Println("── Estado actual ──────────────────────────────────────────")
	fmt.Printf("  usuario         : %s <%s> (idioma %s)\n", fullName, *email, language)
	fmt.Printf("  story playlist  : %d %q\n", *playlistID, playlistName)
	fmt.Printf("  historia        : %d %q\n", *storyID, storyTitle)
	fmt.Printf("  palabras guardadas: %d filas, %d distintas\n", vocabRows, vocabDistinct)
	if phrasePlaylistID == 0 {
		fmt.Println("  playlist de palabras: NO EXISTE (se creará)")
	} else {
		fmt.Printf("  playlist de palabras: %d, con %d frases\n", phrasePlaylistID, phrasesBefore)
	}

	if vocabRows == 0 {
		fmt.Println("\nNo hay palabras guardadas de esta historia: nada que hacer.")
		return
	}
	if !*apply {
		fmt.Printf("\nSimulación: con -apply se empujarían %d palabras (las repetidas se deduplican).\n", vocabRows)
		return
	}

	repo := repository.NewStoryRepo(db)
	pushed, err := repo.BackfillStoryPlaylistPhrases(userID, *playlistID, *storyID)
	if err != nil {
		log.Fatalf("backfill falló tras empujar %d palabras: %v", pushed, err)
	}

	phrasePlaylistAfter, phrasesAfter := wordPlaylistState(db, userID, *playlistID)
	fmt.Println("\n── Resultado ──────────────────────────────────────────────")
	fmt.Printf("  palabras procesadas : %d\n", pushed)
	fmt.Printf("  playlist de palabras: %d\n", phrasePlaylistAfter)
	fmt.Printf("  frases: %d → %d (+%d)\n", phrasesBefore, phrasesAfter, phrasesAfter-phrasesBefore)
}

// wordPlaylistState devuelve el id del playlist de palabras del usuario para ese story
// playlist (0 si no existe) y cuántas frases tiene.
func wordPlaylistState(db *sql.DB, userID string, storyPlaylistID int) (int, int) {
	var id, count int
	err := db.QueryRow(
		`SELECT p.id, (SELECT count(*) FROM phrases ph
		                JOIN phrase_groups g ON g.id = ph.phrase_group_id
		               WHERE g.phrase_playlist_id = p.id)
		 FROM phrase_playlists p
		 WHERE p.user_id = $1::uuid AND p.story_playlist_id = $2`,
		userID, storyPlaylistID,
	).Scan(&id, &count)
	if err == sql.ErrNoRows {
		return 0, 0
	}
	if err != nil {
		log.Fatal(err)
	}
	return id, count
}
