package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Hyzokaaa/opencroft/internal/autodeploy"
)

// AutoDeployRepository keeps which services deploy themselves. The secret is
// a webhook's password, which is why it lives here, in the file only the
// panel can read, and not on the container.
type AutoDeployRepository struct {
	store *Store
}

func NewAutoDeployRepository(store *Store) *AutoDeployRepository {
	return &AutoDeployRepository{store: store}
}

func (a *AutoDeployRepository) Find(ctx context.Context, container, service string) (*autodeploy.Setting, error) {
	s := autodeploy.Setting{Container: container, Service: service}
	err := a.store.db.QueryRowContext(ctx,
		`SELECT secret, failed FROM autodeploy WHERE container = ? AND service = ?`, container, service).
		Scan(&s.Secret, &s.Failed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (a *AutoDeployRepository) FindAll(ctx context.Context) ([]autodeploy.Setting, error) {
	rows, err := a.store.db.QueryContext(ctx,
		`SELECT container, service, secret, failed FROM autodeploy ORDER BY container, service`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all := []autodeploy.Setting{}
	for rows.Next() {
		var s autodeploy.Setting
		if err := rows.Scan(&s.Container, &s.Service, &s.Secret, &s.Failed); err != nil {
			return nil, err
		}
		all = append(all, s)
	}
	return all, rows.Err()
}

func (a *AutoDeployRepository) Save(ctx context.Context, s autodeploy.Setting) error {
	_, err := a.store.db.ExecContext(ctx,
		`INSERT INTO autodeploy (container, service, secret, failed) VALUES (?, ?, ?, ?)
		 ON CONFLICT (container, service) DO UPDATE SET secret = excluded.secret, failed = excluded.failed`,
		s.Container, s.Service, s.Secret, s.Failed)
	return err
}

func (a *AutoDeployRepository) Remove(ctx context.Context, container, service string) error {
	_, err := a.store.db.ExecContext(ctx,
		`DELETE FROM autodeploy WHERE container = ? AND service = ?`, container, service)
	return err
}
