package handler

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Max results returned per kind (phrases / story paragraphs). One extra row is
// fetched to tell the client the list was truncated.
const searchResultLimit = 300

// Accent folding shared by SQL and Go so "cancion" matches "canción" and "voce"
// matches "você". The unaccent extension isn't installed, so translate() it is.
const (
	foldFrom = "áàâãäéèêëíìîïóòôõöúùûüçñ"
	foldTo   = "aaaaaeeeeiiiiooooouuuucn"
)

var foldReplacer = func() *strings.Replacer {
	from, to := []rune(foldFrom), []rune(foldTo)
	pairs := make([]string, 0, len(from)*2)
	for i := range from {
		pairs = append(pairs, string(from[i]), string(to[i]))
	}
	return strings.NewReplacer(pairs...)
}()

func foldSQL(expr string) string {
	return fmt.Sprintf("translate(lower(%s), '%s', '%s')", expr, foldFrom, foldTo)
}

// searchToken is one folded query word. A leading space in the query anchors the
// first word to a word start (" se" → "second", not "close"); a trailing space
// anchors the last word to a word end ("se " → "close", not "closet").
type searchToken struct {
	word       string
	start, end bool
}

func searchTokens(q string) []searchToken {
	var out []searchToken
	for _, f := range strings.Fields(foldReplacer.Replace(strings.ToLower(q))) {
		out = append(out, searchToken{word: f})
	}
	if len(out) > 0 {
		out[0].start = strings.TrimLeftFunc(q, unicode.IsSpace) != q
		out[len(out)-1].end = strings.TrimRightFunc(q, unicode.IsSpace) != q
	}
	return out
}

// condition returns the SQL test of the token against haystack and its argument:
// a plain LIKE, or a regex when the word has to sit at a word boundary.
func (t searchToken) condition(haystack string, arg int) (string, any) {
	if !t.start && !t.end {
		esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
		return fmt.Sprintf("%s LIKE $%d", haystack, arg), "%" + esc.Replace(t.word) + "%"
	}
	re := regexp.QuoteMeta(t.word)
	if t.start {
		re = `(^|[^[:alnum:]])` + re
	}
	if t.end {
		re += `([^[:alnum:]]|$)`
	}
	return fmt.Sprintf("%s ~ $%d", haystack, arg), re
}

// parseIDList reads a CSV of ids. A missing param means "all"; a present but
// empty one means "none selected".
func parseIDList(r *http.Request, key string) (all bool, ids []int64, err error) {
	if !r.URL.Query().Has(key) {
		return true, []int64{}, nil
	}
	ids = []int64{}
	for _, part := range strings.Split(r.URL.Query().Get(key), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return false, nil, err
		}
		ids = append(ids, v)
	}
	return false, ids, nil
}

type searchPhrasePlaylist struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Kind        string `json:"kind"` // phrases | vocab | story_words
	PhraseCount int    `json:"phrase_count"`
	IsFavorite  bool   `json:"is_favorite"`
}

type searchStoryPlaylist struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	StoryCount int    `json:"story_count"`
	IsFavorite bool   `json:"is_favorite"`
}

