// Package sqlite holds the only state OpenCroft owns.
//
// Everything about containers, routes and certificates is read from the system
// itself. What lives here is what cannot be derived from it: who may log in,
// which sessions are open, and what was done. If a repository over
// infrastructure ever needs this package, the feature is designed wrong.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    expires    TEXT NOT NULL,
    created    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_expires ON sessions (expires);
`

type Store struct {
	db *sql.DB
}

// Open creates the database if it is not there. Deleting this file costs you
// your users and sessions — never your infrastructure.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("creating the state directory: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, err
	}

	// One writer at a time: SQLite does not benefit from more, and this
	// removes a whole class of "database is locked" surprises.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		return nil, fmt.Errorf("applying the schema: %w", err)
	}

	// The file holds password hashes and live session tokens.
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("restricting access to the database: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
