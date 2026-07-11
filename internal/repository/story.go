package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

	"listen-with-me/backend/internal/model"
)

type StoryRepo struct {
	db *sql.DB
}

func NewStoryRepo(db *sql.DB) *StoryRepo {
	return &StoryRepo{db: db}
}

// --- Categories ---

func (r *StoryRepo) ListCategories() ([]model.Category, error) {
	rows, err := r.db.Query(`SELECT id, name, slug FROM categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, nil
}

// --- Stories ---

func (r *StoryRepo) Create(s *model.Story) error {
	return r.db.QueryRow(
		`INSERT INTO stories (title, level, category_id, cover_url, author, status)
		 VALUES ($1, $2, $3, $4, $5, 'draft')
		 RETURNING id, status, created_at, updated_at`,
		s.Title, s.Level, s.CategoryID, s.CoverURL, s.Author,
	).Scan(&s.ID, &s.Status, &s.CreatedAt, &s.UpdatedAt)
}

func (r *StoryRepo) List(onlyPublished bool, playlistID int, userID string, sortBy string, limit int, offset int) ([]model.Story, bool, error) {
	log.Printf("Listing stories (onlyPublished=%v, playlistID=%d, sort=%s, limit=%d, offset=%d)", onlyPublished, playlistID, sortBy, limit, offset)
	query := `
		SELECT s.id, s.title, s.level, s.cover_url, s.author, s.status, s.created_at, s.updated_at,
		       c.id, c.name, c.slug,
		       COUNT(r.id) AS review_count,
		       MAX(r.reviewed_at) AS last_reviewed_at
		FROM stories s
		JOIN categories c ON c.id = s.category_id
		LEFT JOIN user_story_reviews r ON r.story_id = s.id AND r.user_id = $1`

	args := []interface{}{userID}
	where := []string{"s.status != 'deleted'"}

	if onlyPublished {
		where = append(where, "s.status = 'published'")
	}

	if playlistID > 0 {
		query += ` JOIN playlist_stories ps ON ps.story_id = s.id`
		where = append(where, fmt.Sprintf("ps.playlist_id = $%d", len(args)+1))
		args = append(args, playlistID)
	}

	query += " WHERE " + strings.Join(where, " AND ")
	query += ` GROUP BY s.id, c.id, c.name, c.slug`

	switch sortBy {
	case "most_reviewed":
		query += ` ORDER BY review_count DESC, last_reviewed_at DESC NULLS LAST, s.id ASC`
	case "last_reviewed":
		query += ` ORDER BY last_reviewed_at DESC NULLS LAST, review_count DESC, s.id ASC`
	case "newest":
		query += ` ORDER BY s.created_at DESC, s.id ASC`
	case "random":
		query += ` ORDER BY RANDOM()`
	default: // least_reviewed
		query += ` ORDER BY review_count ASC, last_reviewed_at ASC NULLS FIRST, s.id ASC`
	}

	if limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
		args = append(args, limit+1, offset) // fetch one extra to detect hasMore
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var stories []model.Story = []model.Story{}
	for rows.Next() {
		var s model.Story
		var cat model.Category
		var lastReviewedAt sql.NullTime
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Level, &s.CoverURL, &s.Author, &s.Status, &s.CreatedAt, &s.UpdatedAt,
			&cat.ID, &cat.Name, &cat.Slug,
			&s.ReviewCount, &lastReviewedAt,
		); err != nil {
			return nil, false, err
		}
		if lastReviewedAt.Valid {
			s.LastReviewedAt = &lastReviewedAt.Time
		}
		s.Category = &cat
		stories = append(stories, s)
	}

	hasMore := false
	if limit > 0 && len(stories) > limit {
		hasMore = true
		stories = stories[:limit]
	}
	return stories, hasMore, nil
}

func (r *StoryRepo) ListDeleted() ([]model.Story, error) {
	log.Printf("Listing deleted stories")
	query := `
		SELECT s.id, s.title, s.level, s.cover_url, s.author, s.status, s.created_at, s.updated_at,
		       c.id, c.name, c.slug
		FROM stories s
		JOIN categories c ON c.id = s.category_id
		WHERE s.status = 'deleted'
		ORDER BY s.updated_at DESC`

	rows, err := r.db.Query(query)
	if err != nil {
		log.Printf("Error querying deleted stories: %v", err)
		return nil, err
	}
	defer rows.Close()

	var stories []model.Story = []model.Story{}
	for rows.Next() {
		var s model.Story
		var cat model.Category
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Level, &s.CoverURL, &s.Author, &s.Status, &s.CreatedAt, &s.UpdatedAt,
			&cat.ID, &cat.Name, &cat.Slug,
		); err != nil {
			return nil, err
		}
		s.Category = &cat
		stories = append(stories, s)
	}
	log.Printf("Found %d deleted stories", len(stories))
	return stories, nil
}

func (r *StoryRepo) GetByID(id int) (*model.Story, error) {
	s := &model.Story{}
	var cat model.Category
	err := r.db.QueryRow(`
		SELECT s.id, s.title, s.level, s.cover_url, s.author, s.status, s.created_at, s.updated_at,
		       c.id, c.name, c.slug
		FROM stories s
		JOIN categories c ON c.id = s.category_id
		WHERE s.id = $1`, id,
	).Scan(
		&s.ID, &s.Title, &s.Level, &s.CoverURL, &s.Author, &s.Status, &s.CreatedAt, &s.UpdatedAt,
		&cat.ID, &cat.Name, &cat.Slug,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Category = &cat

	paragraphs, err := r.listParagraphs(id)
	if err != nil {
		return nil, err
	}
	s.Paragraphs = paragraphs

	voices, err := r.listVoices(id)
	if err != nil {
		return nil, err
	}
	s.Voices = voices
	return s, nil
}

func (r *StoryRepo) Publish(id int) error {
	_, err := r.db.Exec(
		`UPDATE stories SET status = 'published', updated_at = NOW() WHERE id = $1`, id,
	)
	return err
}

func (r *StoryRepo) Delete(id int) error {
	log.Printf("Soft deleting story ID: %d", id)
	_, err := r.db.Exec(`UPDATE stories SET status = 'deleted', updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *StoryRepo) Restore(id int) error {
	log.Printf("Restoring story ID: %d", id)
	_, err := r.db.Exec(`UPDATE stories SET status = 'draft', updated_at = NOW() WHERE id = $1`, id)
	return err
}

// UpdateFull updates a story by deleting all its paragraphs and re-inserting them.
func (r *StoryRepo) UpdateFull(id int, req *model.CreateFullStoryRequest) error {
	log.Printf("Updating story ID: %d", id)
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update main story metadata
	// If category_id is 0, we fetch the existing one to avoid FK violation
	finalCategoryID := req.CategoryID
	if finalCategoryID == 0 {
		err = tx.QueryRow(`SELECT category_id FROM stories WHERE id = $1`, id).Scan(&finalCategoryID)
		if err != nil {
			return fmt.Errorf("could not fetch existing category: %v", err)
		}
	}

	res, err := tx.Exec(
		`UPDATE stories SET title = $1, level = $2, category_id = $3, cover_url = $4, author = $5, updated_at = NOW()
		 WHERE id = $6`,
		req.Title, req.Level, finalCategoryID, req.CoverURL, req.Author, id,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("story not found")
	}

	// Delete existing paragraphs (cascades to translations and vocabulary)
	_, err = tx.Exec(`DELETE FROM paragraphs WHERE story_id = $1`, id)
	if err != nil {
		return err
	}

	// Re-insert all paragraphs
	for _, fp := range req.Paragraphs {
		var pID int
		err = tx.QueryRow(
			`INSERT INTO paragraphs (story_id, position, content, audio_url)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			id, fp.Position, fp.Content, fp.AudioURL,
		).Scan(&pID)
		if err != nil {
			return err
		}

		for i, imgURL := range fp.Images {
			_, err = tx.Exec(
				`INSERT INTO paragraph_images (paragraph_id, image_url, position)
				 VALUES ($1, $2, $3)`,
				pID, imgURL, i,
			)
			if err != nil {
				return err
			}
		}

		for _, tr := range fp.Translations {
			_, err = tx.Exec(
				`INSERT INTO paragraph_translations (paragraph_id, language, content)
				 VALUES ($1, $2, $3)`,
				pID, tr.Language, tr.Content,
			)
			if err != nil {
				return err
			}
		}

		for _, vr := range fp.Vocabulary {
			_, err = tx.Exec(
				`INSERT INTO vocabulary (paragraph_id, word, definition)
				 VALUES ($1, $2, $3)`,
				pID, vr.Word, vr.Definition,
			)
			if err != nil {
				return err
			}
		}
	}

	// Delete existing voices
	_, err = tx.Exec(`DELETE FROM story_voices WHERE story_id = $1`, id)
	if err != nil {
		return err
	}

	// Re-insert voices
	for _, v := range req.Voices {
		ts, _ := json.Marshal(v.Timestamps)
		_, err = tx.Exec(
			`INSERT INTO story_voices (story_id, name, audio_url, timestamps)
			 VALUES ($1, $2, $3, $4)`,
			id, v.Name, v.AudioURL, ts,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// CreateFull inserts a complete story with paragraphs, translations, vocabulary and voices
// inside a single transaction.
func (r *StoryRepo) CreateFull(req *model.CreateFullStoryRequest) (*model.Story, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	story := &model.Story{}
	err = tx.QueryRow(
		`INSERT INTO stories (title, level, category_id, cover_url, author, status)
		 VALUES ($1, $2, $3, $4, $5, 'draft')
		 RETURNING id, title, level, category_id, cover_url, author, status, created_at, updated_at`,
		req.Title, req.Level, req.CategoryID, req.CoverURL, req.Author,
	).Scan(&story.ID, &story.Title, &story.Level, &story.CategoryID,
		&story.CoverURL, &story.Author, &story.Status, &story.CreatedAt, &story.UpdatedAt)
	if err != nil {
		return nil, err
	}

	for _, fp := range req.Paragraphs {
		var pID int
		err = tx.QueryRow(
			`INSERT INTO paragraphs (story_id, position, content, audio_url)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			story.ID, fp.Position, fp.Content, fp.AudioURL,
		).Scan(&pID)
		if err != nil {
			return nil, err
		}

		p := model.Paragraph{ID: pID, StoryID: story.ID, Position: fp.Position, Content: fp.Content, AudioURL: fp.AudioURL}

		for i, imgURL := range fp.Images {
			var imgID int
			err = tx.QueryRow(
				`INSERT INTO paragraph_images (paragraph_id, image_url, position)
				 VALUES ($1, $2, $3) RETURNING id`,
				pID, imgURL, i,
			).Scan(&imgID)
			if err != nil {
				return nil, err
			}
			p.Images = append(p.Images, model.ParagraphImage{
				ID: imgID, ParagraphID: pID, ImageURL: imgURL, Position: i,
			})
		}

		for _, tr := range fp.Translations {
			var tID int
			err = tx.QueryRow(
				`INSERT INTO paragraph_translations (paragraph_id, language, content)
				 VALUES ($1, $2, $3) RETURNING id`,
				pID, tr.Language, tr.Content,
			).Scan(&tID)
			if err != nil {
				return nil, err
			}
			p.Translations = append(p.Translations, model.ParagraphTranslation{
				ID: tID, ParagraphID: pID, Language: tr.Language, Content: tr.Content,
			})
		}

		for _, vr := range fp.Vocabulary {
			var vID int
			err = tx.QueryRow(
				`INSERT INTO vocabulary (paragraph_id, word, definition)
				 VALUES ($1, $2, $3) RETURNING id`,
				pID, vr.Word, vr.Definition,
			).Scan(&vID)
			if err != nil {
				return nil, err
			}
			p.Vocabulary = append(p.Vocabulary, model.Vocabulary{
				ID: vID, ParagraphID: pID, Word: vr.Word, Definition: vr.Definition,
			})
		}

		story.Paragraphs = append(story.Paragraphs, p)
	}

	for _, v := range req.Voices {
		ts, _ := json.Marshal(v.Timestamps)
		var vID int
		err = tx.QueryRow(
			`INSERT INTO story_voices (story_id, name, audio_url, timestamps)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			story.ID, v.Name, v.AudioURL, ts,
		).Scan(&vID)
		if err != nil {
			return nil, err
		}
		story.Voices = append(story.Voices, model.StoryVoice{
			ID: vID, StoryID: story.ID, Name: v.Name, AudioURL: v.AudioURL, Timestamps: v.Timestamps,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return story, nil
}

// --- Paragraphs ---

func (r *StoryRepo) AddParagraph(p *model.Paragraph) error {
	err := r.db.QueryRow(
		`INSERT INTO paragraphs (story_id, position, content, audio_url)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		p.StoryID, p.Position, p.Content, p.AudioURL,
	).Scan(&p.ID)
	if err != nil {
		return err
	}

	for i, url := range p.Images {
		_, err = r.db.Exec(
			`INSERT INTO paragraph_images (paragraph_id, image_url, position)
			 VALUES ($1, $2, $3)`,
			p.ID, url.ImageURL, i,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *StoryRepo) SetParagraphAudio(id int, url string) error {
	_, err := r.db.Exec(
		`UPDATE paragraphs SET audio_url = $1 WHERE id = $2`, url, id,
	)
	return err
}

func (r *StoryRepo) GetParagraphByID(id int) (*model.Paragraph, error) {
	p := &model.Paragraph{}
	err := r.db.QueryRow(
		`SELECT id, story_id, position, content, COALESCE(audio_url,'')
		 FROM paragraphs WHERE id = $1`, id,
	).Scan(&p.ID, &p.StoryID, &p.Position, &p.Content, &p.AudioURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	images, err := r.listImages(p.ID)
	if err != nil {
		return nil, err
	}
	p.Images = images

	return p, nil
}

func (r *StoryRepo) listParagraphs(storyID int) ([]model.Paragraph, error) {
	rows, err := r.db.Query(
		`SELECT id, story_id, position, content, COALESCE(audio_url,'')
		 FROM paragraphs WHERE story_id = $1 ORDER BY position`, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paragraphs []model.Paragraph = []model.Paragraph{}
	for rows.Next() {
		var p model.Paragraph
		if err := rows.Scan(&p.ID, &p.StoryID, &p.Position, &p.Content, &p.AudioURL); err != nil {
			return nil, err
		}

		images, err := r.listImages(p.ID)
		if err != nil {
			return nil, err
		}
		p.Images = images

		translations, err := r.listTranslations(p.ID)
		if err != nil {
			return nil, err
		}
		p.Translations = translations

		vocab, err := r.listVocabulary(p.ID)
		if err != nil {
			return nil, err
		}
		p.Vocabulary = vocab

		words, err := r.listParagraphWordTimestamps(p.ID)
		if err != nil {
			return nil, err
		}
		p.WordTimestamps = words

		paragraphs = append(paragraphs, p)
	}
	return paragraphs, nil
}

func (r *StoryRepo) listParagraphWordTimestamps(paragraphID int) ([]model.WordTimestamp, error) {
	rows, err := r.db.Query(
		`SELECT word_index, word, start_ms, end_ms
		 FROM paragraph_word_timestamps
		 WHERE paragraph_id = $1
		 ORDER BY word_index`, paragraphID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	words := []model.WordTimestamp{}
	for rows.Next() {
		w := model.WordTimestamp{ParagraphID: paragraphID}
		if err := rows.Scan(&w.WordIndex, &w.Word, &w.StartMs, &w.EndMs); err != nil {
			return nil, err
		}
		words = append(words, w)
	}
	return words, nil
}

// SaveParagraphWordTimestamps replaces the word timestamps for a paragraph
// (called after regenerating that paragraph's audio).
func (r *StoryRepo) SaveParagraphWordTimestamps(paragraphID int, words []model.WordTimestamp) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM paragraph_word_timestamps WHERE paragraph_id = $1`, paragraphID); err != nil {
		return err
	}

	if len(words) > 0 {
		stmt, err := tx.Prepare(
			`INSERT INTO paragraph_word_timestamps (paragraph_id, word_index, word, start_ms, end_ms)
			 VALUES ($1, $2, $3, $4, $5)`,
		)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, wt := range words {
			if _, err := stmt.Exec(paragraphID, wt.WordIndex, wt.Word, wt.StartMs, wt.EndMs); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (r *StoryRepo) listImages(paragraphID int) ([]model.ParagraphImage, error) {
	rows, err := r.db.Query(
		`SELECT id, paragraph_id, image_url, position FROM paragraph_images WHERE paragraph_id = $1 ORDER BY position`,
		paragraphID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []model.ParagraphImage = []model.ParagraphImage{}
	for rows.Next() {
		var img model.ParagraphImage
		if err := rows.Scan(&img.ID, &img.ParagraphID, &img.ImageURL, &img.Position); err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, nil
}

func (r *StoryRepo) AddParagraphImage(img *model.ParagraphImage) error {
	return r.db.QueryRow(
		`INSERT INTO paragraph_images (paragraph_id, image_url, position)
		 VALUES ($1, $2, $3) RETURNING id`,
		img.ParagraphID, img.ImageURL, img.Position,
	).Scan(&img.ID)
}

func (r *StoryRepo) DeleteParagraphImage(id int) error {
	_, err := r.db.Exec(`DELETE FROM paragraph_images WHERE id = $1`, id)
	return err
}


// --- Translations ---

func (r *StoryRepo) AddTranslation(t *model.ParagraphTranslation) error {
	return r.db.QueryRow(
		`INSERT INTO paragraph_translations (paragraph_id, language, content)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (paragraph_id, language) DO UPDATE SET content = EXCLUDED.content
		 RETURNING id`,
		t.ParagraphID, t.Language, t.Content,
	).Scan(&t.ID)
}

func (r *StoryRepo) listTranslations(paragraphID int) ([]model.ParagraphTranslation, error) {
	rows, err := r.db.Query(
		`SELECT id, paragraph_id, language, content, COALESCE(audio_url, '') FROM paragraph_translations WHERE paragraph_id = $1`,
		paragraphID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.ParagraphTranslation
	for rows.Next() {
		var t model.ParagraphTranslation
		if err := rows.Scan(&t.ID, &t.ParagraphID, &t.Language, &t.Content, &t.AudioURL); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func (r *StoryRepo) GetTranslationByLang(paragraphID int, lang string) (*model.ParagraphTranslation, error) {
	t := &model.ParagraphTranslation{}
	err := r.db.QueryRow(
		`SELECT id, paragraph_id, language, content, COALESCE(audio_url, '') FROM paragraph_translations WHERE paragraph_id = $1 AND language = $2`,
		paragraphID, lang,
	).Scan(&t.ID, &t.ParagraphID, &t.Language, &t.Content, &t.AudioURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (r *StoryRepo) SetTranslationAudio(paragraphID int, lang string, url string) error {
	_, err := r.db.Exec(
		`UPDATE paragraph_translations SET audio_url = $1 WHERE paragraph_id = $2 AND language = $3`,
		url, paragraphID, lang,
	)
	return err
}

// --- Vocabulary ---

func (r *StoryRepo) AddVocabulary(v *model.Vocabulary) error {
	return r.db.QueryRow(
		`INSERT INTO vocabulary (paragraph_id, word, definition) VALUES ($1, $2, $3) RETURNING id`,
		v.ParagraphID, v.Word, v.Definition,
	).Scan(&v.ID)
}

func (r *StoryRepo) listVocabulary(paragraphID int) ([]model.Vocabulary, error) {
	rows, err := r.db.Query(
		`SELECT id, paragraph_id, word, definition FROM vocabulary WHERE paragraph_id = $1`,
		paragraphID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Vocabulary
	for rows.Next() {
		var v model.Vocabulary
		if err := rows.Scan(&v.ID, &v.ParagraphID, &v.Word, &v.Definition); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

// --- Story-linked phrase playlists ---

// normalizeWord lowercases and strips everything except letters, digits and apostrophes,
// matching the frontend's phrase-matching normalization.
func normalizeWord(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '\'' {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// AudioSegment is a [start, end] ms window of a story's audio file.
type AudioSegment struct {
	AudioURL string
	StartMs  int
	EndMs    int
}

// StoryPlaylistRef identifies one of the user's story playlists.
type StoryPlaylistRef struct {
	ID   int
	Name string
}

// FindStorySegment locates a contiguous phrase inside a story's audio using the
// per-word timestamps, returning the matching paragraph's audio segment. Returns
// nil (no error) when the phrase can't be matched to any paragraph that has audio.
func (r *StoryRepo) FindStorySegment(storyID int, phrase string) (*AudioSegment, error) {
	var tokens []string
	for _, t := range strings.Fields(phrase) {
		if n := normalizeWord(t); n != "" {
			tokens = append(tokens, n)
		}
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	rows, err := r.db.Query(
		`SELECT id, COALESCE(audio_url, '') FROM paragraphs WHERE story_id = $1 ORDER BY position`, storyID,
	)
	if err != nil {
		return nil, err
	}
	type para struct {
		id  int
		url string
	}
	var paras []para
	for rows.Next() {
		var p para
		if err := rows.Scan(&p.id, &p.url); err != nil {
			rows.Close()
			return nil, err
		}
		paras = append(paras, p)
	}
	rows.Close()

	for _, p := range paras {
		if p.url == "" {
			continue
		}
		words, err := r.listParagraphWordTimestamps(p.id)
		if err != nil {
			return nil, err
		}
		if len(words) == 0 {
			continue
		}
		for i := 0; i+len(tokens) <= len(words); i++ {
			match := true
			for j := 0; j < len(tokens); j++ {
				if normalizeWord(words[i+j].Word) != tokens[j] {
					match = false
					break
				}
			}
			if match {
				return &AudioSegment{
					AudioURL: p.url,
					StartMs:  words[i].StartMs,
					EndMs:    words[i+len(tokens)-1].EndMs,
				}, nil
			}
		}
	}
	return nil, nil
}

// PlaylistsContainingStory returns the story playlists that contain the story and
// that the user can access — both owned and playlists shared with the user. This
// lets a shared user build their own vocabulary playlist for a shared collection.
func (r *StoryRepo) PlaylistsContainingStory(userID string, storyID int) ([]StoryPlaylistRef, error) {
	rows, err := r.db.Query(
		`SELECT DISTINCT p.id, p.name
		 FROM playlists p
		 JOIN playlist_stories ps ON ps.playlist_id = p.id
		 LEFT JOIN playlist_shares sh ON sh.playlist_id = p.id AND sh.user_id = $1
		 WHERE ps.story_id = $2 AND (p.user_id = $1 OR sh.user_id = $1)
		 ORDER BY p.name`, userID, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoryPlaylistRef
	for rows.Next() {
		var ref StoryPlaylistRef
		if err := rows.Scan(&ref.ID, &ref.Name); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}

// UpsertStoryPlaylistPhrase adds (or refreshes) a saved word in the phrase playlist
// tied to a story playlist, creating that playlist and its default group on first use.
// seg may be nil (the word is stored without audio until its paragraph audio exists).
func (r *StoryRepo) UpsertStoryPlaylistPhrase(userID string, storyPlaylistID int, storyPlaylistName, language, text string, sourceStoryID int, seg *AudioSegment) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Find or create the word playlist for this story playlist.
	var playlistID int
	err = tx.QueryRow(
		`SELECT id FROM phrase_playlists WHERE user_id = $1 AND story_playlist_id = $2`, userID, storyPlaylistID,
	).Scan(&playlistID)
	if err == sql.ErrNoRows {
		if err := tx.QueryRow(
			`INSERT INTO phrase_playlists (user_id, name, description, language, story_playlist_id)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			userID, "📖 "+storyPlaylistName, "Words saved from stories in '"+storyPlaylistName+"'", language, storyPlaylistID,
		).Scan(&playlistID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Find or create the default "Words" group.
	var groupID int
	err = tx.QueryRow(
		`SELECT id FROM phrase_groups WHERE phrase_playlist_id = $1 ORDER BY position, id LIMIT 1`, playlistID,
	).Scan(&groupID)
	if err == sql.ErrNoRows {
		if err := tx.QueryRow(
			`INSERT INTO phrase_groups (phrase_playlist_id, name, position) VALUES ($1, 'Words', 0) RETURNING id`,
			playlistID,
		).Scan(&groupID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	var audioURL any
	var startMs, endMs any
	if seg != nil {
		audioURL, startMs, endMs = seg.AudioURL, seg.StartMs, seg.EndMs
	}

	// Dedup by (text, source story): the same word is one entry within a story, but
	// the same word saved from a DIFFERENT story becomes a new, independent phrase.
	var existing int
	err = tx.QueryRow(
		`SELECT id FROM phrases
		 WHERE phrase_group_id = $1 AND lower(text) = lower($2)
		   AND source_story_id IS NOT DISTINCT FROM $3
		 LIMIT 1`,
		groupID, text, sourceStoryID,
	).Scan(&existing)
	if err == sql.ErrNoRows {
		var pos int
		if err := tx.QueryRow(
			`SELECT COALESCE(MAX(position), -1) + 1 FROM phrases WHERE phrase_group_id = $1`, groupID,
		).Scan(&pos); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO phrases (phrase_group_id, text, translation_es, pronunciation_es, position,
			                      source_story_id, source_audio_url, source_start_ms, source_end_ms)
			 VALUES ($1, $2, '', '', $3, $4, $5, $6, $7)`,
			groupID, text, pos, sourceStoryID, audioURL, startMs, endMs,
		); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if seg != nil {
		// Only refresh the segment when we have a new one (don't clobber existing audio with nulls).
		if _, err := tx.Exec(
			`UPDATE phrases SET source_story_id = $2, source_audio_url = $3, source_start_ms = $4, source_end_ms = $5 WHERE id = $1`,
			existing, sourceStoryID, audioURL, startMs, endMs,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// --- Voices ---

func (r *StoryRepo) AddVoice(v *model.StoryVoice) error {
	ts, err := json.Marshal(v.Timestamps)
	if err != nil {
		return err
	}
	return r.db.QueryRow(
		`INSERT INTO story_voices (story_id, name, audio_url, timestamps)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		v.StoryID, v.Name, v.AudioURL, ts,
	).Scan(&v.ID)
}

func (r *StoryRepo) listVoices(storyID int) ([]model.StoryVoice, error) {
	rows, err := r.db.Query(
		`SELECT id, story_id, name, audio_url, COALESCE(timestamps, '[]'::jsonb)
		 FROM story_voices WHERE story_id = $1`, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var voices []model.StoryVoice
	for rows.Next() {
		var v model.StoryVoice
		var tsRaw []byte
		if err := rows.Scan(&v.ID, &v.StoryID, &v.Name, &v.AudioURL, &tsRaw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tsRaw, &v.Timestamps)
		voices = append(voices, v)
	}

	for i := range voices {
		words, err := r.listWordTimestamps(voices[i].ID)
		if err != nil {
			return nil, err
		}
		voices[i].WordTimestamps = words
	}
	return voices, nil
}

func (r *StoryRepo) listWordTimestamps(voiceID int) ([]model.WordTimestamp, error) {
	rows, err := r.db.Query(
		`SELECT paragraph_id, word_index, word, start_ms, end_ms
		 FROM story_voice_word_timestamps
		 WHERE voice_id = $1
		 ORDER BY paragraph_id, word_index`, voiceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	words := []model.WordTimestamp{}
	for rows.Next() {
		var w model.WordTimestamp
		if err := rows.Scan(&w.ParagraphID, &w.WordIndex, &w.Word, &w.StartMs, &w.EndMs); err != nil {
			return nil, err
		}
		words = append(words, w)
	}
	return words, nil
}

// SaveVoiceWithTimestamps inserts a story voice + its paragraph and word timestamps atomically.
func (r *StoryRepo) SaveVoiceWithTimestamps(storyID int, name, audioURL string, para []model.VoiceTimestamp, words []model.WordTimestamp) (*model.StoryVoice, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ts, err := json.Marshal(para)
	if err != nil {
		return nil, err
	}

	var vID int
	if err := tx.QueryRow(
		`INSERT INTO story_voices (story_id, name, audio_url, timestamps)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		storyID, name, audioURL, ts,
	).Scan(&vID); err != nil {
		return nil, err
	}

	if len(words) > 0 {
		stmt, err := tx.Prepare(
			`INSERT INTO story_voice_word_timestamps
			 (voice_id, paragraph_id, word_index, word, start_ms, end_ms)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
		)
		if err != nil {
			return nil, err
		}
		defer stmt.Close()
		for _, w := range words {
			if _, err := stmt.Exec(vID, w.ParagraphID, w.WordIndex, w.Word, w.StartMs, w.EndMs); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &model.StoryVoice{
		ID:             vID,
		StoryID:        storyID,
		Name:           name,
		AudioURL:       audioURL,
		Timestamps:     para,
		WordTimestamps: words,
	}, nil
}

// --- Reviews & Stats ---

func (r *StoryRepo) AddReview(userID string, storyID int) error {
	_, err := r.db.Exec(
		`INSERT INTO user_story_reviews (user_id, story_id) VALUES ($1, $2)`,
		userID, storyID,
	)
	return err
}

func (r *StoryRepo) GetUserStats(userID string) (*model.UserStats, error) {
	stats := &model.UserStats{
		DailyReviews:   []model.StatPeriod{},
		MonthlyReviews: []model.StatPeriod{},
		YearlyReviews:  []model.StatPeriod{},
		HistorySummary: []model.StorySummary{},
	}

	if err := r.db.QueryRow(
		`SELECT COUNT(*) FROM user_story_reviews WHERE user_id = $1`, userID,
	).Scan(&stats.TotalReviews); err != nil {
		return nil, err
	}

	// Daily (last 30 days with activity; frontend fills the gaps)
	rows, err := r.db.Query(`
		SELECT TO_CHAR(reviewed_at, 'YYYY-MM-DD') as period, COUNT(*)
		FROM user_story_reviews
		WHERE user_id = $1
		GROUP BY period ORDER BY period DESC LIMIT 30`, userID)
	if err != nil {
		log.Printf("GetUserStats daily query error: %v", err)
	} else {
		for rows.Next() {
			var p model.StatPeriod
			if err := rows.Scan(&p.Period, &p.Count); err != nil {
				log.Printf("GetUserStats daily scan error: %v", err)
				continue
			}
			stats.DailyReviews = append(stats.DailyReviews, p)
		}
		if err := rows.Close(); err != nil {
			log.Printf("GetUserStats daily rows error: %v", err)
		}
	}

	// Monthly
	rows, err = r.db.Query(`
		SELECT TO_CHAR(reviewed_at, 'YYYY-MM') as period, COUNT(*)
		FROM user_story_reviews
		WHERE user_id = $1
		GROUP BY period ORDER BY period DESC`, userID)
	if err != nil {
		log.Printf("GetUserStats monthly query error: %v", err)
	} else {
		for rows.Next() {
			var p model.StatPeriod
			if err := rows.Scan(&p.Period, &p.Count); err != nil {
				log.Printf("GetUserStats monthly scan error: %v", err)
				continue
			}
			stats.MonthlyReviews = append(stats.MonthlyReviews, p)
		}
		if err := rows.Close(); err != nil {
			log.Printf("GetUserStats monthly rows error: %v", err)
		}
	}

	// Yearly
	rows, err = r.db.Query(`
		SELECT TO_CHAR(reviewed_at, 'YYYY') as period, COUNT(*)
		FROM user_story_reviews
		WHERE user_id = $1
		GROUP BY period ORDER BY period DESC`, userID)
	if err != nil {
		log.Printf("GetUserStats yearly query error: %v", err)
	} else {
		for rows.Next() {
			var p model.StatPeriod
			if err := rows.Scan(&p.Period, &p.Count); err != nil {
				log.Printf("GetUserStats yearly scan error: %v", err)
				continue
			}
			stats.YearlyReviews = append(stats.YearlyReviews, p)
		}
		if err := rows.Close(); err != nil {
			log.Printf("GetUserStats yearly rows error: %v", err)
		}
	}

	// History by Story
	rows, err = r.db.Query(`
		SELECT s.id, s.title, COUNT(r.id), MAX(r.reviewed_at)
		FROM stories s
		JOIN user_story_reviews r ON r.story_id = s.id
		WHERE r.user_id = $1
		GROUP BY s.id, s.title
		ORDER BY MAX(r.reviewed_at) DESC`, userID)
	if err != nil {
		log.Printf("GetUserStats history query error: %v", err)
	} else {
		for rows.Next() {
			var s model.StorySummary
			if err := rows.Scan(&s.StoryID, &s.Title, &s.ReviewCount, &s.LastReviewed); err != nil {
				log.Printf("GetUserStats history scan error: %v", err)
				continue
			}
			stats.HistorySummary = append(stats.HistorySummary, s)
		}
		if err := rows.Close(); err != nil {
			log.Printf("GetUserStats history rows error: %v", err)
		}
	}

	// Streak: consecutive days with at least one review ending today or yesterday
	rows, err = r.db.Query(`
		SELECT TO_CHAR(reviewed_at, 'YYYY-MM-DD')
		FROM user_story_reviews
		WHERE user_id = $1
		GROUP BY TO_CHAR(reviewed_at, 'YYYY-MM-DD')
		ORDER BY 1 DESC`, userID)
	if err != nil {
		log.Printf("GetUserStats streak query error: %v", err)
	} else {
		var dates []string
		for rows.Next() {
			var d string
			if err := rows.Scan(&d); err != nil {
				log.Printf("GetUserStats streak scan error: %v", err)
				continue
			}
			dates = append(dates, d)
		}
		rows.Close()

		if len(dates) > 0 {
			today := time.Now().UTC().Format("2006-01-02")
			yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
			// Streak must start from today or yesterday to be active
			if dates[0] == today || dates[0] == yesterday {
				expected := dates[0]
				for _, d := range dates {
					if d != expected {
						break
					}
					stats.Streak++
					t, _ := time.Parse("2006-01-02", expected)
					expected = t.AddDate(0, 0, -1).Format("2006-01-02")
				}
			}
		}
	}

	return stats, nil
}

// --- Playlists ---

func (r *StoryRepo) CreatePlaylist(p *model.Playlist) error {
	return r.db.QueryRow(
		`INSERT INTO playlists (user_id, name, description) VALUES ($1, $2, $3)
		 RETURNING id, created_at, updated_at`,
		p.UserID, p.Name, p.Description,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *StoryRepo) UpdatePlaylist(p *model.Playlist) error {
	_, err := r.db.Exec(
		`UPDATE playlists SET name = $1, description = $2, updated_at = NOW()
		 WHERE id = $3 AND user_id = $4`,
		p.Name, p.Description, p.ID, p.UserID,
	)
	return err
}

func (r *StoryRepo) ListPlaylists(userID string) ([]model.Playlist, error) {
	// Owned playlists + playlists shared with the user. Owned first, then favorites, newest.
	rows, err := r.db.Query(`
		SELECT p.id, p.user_id, p.name, p.description,
		       EXISTS(SELECT 1 FROM playlist_favorites f WHERE f.playlist_id = p.id AND f.user_id = $1::uuid) AS is_favorite,
		       p.created_at, p.updated_at,
		       (SELECT COUNT(*) FROM playlist_stories WHERE playlist_id = p.id) AS story_count,
		       CASE WHEN p.user_id = $1::uuid THEN 'owner' ELSE COALESCE(sh.permission, '') END AS role,
		       CASE WHEN p.user_id = $1::uuid THEN '' ELSE owner."fullName" END AS owner_name
		FROM playlists p
		JOIN users owner ON owner.id = p.user_id
		LEFT JOIN playlist_shares sh ON sh.playlist_id = p.id AND sh.user_id = $1::uuid
		WHERE p.user_id = $1::uuid OR sh.user_id = $1::uuid
		ORDER BY (p.user_id = $1::uuid) DESC, is_favorite DESC, p.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Playlist = []model.Playlist{}
	for rows.Next() {
		var p model.Playlist
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.IsFavorite, &p.CreatedAt, &p.UpdatedAt, &p.StoryCount, &p.Role, &p.OwnerName); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

// --- Story playlist sharing ---

func (r *StoryRepo) PlaylistOwner(playlistID int) (string, error) {
	var ownerID string
	err := r.db.QueryRow(`SELECT user_id FROM playlists WHERE id = $1`, playlistID).Scan(&ownerID)
	return ownerID, err
}

func (r *StoryRepo) ListPlaylistShares(playlistID int) ([]model.PlaylistShare, error) {
	rows, err := r.db.Query(
		`SELECT u.id, u."fullName", u.email, s.permission, s.created_at
		 FROM playlist_shares s JOIN users u ON u.id = s.user_id
		 WHERE s.playlist_id = $1 ORDER BY s.created_at DESC`, playlistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.PlaylistShare{}
	for rows.Next() {
		var s model.PlaylistShare
		if err := rows.Scan(&s.UserID, &s.FullName, &s.Email, &s.Permission, &s.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, nil
}

func (r *StoryRepo) SearchPlaylistShareCandidates(playlistID int, ownerID, like string) ([]model.ShareCandidate, error) {
	rows, err := r.db.Query(
		`SELECT id, "fullName", email FROM users
		 WHERE (lower("fullName") LIKE $1 OR lower(email) LIKE $1)
		   AND id != $2
		   AND id NOT IN (SELECT user_id FROM playlist_shares WHERE playlist_id = $3)
		 ORDER BY "fullName" ASC LIMIT 5`, like, ownerID, playlistID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.ShareCandidate{}
	for rows.Next() {
		var c model.ShareCandidate
		if err := rows.Scan(&c.UserID, &c.FullName, &c.Email); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}

func (r *StoryRepo) UserIDByEmail(email string) (string, error) {
	var id string
	err := r.db.QueryRow(`SELECT id FROM users WHERE lower(email) = $1`, email).Scan(&id)
	return id, err
}

func (r *StoryRepo) AddPlaylistShare(playlistID int, targetID, permission string) error {
	_, err := r.db.Exec(
		`INSERT INTO playlist_shares (playlist_id, user_id, permission)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (playlist_id, user_id) DO UPDATE SET permission = EXCLUDED.permission`,
		playlistID, targetID, permission,
	)
	return err
}

func (r *StoryRepo) GetPlaylistShare(playlistID int, targetID string) (*model.PlaylistShare, error) {
	var s model.PlaylistShare
	err := r.db.QueryRow(
		`SELECT u.id, u."fullName", u.email, s.permission, s.created_at
		 FROM playlist_shares s JOIN users u ON u.id = s.user_id
		 WHERE s.playlist_id = $1 AND s.user_id = $2`, playlistID, targetID,
	).Scan(&s.UserID, &s.FullName, &s.Email, &s.Permission, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *StoryRepo) UpdatePlaylistShare(playlistID int, targetID, permission string) (int64, error) {
	res, err := r.db.Exec(
		`UPDATE playlist_shares SET permission = $1 WHERE playlist_id = $2 AND user_id = $3`,
		permission, playlistID, targetID,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *StoryRepo) RemovePlaylistShare(playlistID int, targetID string) error {
	_, err := r.db.Exec(`DELETE FROM playlist_shares WHERE playlist_id = $1 AND user_id = $2`, playlistID, targetID)
	return err
}

// SetPlaylistFavorite toggles the current user's own favorite (per-user, works for
// owners and users the playlist was shared with).
func (r *StoryRepo) SetPlaylistFavorite(id int, userID string, isFavorite bool) error {
	if isFavorite {
		_, err := r.db.Exec(
			`INSERT INTO playlist_favorites (playlist_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			id, userID,
		)
		return err
	}
	_, err := r.db.Exec(
		`DELETE FROM playlist_favorites WHERE playlist_id = $1 AND user_id = $2`, id, userID,
	)
	return err
}

// HasPlaylistAccess reports whether the user owns or has a share for the playlist.
func (r *StoryRepo) HasPlaylistAccess(userID string, playlistID int) (bool, error) {
	var ok bool
	err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM playlists p
			LEFT JOIN playlist_shares s ON s.playlist_id = p.id AND s.user_id = $1
			WHERE p.id = $2 AND (p.user_id = $1 OR s.user_id = $1)
		)`, userID, playlistID,
	).Scan(&ok)
	return ok, err
}

func (r *StoryRepo) DeletePlaylist(id int, userID string) error {
	_, err := r.db.Exec(`DELETE FROM playlists WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (r *StoryRepo) AddStoryToPlaylist(playlistID, storyID int) error {
	_, err := r.db.Exec(
		`INSERT INTO playlist_stories (playlist_id, story_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		playlistID, storyID,
	)
	return err
}

func (r *StoryRepo) RemoveStoryFromPlaylist(playlistID, storyID int) error {
	_, err := r.db.Exec(
		`DELETE FROM playlist_stories WHERE playlist_id = $1 AND story_id = $2`,
		playlistID, storyID,
	)
	return err
}

// --- User Vocabulary ---

func (r *StoryRepo) AddUserVocabulary(v *model.UserVocabulary) error {
	return r.db.QueryRow(
		`WITH next_pos AS (
		   SELECT COALESCE(MAX(position), -1) + 1 AS pos
		   FROM user_story_vocabulary
		   WHERE user_id = $1 AND story_id = $2
		 )
		 INSERT INTO user_story_vocabulary (user_id, story_id, phrase, position)
		 SELECT $1, $2, $3, pos FROM next_pos
		 RETURNING id, position, audio_url, created_at`,
		v.UserID, v.StoryID, v.Phrase,
	).Scan(&v.ID, &v.Position, &v.AudioURL, &v.CreatedAt)
}

func (r *StoryRepo) ListUserVocabulary(userID string, storyID int) ([]model.UserVocabulary, error) {
	rows, err := r.db.Query(
		`SELECT id, user_id, story_id, phrase, position, audio_url, created_at
		 FROM user_story_vocabulary
		 WHERE user_id = $1 AND story_id = $2
		 ORDER BY position ASC`,
		userID, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.UserVocabulary = []model.UserVocabulary{}
	for rows.Next() {
		var v model.UserVocabulary
		if err := rows.Scan(&v.ID, &v.UserID, &v.StoryID, &v.Phrase, &v.Position, &v.AudioURL, &v.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

func (r *StoryRepo) GetUserVocabByID(id int, userID string) (*model.UserVocabulary, error) {
	var v model.UserVocabulary
	err := r.db.QueryRow(
		`SELECT id, user_id, story_id, phrase, position, audio_url, created_at
		 FROM user_story_vocabulary WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&v.ID, &v.UserID, &v.StoryID, &v.Phrase, &v.Position, &v.AudioURL, &v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *StoryRepo) UpdateVocabAudioURL(id int, userID, audioURL string) error {
	_, err := r.db.Exec(
		`UPDATE user_story_vocabulary SET audio_url = $1 WHERE id = $2 AND user_id = $3`,
		audioURL, id, userID,
	)
	return err
}

func (r *StoryRepo) ReorderUserVocabulary(userID string, storyID int, ids []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for pos, id := range ids {
		if _, err := tx.Exec(
			`UPDATE user_story_vocabulary SET position = $1
			 WHERE id = $2 AND user_id = $3 AND story_id = $4`,
			pos, id, userID, storyID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *StoryRepo) DeleteUserVocabulary(id int, userID string) error {
	_, err := r.db.Exec(`DELETE FROM user_story_vocabulary WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// RemoveStoryPlaylistPhrases deletes the matching phrase from all of the user's
// story-linked word playlists (same source story + text). Cascades drop its
// reviews / zen listens / SRS — losing those stats is intentional here.
func (r *StoryRepo) RemoveStoryPlaylistPhrases(userID string, storyID int, phrase string) error {
	_, err := r.db.Exec(`
		DELETE FROM phrases ph
		USING phrase_groups g, phrase_playlists pp
		WHERE ph.phrase_group_id = g.id
		  AND g.phrase_playlist_id = pp.id
		  AND pp.user_id = $1
		  AND pp.story_playlist_id IS NOT NULL
		  AND ph.source_story_id = $2
		  AND lower(ph.text) = lower($3)`,
		userID, storyID, phrase,
	)
	return err
}

// --- Zen Mode ---

func (r *StoryRepo) ListZen(userID string, playlistID, limit int, sort string) ([]model.Story, error) {
	query := `
		SELECT s.id, s.title, s.level, s.cover_url, s.author, s.status, s.created_at, s.updated_at,
		       c.id, c.name, c.slug,
		       (SELECT COUNT(*) FROM zen_listens z WHERE z.story_id = s.id AND z.user_id = $1) AS review_count,
		       (SELECT MAX(listened_at) FROM zen_listens z WHERE z.story_id = s.id AND z.user_id = $1) AS last_reviewed_at
		FROM stories s
		JOIN categories c ON c.id = s.category_id`

	args := []interface{}{userID}
	where := []string{"s.status != 'deleted'"}

	if playlistID > 0 {
		query += ` JOIN playlist_stories ps ON ps.story_id = s.id`
		where = append(where, fmt.Sprintf("ps.playlist_id = $%d", len(args)+1))
		args = append(args, playlistID)
	}

	query += " WHERE " + strings.Join(where, " AND ")
	query += ` GROUP BY s.id, c.id, c.name, c.slug`

	switch sort {
	case "newest":
		query += ` ORDER BY s.created_at DESC, s.id ASC`
	case "oldest":
		query += ` ORDER BY s.created_at ASC, s.id ASC`
	case "least_played":
		query += ` ORDER BY review_count ASC, last_reviewed_at ASC NULLS FIRST, s.id ASC`
	default:
		query += ` ORDER BY RANDOM()`
	}

	if limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, len(args)+1)
		args = append(args, limit)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stories []model.Story = []model.Story{}
	for rows.Next() {
		var s model.Story
		var cat model.Category
		var lastReviewedAt sql.NullTime
		if err := rows.Scan(
			&s.ID, &s.Title, &s.Level, &s.CoverURL, &s.Author, &s.Status, &s.CreatedAt, &s.UpdatedAt,
			&cat.ID, &cat.Name, &cat.Slug,
			&s.ReviewCount, &lastReviewedAt,
		); err != nil {
			return nil, err
		}
		if lastReviewedAt.Valid {
			s.LastReviewedAt = &lastReviewedAt.Time
		}
		s.Category = &cat
		stories = append(stories, s)
	}
	return stories, nil
}

func (r *StoryRepo) LogZenListen(userID string, storyID int) error {
	_, err := r.db.Exec(
		`INSERT INTO zen_listens (user_id, story_id) VALUES ($1, $2)`,
		userID, storyID,
	)
	return err
}

// --- Sentences ---

func (r *StoryRepo) AddSentences(sentences []model.StorySentence) error {
	if len(sentences) == 0 {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM story_sentences WHERE story_id = $1`, sentences[0].StoryID)
	if err != nil {
		return err
	}

	for _, s := range sentences {
		_, err = tx.Exec(
			`INSERT INTO story_sentences (story_id, paragraph_id, en, es, position)
			 VALUES ($1, $2, $3, $4, $5)`,
			s.StoryID, s.ParagraphID, s.En, s.Es, s.Position,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *StoryRepo) ListSentences(storyID int) ([]model.StorySentence, error) {
	rows, err := r.db.Query(
		`SELECT id, story_id, paragraph_id, en, es, position, created_at
		 FROM story_sentences WHERE story_id = $1 ORDER BY position`,
		storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.StorySentence = []model.StorySentence{}
	for rows.Next() {
		var s model.StorySentence
		if err := rows.Scan(&s.ID, &s.StoryID, &s.ParagraphID, &s.En, &s.Es, &s.Position, &s.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func (r *StoryRepo) GetSentenceByID(id int) (*model.StorySentence, error) {
	s := &model.StorySentence{}
	err := r.db.QueryRow(
		`SELECT id, story_id, paragraph_id, en, es, position, created_at
		 FROM story_sentences WHERE id = $1`, id,
	).Scan(&s.ID, &s.StoryID, &s.ParagraphID, &s.En, &s.Es, &s.Position, &s.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *StoryRepo) AddSentenceAttempt(a *model.UserSentenceAttempt) error {
	return r.db.QueryRow(
		`INSERT INTO user_sentence_attempts (user_id, sentence_id, is_correct, user_answer)
		 VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
		a.UserID, a.SentenceID, a.IsCorrect, a.UserAnswer,
	).Scan(&a.ID, &a.CreatedAt)
}

func (r *StoryRepo) GetSentenceStats(userID string, storyID int) ([]model.SentenceStats, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.en, s.es,
		       COUNT(a.id) FILTER (WHERE a.is_correct = true) as correct_count,
		       COUNT(a.id) FILTER (WHERE a.is_correct = false) as failed_count,
		       COUNT(a.id) as total_attempts
		FROM story_sentences s
		LEFT JOIN user_sentence_attempts a ON a.sentence_id = s.id AND a.user_id = $1
		WHERE s.story_id = $2
		GROUP BY s.id, s.en, s.es, s.position
		ORDER BY s.position`,
		userID, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.SentenceStats = []model.SentenceStats{}
	for rows.Next() {
		var s model.SentenceStats
		if err := rows.Scan(&s.SentenceID, &s.En, &s.Es, &s.CorrectCount, &s.FailedCount, &s.TotalAttempts); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func (r *StoryRepo) GetSentenceHistory(userID string, sentenceID int) ([]model.UserSentenceAttempt, error) {
	rows, err := r.db.Query(
		`SELECT id, user_id, sentence_id, is_correct, COALESCE(user_answer, ''), created_at
		 FROM user_sentence_attempts
		 WHERE user_id = $1 AND sentence_id = $2
		 ORDER BY created_at DESC`,
		userID, sentenceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.UserSentenceAttempt = []model.UserSentenceAttempt{}
	for rows.Next() {
		var a model.UserSentenceAttempt
		if err := rows.Scan(&a.ID, &a.UserID, &a.SentenceID, &a.IsCorrect, &a.UserAnswer, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}
