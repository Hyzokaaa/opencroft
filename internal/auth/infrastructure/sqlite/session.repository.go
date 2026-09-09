package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Hyzokaaa/opencroft/internal/auth/domain/entities"
)

type SQLiteSessionRepository struct {
	store *Store
}

func NewSQLiteSessionRepository(store *Store) *SQLiteSessionRepository {
	return &SQLiteSessionRepository{store: store}
}

func (r *SQLiteSessionRepository) Create(ctx context.Context, session *entities.Session) error {
	_, err := r.store.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires, created) VALUES (?, ?, ?, ?)`,
		session.TokenHash, session.UserId,
		session.Expires.Format(time.RFC3339), session.Created.Format(time.RFC3339))
	return err
}

func (r *SQLiteSessionRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*entities.Session, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT token_hash, user_id, expires, created FROM sessions WHERE token_hash = ?`, tokenHash)

	var (
		props            entities.SessionProps
		expires, created string
	)
	err := row.Scan(&props.TokenHash, &props.UserId, &expires, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	props.Expires, _ = time.Parse(time.RFC3339, expires)
	props.Created, _ = time.Parse(time.RFC3339, created)
	return entities.NewSession(props), nil
}

func (r *SQLiteSessionRepository) Delete(ctx context.Context, tokenHash string) error {
	_, err := r.store.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (r *SQLiteSessionRepository) DeleteExpired(ctx context.Context) error {
	_, err := r.store.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}
