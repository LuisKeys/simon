package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Store is the persistence interface for ActivityEvents, the Go analogue
// of Python's EventStore ABC. SQLite is the default (and, for now, only)
// backend — everything in this product runs local-first.
type Store interface {
	Append(ctx context.Context, event ActivityEvent) error
	GetEvents(ctx context.Context, opts GetEventsOptions) ([]ActivityEvent, error)
	Close() error
}

// GetEventsOptions filters GetEvents, mirroring get_events's optional
// (event_type, since, limit) parameters.
type GetEventsOptions struct {
	EventType string  // "" means no type filter
	Since     float64 // 0 means no time filter; use SinceSet to filter from Unix epoch itself
	SinceSet  bool
	Limit     int // 0 defaults to 100, matching Python's default
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS activity_events (
	id          TEXT PRIMARY KEY,
	type        TEXT NOT NULL,
	source      TEXT NOT NULL,
	data        TEXT NOT NULL,
	timestamp   REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_activity_events_type ON activity_events (type);
CREATE INDEX IF NOT EXISTS idx_activity_events_timestamp ON activity_events (timestamp);
`

// SQLiteStore is a SQLite-backed Store using modernc.org/sqlite (pure Go,
// no CGO), lazily opening and migrating its database file on first use —
// mirroring Python's SQLiteEventStore/aiosqlite lazy-init pattern.
//
// Go's *sql.DB is already safe for concurrent use (it pools connections
// internally), so there is no separate locking here beyond what
// database/sql provides.
type SQLiteStore struct {
	path string

	mu sync.Mutex
	db *sql.DB
}

// NewSQLiteStore builds a store backed by the SQLite file at path
// (created, including parent dirs, on first use).
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

func (s *SQLiteStore) Append(ctx context.Context, event ActivityEvent) error {
	db, err := s.ensureOpen()
	if err != nil {
		return err
	}
	data, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO activity_events (id, type, source, data, timestamp) VALUES (?, ?, ?, ?, ?)",
		event.ID, event.Type, event.Source, string(data), event.Timestamp)
	return err
}

func (s *SQLiteStore) GetEvents(ctx context.Context, opts GetEventsOptions) ([]ActivityEvent, error) {
	db, err := s.ensureOpen()
	if err != nil {
		return nil, err
	}

	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}

	var clauses []string
	var args []any
	if opts.EventType != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, opts.EventType)
	}
	if opts.SinceSet {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, opts.Since)
	}

	query := "SELECT id, type, source, data, timestamp FROM activity_events "
	if len(clauses) > 0 {
		query += "WHERE " + joinAnd(clauses) + " "
	}
	query += "ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActivityEvent
	for rows.Next() {
		var e ActivityEvent
		var data string
		if err := rows.Scan(&e.ID, &e.Type, &e.Source, &data, &e.Timestamp); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			return nil, err
		}
		out = append(out, e)
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

func joinAnd(clauses []string) string {
	out := clauses[0]
	for _, c := range clauses[1:] {
		out += " AND " + c
	}
	return out
}
