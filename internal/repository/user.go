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
