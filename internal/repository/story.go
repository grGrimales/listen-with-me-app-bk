package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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

func (r *StoryRepo) List(onlyPublished bool, playlistID int, userID string) ([]model.Story, error) {
	log.Printf("Listing stories (onlyPublished=%v, playlistID=%d)", onlyPublished, playlistID)
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
	query += ` ORDER BY last_reviewed_at ASC NULLS FIRST, review_count ASC`

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
		paragraphs = append(paragraphs, p)
	}
	return paragraphs, nil
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
		`SELECT id, paragraph_id, language, content FROM paragraph_translations WHERE paragraph_id = $1`,
		paragraphID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.ParagraphTranslation
	for rows.Next() {
		var t model.ParagraphTranslation
		if err := rows.Scan(&t.ID, &t.ParagraphID, &t.Language, &t.Content); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
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
	return voices, nil
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

	// Total reviews
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM user_story_reviews WHERE user_id = $1`, userID,
	).Scan(&stats.TotalReviews)
	if err != nil {
		return nil, err
	}

	// Daily
	rows, err := r.db.Query(`
		SELECT TO_CHAR(reviewed_at, 'YYYY-MM-DD') as period, COUNT(*) 
		FROM user_story_reviews 
		WHERE user_id = $1 
		GROUP BY period ORDER BY period DESC LIMIT 30`, userID)
	if err == nil {
		for rows.Next() {
			var p model.StatPeriod
			rows.Scan(&p.Period, &p.Count)
			stats.DailyReviews = append(stats.DailyReviews, p)
		}
		rows.Close()
	}

	// Monthly
	rows, err = r.db.Query(`
		SELECT TO_CHAR(reviewed_at, 'YYYY-MM') as period, COUNT(*) 
		FROM user_story_reviews 
		WHERE user_id = $1 
		GROUP BY period ORDER BY period DESC`, userID)
	if err == nil {
		for rows.Next() {
			var p model.StatPeriod
			rows.Scan(&p.Period, &p.Count)
			stats.MonthlyReviews = append(stats.MonthlyReviews, p)
		}
		rows.Close()
	}

	// Yearly
	rows, err = r.db.Query(`
		SELECT TO_CHAR(reviewed_at, 'YYYY') as period, COUNT(*) 
		FROM user_story_reviews 
		WHERE user_id = $1 
		GROUP BY period ORDER BY period DESC`, userID)
	if err == nil {
		for rows.Next() {
			var p model.StatPeriod
			rows.Scan(&p.Period, &p.Count)
			stats.YearlyReviews = append(stats.YearlyReviews, p)
		}
		rows.Close()
	}

	// History by Story
	rows, err = r.db.Query(`
		SELECT s.id, s.title, COUNT(r.id), MAX(r.reviewed_at)
		FROM stories s
		JOIN user_story_reviews r ON r.story_id = s.id
		WHERE r.user_id = $1
		GROUP BY s.id, s.title
		ORDER BY MAX(r.reviewed_at) DESC`, userID)
	if err == nil {
		for rows.Next() {
			var s model.StorySummary
			rows.Scan(&s.StoryID, &s.Title, &s.ReviewCount, &s.LastReviewed)
			stats.HistorySummary = append(stats.HistorySummary, s)
		}
		rows.Close()
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
	rows, err := r.db.Query(`
		SELECT p.id, p.user_id, p.name, p.description, p.is_favorite, p.created_at, p.updated_at,
		       (SELECT COUNT(*) FROM playlist_stories WHERE playlist_id = p.id) as story_count
		FROM playlists p
		WHERE p.user_id = $1::uuid
		ORDER BY p.is_favorite DESC, p.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Playlist = []model.Playlist{}
	for rows.Next() {
		var p model.Playlist
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.IsFavorite, &p.CreatedAt, &p.UpdatedAt, &p.StoryCount); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (r *StoryRepo) SetPlaylistFavorite(id int, userID string, isFavorite bool) error {
	_, err := r.db.Exec(
		`UPDATE playlists SET is_favorite = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3`,
		isFavorite, id, userID,
	)
	return err
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
		`INSERT INTO user_story_vocabulary (user_id, story_id, phrase)
		 VALUES ($1, $2, $3) RETURNING id, created_at`,
		v.UserID, v.StoryID, v.Phrase,
	).Scan(&v.ID, &v.CreatedAt)
}

func (r *StoryRepo) ListUserVocabulary(userID string, storyID int) ([]model.UserVocabulary, error) {
	rows, err := r.db.Query(
		`SELECT id, user_id, story_id, phrase, created_at
		 FROM user_story_vocabulary
		 WHERE user_id = $1 AND story_id = $2
		 ORDER BY created_at DESC`,
		userID, storyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.UserVocabulary = []model.UserVocabulary{}
	for rows.Next() {
		var v model.UserVocabulary
		if err := rows.Scan(&v.ID, &v.UserID, &v.StoryID, &v.Phrase, &v.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

func (r *StoryRepo) DeleteUserVocabulary(id int, userID string) error {
	_, err := r.db.Exec(`DELETE FROM user_story_vocabulary WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// --- Zen Mode ---

func (r *StoryRepo) ListZen(userID string, playlistID, limit int, sort string) ([]model.Story, error) {
	query := `
		SELECT s.id, s.title, s.level, s.cover_url, s.author, s.status, s.created_at, s.updated_at,
		       c.id, c.name, c.slug,
		       COUNT(DISTINCT r.id) AS review_count,
		       MAX(r.reviewed_at) AS last_reviewed_at
		FROM stories s
		JOIN categories c ON c.id = s.category_id
		LEFT JOIN user_story_reviews r ON r.story_id = s.id AND r.user_id = $1`

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
		query += ` ORDER BY s.created_at DESC`
	case "oldest":
		query += ` ORDER BY s.created_at ASC`
	case "least_played":
		query += ` ORDER BY COUNT(DISTINCT r.id) ASC, s.created_at ASC`
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
