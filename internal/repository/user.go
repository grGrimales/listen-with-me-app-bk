package repository

import (
	"database/sql"
	"errors"

	"github.com/lib/pq"

	"listen-with-me/backend/internal/model"
)

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) FindByEmail(email string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(
		`SELECT id, "fullName", email, password, roles, "isActive", COALESCE(target_language, 'en') FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.FullName, &u.Email, &u.Password, pq.Array(&u.Roles), &u.IsActive, &u.TargetLanguage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (r *UserRepo) FindByID(id string) (*model.User, error) {
	u := &model.User{}
	err := r.db.QueryRow(
		`SELECT id, "fullName", email, roles, "isActive", COALESCE(target_language, 'en') FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.FullName, &u.Email, pq.Array(&u.Roles), &u.IsActive, &u.TargetLanguage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (r *UserRepo) Create(u *model.User) error {
	if u.TargetLanguage == "" {
		u.TargetLanguage = "en"
	}
	return r.db.QueryRow(
		`INSERT INTO users ("fullName", email, password, roles, "isActive", target_language) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		u.FullName, u.Email, u.Password, pq.Array(u.Roles), u.IsActive, u.TargetLanguage,
	).Scan(&u.ID)
}

func (r *UserRepo) UpdateLanguage(userID string, lang string) error {
	_, err := r.db.Exec(
		`UPDATE users SET target_language = $1, updated_at = NOW() WHERE id = $2`,
		lang, userID,
	)
	return err
}

// ListAll returns every user (no password), ordered by name, for the admin share page.
func (r *UserRepo) ListAll() ([]model.User, error) {
	rows, err := r.db.Query(
		`SELECT id, "fullName", email, roles, "isActive", COALESCE(target_language, 'en') FROM users ORDER BY lower("fullName")`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []model.User{}
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.FullName, &u.Email, pq.Array(&u.Roles), &u.IsActive, &u.TargetLanguage); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// SetRole adds or removes one role and returns the user's resulting roles.
// ok is false when no user has that id.
func (r *UserRepo) SetRole(userID, role string, enabled bool) (roles []string, ok bool, err error) {
	q := `UPDATE users SET roles = array_append(array_remove(roles, $2), $2), updated_at = NOW() WHERE id = $1 RETURNING roles`
	if !enabled {
		q = `UPDATE users SET roles = array_remove(roles, $2), updated_at = NOW() WHERE id = $1 RETURNING roles`
	}
	err = r.db.QueryRow(q, userID, role).Scan(pq.Array(&roles))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return roles, err == nil, err
}
