package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"listen-with-me/backend/internal/middleware"
	"listen-with-me/backend/internal/storage"
	"listen-with-me/backend/internal/tts/polly"
)

type PhraseHandler struct {
	db      *sql.DB
	storage storage.FileStorage
	polly   *polly.Provider
}

func NewPhraseHandler(db *sql.DB, fs storage.FileStorage, pollyProv *polly.Provider) *PhraseHandler {
	return &PhraseHandler{db: db, storage: fs, polly: pollyProv}
}


// PATCH /api/phrase-playlists/{id}  body: {"is_favorite": bool}
func (h *PhraseHandler) SetPlaylistFavorite(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		IsFavorite bool `json:"is_favorite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	res, err := h.db.Exec(
		`UPDATE phrase_playlists SET is_favorite = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`,
		body.IsFavorite, id, userID,
	)
	if err != nil {
		log.Printf("SetPlaylistFavorite: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{"is_favorite": body.IsFavorite})
}

type playlistShare struct {
	UserID     string `json:"user_id"`
	FullName   string `json:"full_name"`
	Email      string `json:"email"`
	Permission string `json:"permission"`
	CreatedAt  string `json:"created_at"`
}

// requireOwner returns nil if the current user owns the playlist. Writes error to w and returns error otherwise.
func (h *PhraseHandler) requireOwner(w http.ResponseWriter, playlistID int, userID string) error {
	var ownerID string
	err := h.db.QueryRow(`SELECT user_id FROM phrase_playlists WHERE id = $1`, playlistID).Scan(&ownerID)
	if err == sql.ErrNoRows {
		jsonError(w, "not found", http.StatusNotFound)
		return err
	}
	if err != nil {
		log.Printf("requireOwner: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return err
	}
	if ownerID != userID {
		jsonError(w, "only the owner can perform this action", http.StatusForbidden)
		return fmt.Errorf("not owner")
	}
	return nil
}

type shareCandidate struct {
	UserID   string `json:"user_id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

// GET /api/phrase-playlists/{id}/share-candidates?q=car — owner only
// Returns up to 5 users whose name or email contains q, excluding the owner and users already shared with.
func (h *PhraseHandler) ShareCandidates(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.requireOwner(w, id, userID); err != nil {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		jsonOK(w, []shareCandidate{})
		return
	}
	like := "%" + strings.ToLower(q) + "%"

	rows, err := h.db.Query(
		`SELECT id, "fullName", email
		 FROM users
		 WHERE (lower("fullName") LIKE $1 OR lower(email) LIKE $1)
		   AND id != $2
		   AND id NOT IN (SELECT user_id FROM phrase_playlist_shares WHERE playlist_id = $3)
		 ORDER BY "fullName" ASC
		 LIMIT 5`, like, userID, id,
	)
	if err != nil {
		log.Printf("ShareCandidates: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []shareCandidate{}
	for rows.Next() {
		var c shareCandidate
		if err := rows.Scan(&c.UserID, &c.FullName, &c.Email); err != nil {
			log.Printf("ShareCandidates scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		items = append(items, c)
	}
	jsonOK(w, items)
}

// GET /api/phrase-playlists/{id}/shares — owner only
func (h *PhraseHandler) ListShares(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.requireOwner(w, id, userID); err != nil {
		return
	}
	rows, err := h.db.Query(
		`SELECT u.id, u."fullName", u.email, s.permission, s.created_at
		 FROM phrase_playlist_shares s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.playlist_id = $1
		 ORDER BY s.created_at DESC`, id,
	)
	if err != nil {
		log.Printf("ListShares: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := []playlistShare{}
	for rows.Next() {
		var s playlistShare
		var createdAt time.Time
		if err := rows.Scan(&s.UserID, &s.FullName, &s.Email, &s.Permission, &createdAt); err != nil {
			log.Printf("ListShares scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		s.CreatedAt = createdAt.Format("2006-01-02T15:04:05Z07:00")
		items = append(items, s)
	}
	jsonOK(w, items)
}

// POST /api/phrase-playlists/{id}/shares  body: {email, permission}
func (h *PhraseHandler) AddShare(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.requireOwner(w, id, userID); err != nil {
		return
	}
	var body struct {
		Email      string `json:"email"`
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}
	if body.Permission != "read" && body.Permission != "editor" {
		jsonError(w, "permission must be 'read' or 'editor'", http.StatusBadRequest)
		return
	}

	var targetID string
	err = h.db.QueryRow(`SELECT id FROM users WHERE lower(email) = $1`, body.Email).Scan(&targetID)
	if err == sql.ErrNoRows {
		jsonError(w, "no user with that email", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("AddShare user lookup: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if targetID == userID {
		jsonError(w, "you already own this playlist", http.StatusBadRequest)
		return
	}

	_, err = h.db.Exec(
		`INSERT INTO phrase_playlist_shares (playlist_id, user_id, permission)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (playlist_id, user_id) DO UPDATE SET permission = EXCLUDED.permission`,
		id, targetID, body.Permission,
	)
	if err != nil {
		log.Printf("AddShare upsert: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Return the created share row
	var share playlistShare
	var createdAt time.Time
	err = h.db.QueryRow(
		`SELECT u.id, u."fullName", u.email, s.permission, s.created_at
		 FROM phrase_playlist_shares s JOIN users u ON u.id = s.user_id
		 WHERE s.playlist_id = $1 AND s.user_id = $2`, id, targetID,
	).Scan(&share.UserID, &share.FullName, &share.Email, &share.Permission, &createdAt)
	if err != nil {
		log.Printf("AddShare read-back: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	share.CreatedAt = createdAt.Format("2006-01-02T15:04:05Z07:00")
	jsonOK(w, share)
}

// PATCH /api/phrase-playlists/{id}/shares/{userID}  body: {permission}
func (h *PhraseHandler) UpdateShare(w http.ResponseWriter, r *http.Request) {
	ownerID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	targetID := r.PathValue("userID")
	if targetID == "" {
		jsonError(w, "user id required", http.StatusBadRequest)
		return
	}
	if err := h.requireOwner(w, id, ownerID); err != nil {
		return
	}
	var body struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Permission != "read" && body.Permission != "editor" {
		jsonError(w, "permission must be 'read' or 'editor'", http.StatusBadRequest)
		return
	}
	res, err := h.db.Exec(
		`UPDATE phrase_playlist_shares SET permission = $1 WHERE playlist_id = $2 AND user_id = $3`,
		body.Permission, id, targetID,
	)
	if err != nil {
		log.Printf("UpdateShare: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "share not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{"permission": body.Permission})
}

// DELETE /api/phrase-playlists/{id}/shares/{userID}
func (h *PhraseHandler) RemoveShare(w http.ResponseWriter, r *http.Request) {
	ownerID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	targetID := r.PathValue("userID")
	if targetID == "" {
		jsonError(w, "user id required", http.StatusBadRequest)
		return
	}
	if err := h.requireOwner(w, id, ownerID); err != nil {
		return
	}
	if _, err := h.db.Exec(
		`DELETE FROM phrase_playlist_shares WHERE playlist_id = $1 AND user_id = $2`, id, targetID,
	); err != nil {
		log.Printf("RemoveShare: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

type importRequest struct {
	Playlist struct {
		Name        string `json:"name"`
		Language    string `json:"language"`
		Description string `json:"description"`
	} `json:"playlist"`
	Groups []struct {
		Name    string `json:"name"`
		Phrases []struct {
			Text            string `json:"text"`
			TranslationES   string `json:"translation_es"`
			PronunciationES string `json:"pronunciation_es"`
		} `json:"phrases"`
	} `json:"groups"`
}

type importGroupResult struct {
	Name           string `json:"name"`
	Created        bool   `json:"created"`
	PhrasesAdded   int    `json:"phrases_added"`
	PhrasesSkipped int    `json:"phrases_skipped"`
}

type importResult struct {
	PlaylistID      int                 `json:"playlist_id"`
	PlaylistName    string              `json:"playlist_name"`
	PlaylistCreated bool                `json:"playlist_created"`
	Groups          []importGroupResult `json:"groups"`
	PhrasesAdded    int                 `json:"phrases_added"`
	PhrasesSkipped  int                 `json:"phrases_skipped"`
}

// POST /api/phrase-playlists/import
// If a playlist with the same (name, language) exists for the user, append to it.
// Otherwise create a new one. Groups are matched by name (case-insensitive), phrases
// are deduped by text within their group.
func (h *PhraseHandler) Import(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid json body", http.StatusBadRequest)
		return
	}

	// Server-side validation (mirrors client-side but enforced)
	req.Playlist.Name = strings.TrimSpace(req.Playlist.Name)
	req.Playlist.Language = strings.TrimSpace(strings.ToLower(req.Playlist.Language))
	if req.Playlist.Name == "" {
		jsonError(w, "playlist.name is required", http.StatusBadRequest)
		return
	}
	if req.Playlist.Language != "en" && req.Playlist.Language != "pt" {
		jsonError(w, "playlist.language must be 'en' or 'pt'", http.StatusBadRequest)
		return
	}
	if len(req.Groups) == 0 {
		jsonError(w, "groups must contain at least one group", http.StatusBadRequest)
		return
	}
	for gi, g := range req.Groups {
		if strings.TrimSpace(g.Name) == "" {
			jsonError(w, fmt.Sprintf("groups[%d].name is required", gi), http.StatusBadRequest)
			return
		}
		if len(g.Phrases) == 0 {
			jsonError(w, fmt.Sprintf("groups[%d].phrases must contain at least one phrase", gi), http.StatusBadRequest)
			return
		}
		for pi, p := range g.Phrases {
			if strings.TrimSpace(p.Text) == "" {
				jsonError(w, fmt.Sprintf("groups[%d].phrases[%d].text is required", gi, pi), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(p.TranslationES) == "" {
				jsonError(w, fmt.Sprintf("groups[%d].phrases[%d].translation_es is required", gi, pi), http.StatusBadRequest)
				return
			}
		}
	}

	tx, err := h.db.Begin()
	if err != nil {
		log.Printf("Import begin: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Find or create playlist
	var playlistID int
	var playlistCreated bool
	err = tx.QueryRow(
		`SELECT id FROM phrase_playlists
		 WHERE user_id = $1 AND lower(name) = lower($2) AND language = $3`,
		userID, req.Playlist.Name, req.Playlist.Language,
	).Scan(&playlistID)
	if err == sql.ErrNoRows {
		err = tx.QueryRow(
			`INSERT INTO phrase_playlists (user_id, name, language, description)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			userID, req.Playlist.Name, req.Playlist.Language, req.Playlist.Description,
		).Scan(&playlistID)
		if err != nil {
			log.Printf("Import create playlist: %v", err)
			jsonError(w, "failed to create playlist", http.StatusInternalServerError)
			return
		}
		playlistCreated = true
	} else if err != nil {
		log.Printf("Import find playlist: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	result := importResult{
		PlaylistID:      playlistID,
		PlaylistName:    req.Playlist.Name,
		PlaylistCreated: playlistCreated,
	}

	for _, g := range req.Groups {
		var groupID int
		var groupCreated bool
		err = tx.QueryRow(
			`SELECT id FROM phrase_groups WHERE phrase_playlist_id = $1 AND lower(name) = lower($2)`,
			playlistID, g.Name,
		).Scan(&groupID)
		if err == sql.ErrNoRows {
			var pos int
			if err := tx.QueryRow(
				`SELECT COALESCE(MAX(position), -1) + 1 FROM phrase_groups WHERE phrase_playlist_id = $1`,
				playlistID,
			).Scan(&pos); err != nil {
				log.Printf("Import group pos: %v", err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			if err := tx.QueryRow(
				`INSERT INTO phrase_groups (phrase_playlist_id, name, position) VALUES ($1, $2, $3) RETURNING id`,
				playlistID, g.Name, pos,
			).Scan(&groupID); err != nil {
				log.Printf("Import create group: %v", err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			groupCreated = true
		} else if err != nil {
			log.Printf("Import find group: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Load existing phrase texts in this group (case-insensitive)
		existing := map[string]bool{}
		rows, err := tx.Query(`SELECT lower(text) FROM phrases WHERE phrase_group_id = $1`, groupID)
		if err != nil {
			log.Printf("Import load phrases: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				rows.Close()
				log.Printf("Import scan phrase: %v", err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			existing[t] = true
		}
		rows.Close()

		var pos int
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), -1) + 1 FROM phrases WHERE phrase_group_id = $1`,
			groupID,
		).Scan(&pos); err != nil {
			log.Printf("Import phrase pos: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}

		added := 0
		skipped := 0
		// Also dedupe within the same import payload
		for _, p := range g.Phrases {
			key := strings.ToLower(strings.TrimSpace(p.Text))
			if existing[key] {
				skipped++
				continue
			}
			if _, err := tx.Exec(
				`INSERT INTO phrases (phrase_group_id, text, translation_es, pronunciation_es, position)
				 VALUES ($1, $2, $3, $4, $5)`,
				groupID, strings.TrimSpace(p.Text), strings.TrimSpace(p.TranslationES),
				strings.TrimSpace(p.PronunciationES), pos,
			); err != nil {
				log.Printf("Import insert phrase: %v", err)
				jsonError(w, "failed to insert phrase", http.StatusInternalServerError)
				return
			}
			existing[key] = true
			pos++
			added++
		}

		result.Groups = append(result.Groups, importGroupResult{
			Name: g.Name, Created: groupCreated, PhrasesAdded: added, PhrasesSkipped: skipped,
		})
		result.PhrasesAdded += added
		result.PhrasesSkipped += skipped
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Import commit: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, result)
}

type phrasePlaylistSummary struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Language    string `json:"language"`
	IsFavorite  bool   `json:"is_favorite"`
	Role        string `json:"role"` // owner | editor | read
	GroupCount  int    `json:"group_count"`
	PhraseCount int    `json:"phrase_count"`
	CreatedAt   string `json:"created_at"`
}

type phraseItem struct {
	ID              int        `json:"id"`
	Text            string     `json:"text"`
	TranslationES   string     `json:"translation_es"`
	PronunciationES string     `json:"pronunciation_es"`
	Position        int        `json:"position"`
	CreatedAt       string     `json:"created_at"`
	ReviewCount     int        `json:"review_count"`
	PollyAudioURLF  string     `json:"polly_audio_url_female"`
	PollyAudioURLM  string     `json:"polly_audio_url_male"`
	SourceAudioURL  string     `json:"source_audio_url"`
	SourceStartMs   *int       `json:"source_start_ms"`
	SourceEndMs     *int       `json:"source_end_ms"`
	SRS             *phraseSRS `json:"srs"`
}

type phraseSRS struct {
	EaseFactor     float64 `json:"ease_factor"`
	IntervalDays   float64 `json:"interval_days"`
	Repetitions    int     `json:"repetitions"`
	Lapses         int     `json:"lapses"`
	LastReviewedAt *string `json:"last_reviewed_at"`
	NextReviewAt   string  `json:"next_review_at"`
}

type phraseGroupDetail struct {
	ID       int          `json:"id"`
	Name     string       `json:"name"`
	Position int          `json:"position"`
	Phrases  []phraseItem `json:"phrases"`
}

type phrasePlaylistDetail struct {
	ID                 int                 `json:"id"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	Language           string              `json:"language"`
	IsFavorite         bool                `json:"is_favorite"`
	Role               string              `json:"role"`
	ParentPlaylistID   *int                `json:"parent_playlist_id"`
	ParentPlaylistName string              `json:"parent_playlist_name"`
	StoryPlaylistID    *int                `json:"story_playlist_id"`
	Groups             []phraseGroupDetail `json:"groups"`
}

// GET /api/phrase-playlists?language=en
func (h *PhraseHandler) ListPlaylists(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	lang := strings.TrimSpace(r.URL.Query().Get("language"))
	q := `
		SELECT p.id, p.name, COALESCE(p.description, ''), p.language,
		       CASE WHEN p.user_id = $1 THEN p.is_favorite ELSE FALSE END AS is_favorite,
		       CASE WHEN p.user_id = $1 THEN 'owner' ELSE COALESCE(s.permission, '') END AS role,
		       p.created_at,
		       COUNT(DISTINCT g.id) AS group_count,
		       COUNT(ph.id)         AS phrase_count
		FROM phrase_playlists p
		LEFT JOIN phrase_groups g ON g.phrase_playlist_id = p.id
		LEFT JOIN phrases ph      ON ph.phrase_group_id  = g.id
		LEFT JOIN phrase_playlist_shares s ON s.playlist_id = p.id AND s.user_id = $1
		WHERE (p.user_id = $1 OR s.user_id = $1)`
	args := []any{userID}
	if lang != "" {
		q += ` AND p.language = $2`
		args = append(args, lang)
	}
	q += ` AND p.parent_playlist_id IS NULL AND p.story_playlist_id IS NULL GROUP BY p.id, s.permission ORDER BY is_favorite DESC, p.created_at DESC`

	rows, err := h.db.Query(q, args...)
	if err != nil {
		log.Printf("ListPlaylists query: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := []phrasePlaylistSummary{}
	for rows.Next() {
		var s phrasePlaylistSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Language, &s.IsFavorite, &s.Role, &s.CreatedAt, &s.GroupCount, &s.PhraseCount); err != nil {
			log.Printf("ListPlaylists scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		items = append(items, s)
	}
	jsonOK(w, items)
}

type storyPhrasePlaylistSummary struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Language          string `json:"language"`
	StoryPlaylistID   int    `json:"story_playlist_id"`
	StoryPlaylistName string `json:"story_playlist_name"`
	PhraseCount       int    `json:"phrase_count"`
	CreatedAt         string `json:"created_at"`
}

// GET /api/story-phrase-playlists
// Lists the current user's word playlists (one per story playlist), kept separate
// from the normal phrase playlists.
func (h *PhraseHandler) ListStoryPhrasePlaylists(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.db.Query(
		`SELECT p.id, p.name, p.language, p.story_playlist_id, sp.name, COUNT(ph.id) AS phrase_count, p.created_at
		 FROM phrase_playlists p
		 JOIN playlists sp ON sp.id = p.story_playlist_id
		 LEFT JOIN phrase_groups g ON g.phrase_playlist_id = p.id
		 LEFT JOIN phrases ph ON ph.phrase_group_id = g.id
		 WHERE p.user_id = $1 AND p.story_playlist_id IS NOT NULL
		 GROUP BY p.id, sp.name
		 ORDER BY p.created_at DESC`,
		userID,
	)
	if err != nil {
		log.Printf("ListStoryPhrasePlaylists query: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := []storyPhrasePlaylistSummary{}
	for rows.Next() {
		var s storyPhrasePlaylistSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Language, &s.StoryPlaylistID, &s.StoryPlaylistName, &s.PhraseCount, &s.CreatedAt); err != nil {
			log.Printf("ListStoryPhrasePlaylists scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		items = append(items, s)
	}
	jsonOK(w, items)
}

// GET /api/phrase-playlists/{id}
func (h *PhraseHandler) GetPlaylist(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	var pl phrasePlaylistDetail
	var parentID sql.NullInt64
	var parentName sql.NullString
	var storyPlaylistID sql.NullInt64
	err = h.db.QueryRow(
		`SELECT p.id, p.name, COALESCE(p.description, ''), p.language,
		        CASE WHEN p.user_id = $2 THEN p.is_favorite ELSE FALSE END,
		        CASE WHEN p.user_id = $2 THEN 'owner' ELSE COALESCE(s.permission, '') END AS role,
		        p.parent_playlist_id, parent.name, p.story_playlist_id
		 FROM phrase_playlists p
		 LEFT JOIN phrase_playlist_shares s ON s.playlist_id = p.id AND s.user_id = $2
		 LEFT JOIN phrase_playlists parent ON parent.id = p.parent_playlist_id
		 WHERE p.id = $1 AND (p.user_id = $2 OR s.user_id = $2)`,
		id, userID,
	).Scan(&pl.ID, &pl.Name, &pl.Description, &pl.Language, &pl.IsFavorite, &pl.Role, &parentID, &parentName, &storyPlaylistID)
	if err == sql.ErrNoRows {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("GetPlaylist row: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if parentID.Valid {
		v := int(parentID.Int64)
		pl.ParentPlaylistID = &v
		pl.ParentPlaylistName = parentName.String
	}
	if storyPlaylistID.Valid {
		v := int(storyPlaylistID.Int64)
		pl.StoryPlaylistID = &v
	}

	rows, err := h.db.Query(
		`SELECT g.id, g.name, g.position,
		        ph.id, ph.text, ph.translation_es, COALESCE(ph.pronunciation_es, ''), ph.position, ph.created_at,
		        COALESCE(ph.polly_audio_url_female, ''),
		        COALESCE(ph.polly_audio_url_male, ''),
		        COALESCE(ph.source_audio_url, ''), ph.source_start_ms, ph.source_end_ms,
		        COALESCE(rc.review_count, 0) AS review_count,
		        srs.ease_factor, srs.interval_days, srs.repetitions, srs.lapses,
		        srs.last_reviewed_at, srs.next_review_at
		 FROM phrase_groups g
		 LEFT JOIN phrases ph ON ph.phrase_group_id = g.id
		 LEFT JOIN (
		     SELECT phrase_id, COUNT(*) AS review_count
		     FROM phrase_reviews WHERE user_id = $2
		     GROUP BY phrase_id
		 ) rc ON rc.phrase_id = ph.id
		 LEFT JOIN user_phrase_srs srs ON srs.phrase_id = ph.id AND srs.user_id = $2
		 WHERE g.phrase_playlist_id = $1
		 ORDER BY g.position, g.id, ph.position, ph.id`,
		id, userID,
	)
	if err != nil {
		log.Printf("GetPlaylist groups query: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	groupByID := map[int]*phraseGroupDetail{}
	orderedGroupIDs := []int{}
	for rows.Next() {
		var gID, gPos int
		var gName string
		var phID sql.NullInt64
		var phText, phTr, phPron sql.NullString
		var phPos sql.NullInt64
		var phCreated sql.NullTime
		var phPollyURLF, phPollyURLM, phSourceURL string
		var phSourceStart, phSourceEnd sql.NullInt64
		var reviewCount int
		var srsEF, srsInterval sql.NullFloat64
		var srsReps, srsLapses sql.NullInt64
		var srsLast, srsNext sql.NullTime
		if err := rows.Scan(&gID, &gName, &gPos, &phID, &phText, &phTr, &phPron, &phPos, &phCreated,
			&phPollyURLF, &phPollyURLM,
			&phSourceURL, &phSourceStart, &phSourceEnd,
			&reviewCount, &srsEF, &srsInterval, &srsReps, &srsLapses, &srsLast, &srsNext); err != nil {
			log.Printf("GetPlaylist scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		g, ok := groupByID[gID]
		if !ok {
			g = &phraseGroupDetail{ID: gID, Name: gName, Position: gPos, Phrases: []phraseItem{}}
			groupByID[gID] = g
			orderedGroupIDs = append(orderedGroupIDs, gID)
		}
		if phID.Valid {
			p := phraseItem{
				ID:              int(phID.Int64),
				Text:            phText.String,
				TranslationES:   phTr.String,
				PronunciationES: phPron.String,
				Position:        int(phPos.Int64),
				ReviewCount:     reviewCount,
				PollyAudioURLF:  phPollyURLF,
				PollyAudioURLM:  phPollyURLM,
				SourceAudioURL:  phSourceURL,
			}
			if phSourceStart.Valid {
				v := int(phSourceStart.Int64)
				p.SourceStartMs = &v
			}
			if phSourceEnd.Valid {
				v := int(phSourceEnd.Int64)
				p.SourceEndMs = &v
			}
			if phCreated.Valid {
				p.CreatedAt = phCreated.Time.Format("2006-01-02T15:04:05Z07:00")
			}
			if srsEF.Valid {
				s := &phraseSRS{
					EaseFactor:   srsEF.Float64,
					IntervalDays: srsInterval.Float64,
					Repetitions:  int(srsReps.Int64),
					Lapses:       int(srsLapses.Int64),
					NextReviewAt: srsNext.Time.Format("2006-01-02T15:04:05Z07:00"),
				}
				if srsLast.Valid {
					t := srsLast.Time.Format("2006-01-02T15:04:05Z07:00")
					s.LastReviewedAt = &t
				}
				p.SRS = s
			}
			g.Phrases = append(g.Phrases, p)
		}
	}
	pl.Groups = make([]phraseGroupDetail, 0, len(orderedGroupIDs))
	for _, gID := range orderedGroupIDs {
		pl.Groups = append(pl.Groups, *groupByID[gID])
	}
	jsonOK(w, pl)
}

// hasPlaylistAccess returns true if the current user owns or has any share for the playlist.
// Used for read-level access checks (viewing, adding to personal vocabulary, etc.).
func (h *PhraseHandler) hasPlaylistAccess(userID string, playlistID int) (bool, error) {
	var ok bool
	err := h.db.QueryRow(`
		SELECT p.user_id = $1
		    OR EXISTS (SELECT 1 FROM phrase_playlist_shares WHERE playlist_id = p.id AND user_id = $1)
		FROM phrase_playlists p WHERE p.id = $2`, userID, playlistID,
	).Scan(&ok)
	return ok, err
}

type vocabWordItem struct {
	ID              int    `json:"id"`
	Text            string `json:"text"`
	PollyAudioURLF  string `json:"polly_audio_url_female"`
	PollyAudioURLM  string `json:"polly_audio_url_male"`
}

// GET /api/phrase-playlists/{id}/vocabulary
// Returns the user's vocab child playlist id, the list of saved words (with cached Polly URLs),
// and their count. If no child exists yet, child_playlist_id is null and words is [].
func (h *PhraseHandler) GetPhraseVocabularyInfo(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	parentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	ok, err := h.hasPlaylistAccess(userID, parentID)
	if err != nil {
		log.Printf("GetPhraseVocabularyInfo access: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		jsonError(w, "playlist not found or no access", http.StatusForbidden)
		return
	}
	var childID sql.NullInt64
	err = h.db.QueryRow(
		`SELECT id FROM phrase_playlists WHERE user_id = $1 AND parent_playlist_id = $2 LIMIT 1`,
		userID, parentID,
	).Scan(&childID)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("GetPhraseVocabularyInfo child: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	words := []vocabWordItem{}
	if childID.Valid {
		rows, err := h.db.Query(
			`SELECT ph.id, ph.text,
			        COALESCE(ph.polly_audio_url_female, ''),
			        COALESCE(ph.polly_audio_url_male, '')
			 FROM phrases ph
			 JOIN phrase_groups g ON g.id = ph.phrase_group_id
			 WHERE g.phrase_playlist_id = $1
			 ORDER BY ph.position, ph.id`, childID.Int64,
		)
		if err != nil {
			log.Printf("GetPhraseVocabularyInfo words: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var item vocabWordItem
			if err := rows.Scan(&item.ID, &item.Text, &item.PollyAudioURLF, &item.PollyAudioURLM); err != nil {
				log.Printf("GetPhraseVocabularyInfo scan: %v", err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			words = append(words, item)
		}
	}
	result := map[string]any{"phrase_count": len(words), "child_playlist_id": nil, "words": words}
	if childID.Valid {
		v := int(childID.Int64)
		result["child_playlist_id"] = v
	}
	jsonOK(w, result)
}

// POST /api/phrase-playlists/{id}/vocabulary  body: {text, source_phrase_id?}
// Ensures a vocab child playlist exists for (user, parent), inserts the word as a phrase
// inside a default "Words" group. Dedup by lower(text). Returns child playlist id + phrase id.
func (h *PhraseHandler) AddPhraseVocabulary(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	parentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Text           string `json:"text"`
		SourcePhraseID *int   `json:"source_phrase_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	if body.Text == "" {
		jsonError(w, "text is required", http.StatusBadRequest)
		return
	}
	if len(body.Text) > 500 {
		jsonError(w, "text too long", http.StatusBadRequest)
		return
	}
	ok, err := h.hasPlaylistAccess(userID, parentID)
	if err != nil {
		log.Printf("AddPhraseVocabulary access: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		jsonError(w, "playlist not found or no access", http.StatusForbidden)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		log.Printf("AddPhraseVocabulary begin tx: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1. Find or create the vocab child playlist for (user, parent)
	var childID int
	var childCreated bool
	err = tx.QueryRow(
		`SELECT id FROM phrase_playlists WHERE user_id = $1 AND parent_playlist_id = $2`,
		userID, parentID,
	).Scan(&childID)
	if err == sql.ErrNoRows {
		var parentName, parentLang string
		if err := tx.QueryRow(
			`SELECT name, language FROM phrase_playlists WHERE id = $1`, parentID,
		).Scan(&parentName, &parentLang); err != nil {
			log.Printf("AddPhraseVocabulary parent info: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		childName := "📗 Vocab · " + parentName
		if err := tx.QueryRow(
			`INSERT INTO phrase_playlists
			   (user_id, name, description, language, parent_playlist_id)
			 VALUES ($1, $2, $3, $4, $5)
			 RETURNING id`,
			userID, childName, "Vocabulary saved from '"+parentName+"'", parentLang, parentID,
		).Scan(&childID); err != nil {
			log.Printf("AddPhraseVocabulary create child: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		childCreated = true
	} else if err != nil {
		log.Printf("AddPhraseVocabulary find child: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// 2. Find or create the default "Words" group
	var groupID int
	err = tx.QueryRow(
		`SELECT id FROM phrase_groups WHERE phrase_playlist_id = $1 ORDER BY position, id LIMIT 1`,
		childID,
	).Scan(&groupID)
	if err == sql.ErrNoRows {
		if err := tx.QueryRow(
			`INSERT INTO phrase_groups (phrase_playlist_id, name, position) VALUES ($1, 'Words', 0) RETURNING id`,
			childID,
		).Scan(&groupID); err != nil {
			log.Printf("AddPhraseVocabulary create group: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		log.Printf("AddPhraseVocabulary find group: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// 3. Dedup by lower(text) inside this group
	var existing sql.NullInt64
	if err := tx.QueryRow(
		`SELECT id FROM phrases WHERE phrase_group_id = $1 AND lower(text) = lower($2) LIMIT 1`,
		groupID, body.Text,
	).Scan(&existing); err != nil && err != sql.ErrNoRows {
		log.Printf("AddPhraseVocabulary dedup: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	var phraseID int
	var phraseCreated bool
	if existing.Valid {
		phraseID = int(existing.Int64)
	} else {
		var pos int
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), -1) + 1 FROM phrases WHERE phrase_group_id = $1`, groupID,
		).Scan(&pos); err != nil {
			log.Printf("AddPhraseVocabulary phrase pos: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := tx.QueryRow(
			`INSERT INTO phrases (phrase_group_id, text, translation_es, pronunciation_es, position)
			 VALUES ($1, $2, '', '', $3) RETURNING id`,
			groupID, body.Text, pos,
		).Scan(&phraseID); err != nil {
			log.Printf("AddPhraseVocabulary insert phrase: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		phraseCreated = true
	}

	if err := tx.Commit(); err != nil {
		log.Printf("AddPhraseVocabulary commit: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]any{
		"child_playlist_id": childID,
		"phrase_id":         phraseID,
		"child_created":     childCreated,
		"phrase_created":    phraseCreated,
	})
}

// requireEditAccessByPhraseID checks that the current user can edit the playlist containing this phrase.
// Writes the appropriate error response and returns false when access is denied.
func (h *PhraseHandler) requireEditAccessByPhraseID(w http.ResponseWriter, userID string, phraseID int) bool {
	var canEdit bool
	err := h.db.QueryRow(`
		SELECT (
			p.user_id = $1
			OR EXISTS (
				SELECT 1 FROM phrase_playlist_shares
				WHERE playlist_id = p.id AND user_id = $1 AND permission = 'editor'
			)
		)
		FROM phrases ph
		JOIN phrase_groups g ON g.id = ph.phrase_group_id
		JOIN phrase_playlists p ON p.id = g.phrase_playlist_id
		WHERE ph.id = $2`, userID, phraseID,
	).Scan(&canEdit)
	if err == sql.ErrNoRows {
		jsonError(w, "phrase not found", http.StatusNotFound)
		return false
	}
	if err != nil {
		log.Printf("requireEditAccessByPhraseID: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return false
	}
	if !canEdit {
		jsonError(w, "you don't have edit access to this playlist", http.StatusForbidden)
		return false
	}
	return true
}

// requireEditAccessByGroupID — same but for a group.
func (h *PhraseHandler) requireEditAccessByGroupID(w http.ResponseWriter, userID string, groupID int) bool {
	var canEdit bool
	err := h.db.QueryRow(`
		SELECT (
			p.user_id = $1
			OR EXISTS (
				SELECT 1 FROM phrase_playlist_shares
				WHERE playlist_id = p.id AND user_id = $1 AND permission = 'editor'
			)
		)
		FROM phrase_groups g
		JOIN phrase_playlists p ON p.id = g.phrase_playlist_id
		WHERE g.id = $2`, userID, groupID,
	).Scan(&canEdit)
	if err == sql.ErrNoRows {
		jsonError(w, "group not found", http.StatusNotFound)
		return false
	}
	if err != nil {
		log.Printf("requireEditAccessByGroupID: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return false
	}
	if !canEdit {
		jsonError(w, "you don't have edit access to this playlist", http.StatusForbidden)
		return false
	}
	return true
}

// PUT /api/phrases/{id}  body: {text, translation_es, pronunciation_es}
func (h *PhraseHandler) UpdatePhrase(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	phraseID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Text            string `json:"text"`
		TranslationES   string `json:"translation_es"`
		PronunciationES string `json:"pronunciation_es"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Text = strings.TrimSpace(body.Text)
	body.TranslationES = strings.TrimSpace(body.TranslationES)
	body.PronunciationES = strings.TrimSpace(body.PronunciationES)
	if body.Text == "" {
		jsonError(w, "text is required", http.StatusBadRequest)
		return
	}
	if body.TranslationES == "" {
		jsonError(w, "translation_es is required", http.StatusBadRequest)
		return
	}
	if !h.requireEditAccessByPhraseID(w, userID, phraseID) {
		return
	}
	if _, err := h.db.Exec(
		`UPDATE phrases SET text = $1, translation_es = $2, pronunciation_es = $3 WHERE id = $4`,
		body.Text, body.TranslationES, body.PronunciationES, phraseID,
	); err != nil {
		log.Printf("UpdatePhrase: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// DELETE /api/phrases/{id}
func (h *PhraseHandler) DeletePhrase(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	phraseID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.requireEditAccessByPhraseID(w, userID, phraseID) {
		return
	}
	if _, err := h.db.Exec(`DELETE FROM phrases WHERE id = $1`, phraseID); err != nil {
		log.Printf("DeletePhrase: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// PUT /api/phrase-groups/{id}  body: {name}
func (h *PhraseHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		jsonError(w, "name is required", http.StatusBadRequest)
		return
	}
	if !h.requireEditAccessByGroupID(w, userID, groupID) {
		return
	}
	if _, err := h.db.Exec(
		`UPDATE phrase_groups SET name = $1, updated_at = NOW() WHERE id = $2`,
		body.Name, groupID,
	); err != nil {
		log.Printf("UpdateGroup: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// DELETE /api/phrase-groups/{id}  (cascades to phrases)
func (h *PhraseHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	groupID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.requireEditAccessByGroupID(w, userID, groupID) {
		return
	}
	if _, err := h.db.Exec(`DELETE FROM phrase_groups WHERE id = $1`, groupID); err != nil {
		log.Printf("DeleteGroup: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// POST /api/phrases/{id}/audio/polly?voice=female|male
// Generates (or returns cached) Polly audio for the phrase in its playlist's language
// using the chosen voice gender. Uploads MP3 to storage and caches the URL per gender.
func (h *PhraseHandler) GeneratePollyAudio(w http.ResponseWriter, r *http.Request) {
	if _, err := h.userIDFromContext(r); err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	phraseID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if h.polly == nil {
		jsonError(w, "polly is not configured on the server", http.StatusServiceUnavailable)
		return
	}
	if h.storage == nil {
		jsonError(w, "audio storage is not configured", http.StatusServiceUnavailable)
		return
	}

	gender := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("voice")))
	if gender != "male" {
		gender = "female"
	}

	// Load phrase text, playlist language, and any cached URLs for both genders
	var text, language, cachedF, cachedM string
	err = h.db.QueryRow(
		`SELECT ph.text, pl.language, COALESCE(ph.polly_audio_url_female, ''), COALESCE(ph.polly_audio_url_male, '')
		 FROM phrases ph
		 JOIN phrase_groups g ON g.id = ph.phrase_group_id
		 JOIN phrase_playlists pl ON pl.id = g.phrase_playlist_id
		 WHERE ph.id = $1`, phraseID,
	).Scan(&text, &language, &cachedF, &cachedM)
	if err == sql.ErrNoRows {
		jsonError(w, "phrase not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("GeneratePollyAudio load: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	cachedURL := cachedF
	if gender == "male" {
		cachedURL = cachedM
	}
	if cachedURL != "" {
		jsonOK(w, map[string]any{"audio_url": cachedURL, "voice": polly.VoiceFor(language, gender), "cached": true})
		return
	}

	data, contentType, err := h.polly.Synthesize(r.Context(), text, language, gender)
	if err != nil {
		log.Printf("GeneratePollyAudio synth: %v", err)
		jsonError(w, "polly synthesis failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	filename := fmt.Sprintf("phrases/polly/%d-%s.mp3", phraseID, gender)
	url, err := h.storage.Upload(r.Context(), filename, bytes.NewReader(data), contentType)
	if err != nil {
		log.Printf("GeneratePollyAudio upload: %v", err)
		jsonError(w, "failed to store audio", http.StatusInternalServerError)
		return
	}

	col := "polly_audio_url_female"
	if gender == "male" {
		col = "polly_audio_url_male"
	}
	if _, err := h.db.Exec(
		fmt.Sprintf(`UPDATE phrases SET %s = $1 WHERE id = $2`, col), url, phraseID,
	); err != nil {
		log.Printf("GeneratePollyAudio persist url: %v", err)
	}

	jsonOK(w, map[string]any{"audio_url": url, "voice": polly.VoiceFor(language, gender), "cached": false})
}

// applySM2 returns updated (easeFactor, intervalDays, repetitions, lapsesDelta)
// quality: 0=Again, 1=Hard, 2=Good, 3=Easy
func applySM2(quality int, ef, intervalDays float64, reps int) (float64, float64, int, int) {
	switch quality {
	case 0: // Again
		return math.Max(1.3, ef-0.2), 0, 0, 1
	case 1: // Hard
		nextInterval := 1.0
		if reps > 0 {
			nextInterval = math.Max(1, intervalDays*1.2)
		}
		return math.Max(1.3, ef-0.15), nextInterval, reps, 0
	case 2: // Good
		nextReps := reps + 1
		var nextInterval float64
		switch nextReps {
		case 1:
			nextInterval = 1
		case 2:
			nextInterval = 6
		default:
			nextInterval = math.Round(intervalDays * ef)
		}
		return ef, nextInterval, nextReps, 0
	case 3: // Easy
		nextReps := reps + 1
		var nextInterval float64
		switch nextReps {
		case 1:
			nextInterval = 4
		case 2:
			nextInterval = 7
		default:
			nextInterval = math.Round(intervalDays * ef * 1.3)
		}
		return ef + 0.15, nextInterval, nextReps, 0
	}
	return ef, intervalDays, reps, 0
}

// POST /api/phrases/{id}/rate  body: {"quality": 0..3}
// Applies SM-2 update, logs the review event, returns new SRS state.
func (h *PhraseHandler) RatePhrase(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	phraseID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		Quality int `json:"quality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Quality < 0 || body.Quality > 3 {
		jsonError(w, "quality must be 0..3", http.StatusBadRequest)
		return
	}

	ef, interval := 2.5, 0.0
	reps, lapses := 0, 0
	err = h.db.QueryRow(
		`SELECT ease_factor, interval_days, repetitions, lapses
		 FROM user_phrase_srs WHERE user_id = $1 AND phrase_id = $2`,
		userID, phraseID,
	).Scan(&ef, &interval, &reps, &lapses)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("RatePhrase load: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	newEF, newInterval, newReps, lapsesDelta := applySM2(body.Quality, ef, interval, reps)
	newLapses := lapses + lapsesDelta

	var nextAt time.Time
	err = h.db.QueryRow(
		`INSERT INTO user_phrase_srs
		   (user_id, phrase_id, ease_factor, interval_days, repetitions, lapses, last_reviewed_at, next_review_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now(), now() + (interval '1 day' * $4))
		 ON CONFLICT (user_id, phrase_id) DO UPDATE SET
		   ease_factor      = EXCLUDED.ease_factor,
		   interval_days    = EXCLUDED.interval_days,
		   repetitions      = EXCLUDED.repetitions,
		   lapses           = EXCLUDED.lapses,
		   last_reviewed_at = EXCLUDED.last_reviewed_at,
		   next_review_at   = EXCLUDED.next_review_at
		 RETURNING next_review_at`,
		userID, phraseID, newEF, newInterval, newReps, newLapses,
	).Scan(&nextAt)
	if err != nil {
		log.Printf("RatePhrase upsert: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	if _, err := h.db.Exec(`INSERT INTO phrase_reviews (user_id, phrase_id) VALUES ($1, $2)`, userID, phraseID); err != nil {
		log.Printf("RatePhrase review log: %v", err)
	}

	jsonOK(w, map[string]any{
		"ease_factor":    newEF,
		"interval_days":  newInterval,
		"repetitions":    newReps,
		"lapses":         newLapses,
		"next_review_at": nextAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// POST /api/phrases/{id}/review
// Logs a review event for the current user on the given phrase.
func (h *PhraseHandler) LogReview(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	phraseID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	_, err = h.db.Exec(
		`INSERT INTO phrase_reviews (user_id, phrase_id) VALUES ($1, $2)`,
		userID, phraseID,
	)
	if err != nil {
		log.Printf("LogReview insert: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

type leaderboardRow struct {
	UserID   string `json:"user_id"`
	FullName string `json:"full_name"`
	Count    int    `json:"count"`
}

// GET /api/phrase-stats/leaderboard?limit=10
// Returns top users by review count in day / week / month / year / all periods (calendar-based, UTC).
func (h *PhraseHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	if _, err := h.userIDFromContext(r); err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	periods := []struct {
		key    string
		filter string // WHERE clause
	}{
		{"day",   "reviewed_at >= date_trunc('day', now())"},
		{"week",  "reviewed_at >= date_trunc('week', now())"},
		{"month", "reviewed_at >= date_trunc('month', now())"},
		{"year",  "reviewed_at >= date_trunc('year', now())"},
		{"all",   "TRUE"},
	}

	result := map[string][]leaderboardRow{}
	for _, p := range periods {
		q := fmt.Sprintf(`
			SELECT u.id, u."fullName", COUNT(pr.id) AS c
			FROM phrase_reviews pr
			JOIN users u ON u.id = pr.user_id
			WHERE %s
			GROUP BY u.id, u."fullName"
			ORDER BY c DESC, u."fullName" ASC
			LIMIT $1`, p.filter)
		rows, err := h.db.Query(q, limit)
		if err != nil {
			log.Printf("Leaderboard %s query: %v", p.key, err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		items := []leaderboardRow{}
		for rows.Next() {
			var row leaderboardRow
			if err := rows.Scan(&row.UserID, &row.FullName, &row.Count); err != nil {
				rows.Close()
				log.Printf("Leaderboard %s scan: %v", p.key, err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			items = append(items, row)
		}
		rows.Close()
		result[p.key] = items
	}

	jsonOK(w, result)
}

// GET /api/phrase-stats/me
// Returns the current user's own review counts per period.
func (h *PhraseHandler) MyStats(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var day, week, month, year, all int
	err = h.db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('day',   now())) AS day,
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('week',  now())) AS week,
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('month', now())) AS month,
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('year',  now())) AS year,
			COUNT(*) AS all
		FROM phrase_reviews WHERE user_id = $1`, userID,
	).Scan(&day, &week, &month, &year, &all)
	if err != nil {
		log.Printf("MyStats query: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]int{"day": day, "week": week, "month": month, "year": year, "all": all})
}

type detailedStats struct {
	Counters             map[string]int         `json:"counters"`
	Streak               map[string]int         `json:"streak"`
	FirstReviewAt        *string                `json:"first_review_at"`
	BestDay              *dayCount              `json:"best_day"`
	UniquePhrases        int                    `json:"unique_phrases"`
	ActiveDays           int                    `json:"active_days"`
	Last30Days           []dayCount             `json:"last_30_days"`
	TopPlaylists         []topPlaylist          `json:"top_playlists"`
	LanguageDistribution []languageCount        `json:"language_distribution"`
	HourDistribution     [24]int                `json:"hour_distribution"`
}

type dayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type topPlaylist struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Count    int    `json:"count"`
}

type languageCount struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

// GET /api/phrase-stats/me/detailed
// Rich personal stats: counters, streaks, activity, top playlists, distributions.
func (h *PhraseHandler) MyStatsDetailed(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromContext(r)
	if err != nil {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	stats := detailedStats{
		Counters: map[string]int{"day": 0, "week": 0, "month": 0, "year": 0, "all": 0},
		Streak:   map[string]int{"current": 0, "longest": 0},
		Last30Days:           []dayCount{},
		TopPlaylists:         []topPlaylist{},
		LanguageDistribution: []languageCount{},
	}

	// Counters + unique phrases + active days + first review
	var firstAt sql.NullTime
	var cDay, cWeek, cMonth, cYear, cAll int
	err = h.db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('day',   now())) AS day,
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('week',  now())) AS week,
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('month', now())) AS month,
			COUNT(*) FILTER (WHERE reviewed_at >= date_trunc('year',  now())) AS year,
			COUNT(*) AS total,
			COUNT(DISTINCT phrase_id) AS unique_phrases,
			COUNT(DISTINCT date_trunc('day', reviewed_at)::date) AS active_days,
			MIN(reviewed_at) AS first_at
		FROM phrase_reviews WHERE user_id = $1`, userID,
	).Scan(&cDay, &cWeek, &cMonth, &cYear, &cAll, &stats.UniquePhrases, &stats.ActiveDays, &firstAt)
	stats.Counters["day"] = cDay
	stats.Counters["week"] = cWeek
	stats.Counters["month"] = cMonth
	stats.Counters["year"] = cYear
	stats.Counters["all"] = cAll
	if err != nil {
		log.Printf("MyStatsDetailed counters: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if firstAt.Valid {
		s := firstAt.Time.Format("2006-01-02T15:04:05Z07:00")
		stats.FirstReviewAt = &s
	}

	// Streaks (current + longest) via gap-and-islands on distinct review dates
	var currentStreak, longestStreak sql.NullInt64
	err = h.db.QueryRow(`
		WITH days AS (
			SELECT DISTINCT date_trunc('day', reviewed_at)::date AS d
			FROM phrase_reviews WHERE user_id = $1
		),
		grouped AS (
			SELECT d, d - (ROW_NUMBER() OVER (ORDER BY d))::int AS grp FROM days
		),
		streaks AS (
			SELECT grp, COUNT(*) AS len, MAX(d) AS end_date FROM grouped GROUP BY grp
		)
		SELECT
			COALESCE(MAX(len), 0) AS longest,
			COALESCE(MAX(CASE WHEN end_date = CURRENT_DATE OR end_date = CURRENT_DATE - 1 THEN len ELSE 0 END), 0) AS current
		FROM streaks`, userID,
	).Scan(&longestStreak, &currentStreak)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("MyStatsDetailed streaks: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	stats.Streak["longest"] = int(longestStreak.Int64)
	stats.Streak["current"] = int(currentStreak.Int64)

	// Best day
	var bdDate sql.NullTime
	var bdCount sql.NullInt64
	err = h.db.QueryRow(`
		SELECT date_trunc('day', reviewed_at)::date AS d, COUNT(*) AS c
		FROM phrase_reviews WHERE user_id = $1
		GROUP BY d ORDER BY c DESC, d DESC LIMIT 1`, userID,
	).Scan(&bdDate, &bdCount)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("MyStatsDetailed best day: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if bdDate.Valid {
		stats.BestDay = &dayCount{Date: bdDate.Time.Format("2006-01-02"), Count: int(bdCount.Int64)}
	}

	// Last 30 days activity — build a zero-filled array
	rows, err := h.db.Query(`
		SELECT date_trunc('day', reviewed_at)::date AS d, COUNT(*) AS c
		FROM phrase_reviews
		WHERE user_id = $1 AND reviewed_at >= CURRENT_DATE - INTERVAL '29 days'
		GROUP BY d ORDER BY d`, userID)
	if err != nil {
		log.Printf("MyStatsDetailed last30 query: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	counts := map[string]int{}
	for rows.Next() {
		var d sql.NullTime
		var c int
		if err := rows.Scan(&d, &c); err != nil {
			rows.Close()
			log.Printf("MyStatsDetailed last30 scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if d.Valid {
			counts[d.Time.Format("2006-01-02")] = c
		}
	}
	rows.Close()
	// Fill last 30 days including today
	now := time.Now().UTC().Truncate(24 * time.Hour)
	for i := 29; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		stats.Last30Days = append(stats.Last30Days, dayCount{Date: key, Count: counts[key]})
	}

	// Top playlists
	rows, err = h.db.Query(`
		SELECT pp.id, pp.name, pp.language, COUNT(pr.id) AS c
		FROM phrase_reviews pr
		JOIN phrases ph        ON ph.id = pr.phrase_id
		JOIN phrase_groups pg  ON pg.id = ph.phrase_group_id
		JOIN phrase_playlists pp ON pp.id = pg.phrase_playlist_id
		WHERE pr.user_id = $1
		GROUP BY pp.id, pp.name, pp.language
		ORDER BY c DESC, pp.name ASC
		LIMIT 5`, userID)
	if err != nil {
		log.Printf("MyStatsDetailed top playlists: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var tp topPlaylist
		if err := rows.Scan(&tp.ID, &tp.Name, &tp.Language, &tp.Count); err != nil {
			rows.Close()
			log.Printf("MyStatsDetailed top playlists scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		stats.TopPlaylists = append(stats.TopPlaylists, tp)
	}
	rows.Close()

	// Language distribution
	rows, err = h.db.Query(`
		SELECT pp.language, COUNT(pr.id) AS c
		FROM phrase_reviews pr
		JOIN phrases ph        ON ph.id = pr.phrase_id
		JOIN phrase_groups pg  ON pg.id = ph.phrase_group_id
		JOIN phrase_playlists pp ON pp.id = pg.phrase_playlist_id
		WHERE pr.user_id = $1
		GROUP BY pp.language
		ORDER BY c DESC`, userID)
	if err != nil {
		log.Printf("MyStatsDetailed lang dist: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var lc languageCount
		if err := rows.Scan(&lc.Language, &lc.Count); err != nil {
			rows.Close()
			log.Printf("MyStatsDetailed lang dist scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		stats.LanguageDistribution = append(stats.LanguageDistribution, lc)
	}
	rows.Close()

	// Hour distribution (UTC)
	rows, err = h.db.Query(`
		SELECT EXTRACT(HOUR FROM reviewed_at)::int AS h, COUNT(*) AS c
		FROM phrase_reviews WHERE user_id = $1
		GROUP BY h`, userID)
	if err != nil {
		log.Printf("MyStatsDetailed hour dist: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var h, c int
		if err := rows.Scan(&h, &c); err != nil {
			rows.Close()
			log.Printf("MyStatsDetailed hour dist scan: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if h >= 0 && h < 24 {
			stats.HourDistribution[h] = c
		}
	}
	rows.Close()

	jsonOK(w, stats)
}

func (h *PhraseHandler) userIDFromContext(r *http.Request) (string, error) {
	claims, ok := r.Context().Value(middleware.ClaimsKey).(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("no claims")
	}
	sub, ok := claims["sub"]
	if !ok {
		return "", fmt.Errorf("no sub in claims")
	}
	switch v := sub.(type) {
	case string:
		return v, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}
