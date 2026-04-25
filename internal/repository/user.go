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
		`SELECT id, "fullName", email, password, roles, "isActive" FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.FullName, &u.Email, &u.Password, pq.Array(&u.Roles), &u.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (r *UserRepo) Create(u *model.User) error {
	return r.db.QueryRow(
		`INSERT INTO users ("fullName", email, password, roles, "isActive") VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		u.FullName, u.Email, u.Password, pq.Array(u.Roles), u.IsActive,
	).Scan(&u.ID)
}
