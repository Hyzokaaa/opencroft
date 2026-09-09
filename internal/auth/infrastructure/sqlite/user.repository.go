package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/entities"
)

type SQLiteUserRepository struct {
	store *Store
}

func NewSQLiteUserRepository(store *Store) *SQLiteUserRepository {
	return &SQLiteUserRepository{store: store}
}

func (r *SQLiteUserRepository) Create(ctx context.Context, user *entities.User) error {
	_, err := r.store.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, created) VALUES (?, ?, ?, ?)`,
		user.GetId(), user.Username, user.PasswordHash, user.Created)
	return err
}

func (r *SQLiteUserRepository) FindByUsername(ctx context.Context, username string) (*entities.User, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created FROM users WHERE username = ?`, username)

	var props entities.UserProps
	err := row.Scan(&props.Id, &props.Username, &props.PasswordHash, &props.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entities.NewUser(props), nil
}

func (r *SQLiteUserRepository) FindAll(ctx context.Context) ([]*entities.User, error) {
	rows, err := r.store.db.QueryContext(ctx,
		`SELECT id, username, password_hash, created FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*entities.User{}
	for rows.Next() {
		var props entities.UserProps
		if err := rows.Scan(&props.Id, &props.Username, &props.PasswordHash, &props.Created); err != nil {
			return nil, err
		}
		users = append(users, entities.NewUser(props))
	}
	return users, rows.Err()
}

func (r *SQLiteUserRepository) Count(ctx context.Context) (int, error) {
	var total int
	err := r.store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
	return total, err
}

func (r *SQLiteUserRepository) Delete(ctx context.Context, username string) error {
	_, err := r.store.db.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	return err
}

func (r *SQLiteUserRepository) UpdatePassword(ctx context.Context, username, hash string) error {
	_, err := r.store.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE username = ?`, hash, username)
	return err
}