// GET /api/search/sources
// Every phrase playlist and story playlist the user can read, for the source picker.
func (h *PhraseHandler) SearchSources(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	phrasePlaylists := []searchPhrasePlaylist{}
	rows, err := h.db.Query(`
		SELECT p.id, p.name, p.language,
		       CASE WHEN p.parent_playlist_id IS NOT NULL THEN 'vocab'
		            WHEN p.story_playlist_id IS NOT NULL THEN 'story_words'
		            ELSE 'phrases' END AS kind,
		       COUNT(ph.id),
		       p.user_id = $1 AND p.is_favorite AS is_favorite
		FROM phrase_playlists p
		LEFT JOIN phrase_playlist_shares s ON s.playlist_id = p.id AND s.user_id = $1
		LEFT JOIN phrase_groups g ON g.phrase_playlist_id = p.id
		LEFT JOIN phrases ph ON ph.phrase_group_id = g.id
		WHERE `+visibleTo(r, "p", "s")+`
		GROUP BY p.id
		ORDER BY is_favorite DESC, kind, p.created_at DESC`, userID)
	if err != nil {
		log.Printf("SearchSources phrase playlists: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var p searchPhrasePlaylist
		if err := rows.Scan(&p.ID, &p.Name, &p.Language, &p.Kind, &p.PhraseCount, &p.IsFavorite); err != nil {
			log.Printf("SearchSources phrase scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		phrasePlaylists = append(phrasePlaylists, p)
	}

	storyPlaylists := []searchStoryPlaylist{}
	srows, err := h.db.Query(`
		SELECT p.id, p.name, (SELECT COUNT(*) FROM playlist_stories WHERE playlist_id = p.id),
		       EXISTS(SELECT 1 FROM playlist_favorites f WHERE f.playlist_id = p.id AND f.user_id = $1::uuid) AS is_favorite
		FROM playlists p
		LEFT JOIN playlist_shares sh ON sh.playlist_id = p.id AND sh.user_id = $1::uuid
		WHERE p.user_id = $1::uuid OR sh.user_id = $1::uuid
		ORDER BY is_favorite DESC, p.created_at DESC`, userID)
	if err != nil {
		log.Printf("SearchSources story playlists: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer srows.Close()
	for srows.Next() {
		var p searchStoryPlaylist
		if err := srows.Scan(&p.ID, &p.Name, &p.StoryCount, &p.IsFavorite); err != nil {
			log.Printf("SearchSources story scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		storyPlaylists = append(storyPlaylists, p)
	}

	jsonOK(w, map[string]any{
		"phrase_playlists": phrasePlaylists,
		"story_playlists":  storyPlaylists,
	})
}

type searchPhraseHit struct {
	ID              int     `json:"id"`
	Text            string  `json:"text"`
	TranslationES   string  `json:"translation_es"`
	PronunciationES string  `json:"pronunciation_es"`
	CreatedAt       string  `json:"created_at"`
	PollyAudioURLF  string  `json:"polly_audio_url_female"`
	SourceAudioURL  string  `json:"source_audio_url"`
	SourceStartMs   *int    `json:"source_start_ms"`
	SourceEndMs     *int    `json:"source_end_ms"`
	PlaylistID      int     `json:"playlist_id"`
	PlaylistName    string  `json:"playlist_name"`
	GroupName       string  `json:"group_name"`
	ReviewCount     int     `json:"review_count"`
	NextReviewAt    *string `json:"next_review_at"`
}

type searchParagraphHit struct {
	ID              int    `json:"id"`
	Text            string `json:"text"`
	TranslationES   string `json:"translation_es"`
	PronunciationES string `json:"pronunciation_es"`
	AudioURL        string `json:"audio_url"`
	Position        int    `json:"position"`
	StoryID         int    `json:"story_id"`
	StoryTitle      string `json:"story_title"`
	CreatedAt       string `json:"created_at"`
	PlaylistNames   string `json:"playlist_names"`
}

// GET /api/search?q=...&phrase_playlists=1,2&story_playlists=3
// Every query word must appear (accent/case-insensitive) in the text or its Spanish
// translation; leading/trailing spaces anchor the first/last word (see searchToken). Omitted playlist params = search all the user can read.
func (h *PhraseHandler) Search(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tokens := searchTokens(r.URL.Query().Get("q"))
	if len(tokens) == 0 {
		jsonError(w, "q is required", http.StatusBadRequest)
		return
	}
	allPhrase, phraseIDs, err := parseIDList(r, "phrase_playlists")
	if err != nil {
		jsonError(w, "invalid phrase_playlists", http.StatusBadRequest)
		return
	}
	allStory, storyIDs, err := parseIDList(r, "story_playlists")
	if err != nil {
		jsonError(w, "invalid story_playlists", http.StatusBadRequest)
		return
	}

	// tokenWhere ANDs one test per word against the folded haystack; args start at $first.
	tokenWhere := func(haystack string, first int) (string, []any) {
		var sb strings.Builder
		args := make([]any, 0, len(tokens))
		for i, t := range tokens {
			cond, arg := t.condition(foldSQL(haystack), first+i)
			sb.WriteString(" AND " + cond)
			args = append(args, arg)
		}
		return sb.String(), args
	}

	phrases := []searchPhraseHit{}
	phrasesTruncated := false
	if allPhrase || len(phraseIDs) > 0 {
		cond, tArgs := tokenWhere("ph.text || ' ' || COALESCE(ph.translation_es, '')", 4)
		q := `
			SELECT ph.id, ph.text, COALESCE(ph.translation_es, ''), COALESCE(ph.pronunciation_es, ''), ph.created_at,
			       COALESCE(ph.polly_audio_url_female, ''), COALESCE(ph.source_audio_url, ''),
			       ph.source_start_ms, ph.source_end_ms,
			       pp.id, pp.name, g.name,
			       COALESCE(rc.review_count, 0), srs.next_review_at
			FROM phrases ph
			JOIN phrase_groups g ON g.id = ph.phrase_group_id
			JOIN phrase_playlists pp ON pp.id = g.phrase_playlist_id
			LEFT JOIN phrase_playlist_shares s ON s.playlist_id = pp.id AND s.user_id = $1
			LEFT JOIN (
			    SELECT phrase_id, COUNT(*) AS review_count
			    FROM phrase_reviews WHERE user_id = $1
			    GROUP BY phrase_id
			) rc ON rc.phrase_id = ph.id
			LEFT JOIN user_phrase_srs srs ON srs.phrase_id = ph.id AND srs.user_id = $1
			WHERE `+visibleTo(r, "pp", "s")+` AND ($2 OR pp.id = ANY($3))` + cond + `
			ORDER BY pp.id, g.position, g.id, ph.position, ph.id
			LIMIT ` + strconv.Itoa(searchResultLimit+1)
		args := append([]any{userID, allPhrase, phraseIDs}, tArgs...)
		rows, err := h.db.Query(q, args...)
		if err != nil {
			log.Printf("Search phrases: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var p searchPhraseHit
			var created time.Time
			var start, end *int
			var next *time.Time
			if err := rows.Scan(&p.ID, &p.Text, &p.TranslationES, &p.PronunciationES, &created,
				&p.PollyAudioURLF, &p.SourceAudioURL, &start, &end,
				&p.PlaylistID, &p.PlaylistName, &p.GroupName, &p.ReviewCount, &next); err != nil {
				log.Printf("Search phrases scan: %v", err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			p.CreatedAt = created.Format(time.RFC3339)
			p.SourceStartMs, p.SourceEndMs = start, end
			if next != nil {
				s := next.Format(time.RFC3339)
				p.NextReviewAt = &s
			}
			phrases = append(phrases, p)
		}
		if len(phrases) > searchResultLimit {
			phrases, phrasesTruncated = phrases[:searchResultLimit], true
		}
	}

	paragraphs := []searchParagraphHit{}
	paragraphsTruncated := false
	if allStory || len(storyIDs) > 0 {
		cond, tArgs := tokenWhere("pa.content || ' ' || COALESCE(tr.content, '')", 4)
		q := `
			SELECT pa.id, pa.content, COALESCE(tr.content, ''), COALESCE(pa.pronunciation_es, ''),
			       COALESCE(pa.audio_url, ''), pa.position, st.id, st.title, st.created_at,
			       string_agg(DISTINCT pl.name, ' · ')
			FROM paragraphs pa
			JOIN stories st ON st.id = pa.story_id AND st.status != 'deleted'
			JOIN playlist_stories ps ON ps.story_id = st.id
			JOIN playlists pl ON pl.id = ps.playlist_id
			LEFT JOIN playlist_shares sh ON sh.playlist_id = pl.id AND sh.user_id = $1::uuid
			LEFT JOIN paragraph_translations tr ON tr.paragraph_id = pa.id AND tr.language = 'es'
			WHERE (pl.user_id = $1::uuid OR sh.user_id = $1::uuid) AND ($2 OR pl.id = ANY($3))` + cond + `
			GROUP BY pa.id, tr.content, st.id
			ORDER BY MIN(pl.id), st.id, pa.position
			LIMIT ` + strconv.Itoa(searchResultLimit+1)
		args := append([]any{userID, allStory, storyIDs}, tArgs...)
		rows, err := h.db.Query(q, args...)
		if err != nil {
			log.Printf("Search paragraphs: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var p searchParagraphHit
			var created time.Time
			if err := rows.Scan(&p.ID, &p.Text, &p.TranslationES, &p.PronunciationES, &p.AudioURL,
				&p.Position, &p.StoryID, &p.StoryTitle, &created, &p.PlaylistNames); err != nil {
				log.Printf("Search paragraphs scan: %v", err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			p.CreatedAt = created.Format(time.RFC3339)
			paragraphs = append(paragraphs, p)
		}
		if len(paragraphs) > searchResultLimit {
			paragraphs, paragraphsTruncated = paragraphs[:searchResultLimit], true
		}
	}

	jsonOK(w, map[string]any{
		"phrases":              phrases,
		"phrases_truncated":    phrasesTruncated,
		"paragraphs":           paragraphs,
		"paragraphs_truncated": paragraphsTruncated,
	})
}
