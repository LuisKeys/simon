package privacy

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the persistence interface for permission grants.
type Store interface {
	Set(ctx context.Context, scope string, granted bool) error
	LoadAll(ctx context.Context) (map[string]bool, error)
	Close() error
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS permissions (
	scope       TEXT PRIMARY KEY,
	granted     INTEGER NOT NULL,
	updated_at  REAL NOT NULL
);
`

// SQLiteStore is a SQLite-backed Store, lazily opening its database file on
// first use. It shares the same on-disk database file as
// events.SQLiteStore by default, since both are part of the same local
// activity subsystem.
type SQLiteStore struct {
	path string

	mu sync.Mutex
	db *sql.DB
}

// NewSQLiteStore builds a store backed by the SQLite file at path.
func NewSQLiteStore(path string) *SQLiteStore {
	return &SQLiteStore{path: path}
}

func (s *SQLiteStore) ensureOpen() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}

	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, err
	}
	s.db = db
	return db, nil
}

func (s *SQLiteStore) Set(ctx context.Context, scope string, granted bool) error {
	db, err := s.ensureOpen()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO permissions (scope, granted, updated_at) VALUES (?, ?, ?) "+
			"ON CONFLICT(scope) DO UPDATE SET granted = excluded.granted, updated_at = excluded.updated_at",
		scope, boolToInt(granted), float64(time.Now().UnixNano())/1e9)
	return err
}

func (s *SQLiteStore) LoadAll(ctx context.Context) (map[string]bool, error) {
	db, err := s.ensureOpen()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, "SELECT scope, granted FROM permissions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var scope string
		var granted int
		if err := rows.Scan(&scope, &granted); err != nil {
			return nil, err
		}
		out[scope] = granted != 0
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
