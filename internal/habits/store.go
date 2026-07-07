// Package habits mines the Activity Store for recurring category n-grams,
// mirroring Python's simon/habits package (Habit, HabitDiscoveryEngine,
// PatternStore).
package habits

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS detected_patterns (
	signature           TEXT PRIMARY KEY,
	data                TEXT NOT NULL,
	first_detected_at   REAL NOT NULL,
	last_detected_at    REAL NOT NULL
);
`

// PatternRecord is one previously detected habit, as stored.
type PatternRecord struct {
	Signature       string
	Data            map[string]any
	FirstDetectedAt float64
	LastDetectedAt  float64
}

// PatternStore is a SQLite-backed store of previously detected habits,
// keyed by signature, used by HabitDiscoveryEngine to avoid re-publishing
// the same finding, unchanged, on every run.
type PatternStore struct {
	path string

	mu sync.Mutex
	db *sql.DB
}

// NewPatternStore builds a store backed by the SQLite file at path.
func NewPatternStore(path string) *PatternStore {
	return &PatternStore{path: path}
}

func (s *PatternStore) ensureOpen() (*sql.DB, error) {
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

// Get looks up a previously detected pattern by signature, returning
// (nil, nil) if not found.
func (s *PatternStore) Get(ctx context.Context, signature string) (*PatternRecord, error) {
	db, err := s.ensureOpen()
	if err != nil {
		return nil, err
	}
	row := db.QueryRowContext(ctx,
		"SELECT data, first_detected_at, last_detected_at FROM detected_patterns WHERE signature = ?", signature)

	var data string
	var rec PatternRecord
	rec.Signature = signature
	if err := row.Scan(&data, &rec.FirstDetectedAt, &rec.LastDetectedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(data), &rec.Data); err != nil {
		return nil, err
	}
	return &rec, nil
}

// Upsert records signature as detected at timestamp (updating first_detected_at
// only on first insert).
func (s *PatternStore) Upsert(ctx context.Context, signature string, data map[string]any, timestamp float64) error {
	db, err := s.ensureOpen()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO detected_patterns (signature, data, first_detected_at, last_detected_at) VALUES (?, ?, ?, ?) "+
			"ON CONFLICT(signature) DO UPDATE SET data = excluded.data, last_detected_at = excluded.last_detected_at",
		signature, string(encoded), timestamp, timestamp)
	return err
}

// All returns every stored pattern.
func (s *PatternStore) All(ctx context.Context) ([]PatternRecord, error) {
	db, err := s.ensureOpen()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, "SELECT signature, data, first_detected_at, last_detected_at FROM detected_patterns")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PatternRecord
	for rows.Next() {
		var rec PatternRecord
		var data string
		if err := rows.Scan(&rec.Signature, &data, &rec.FirstDetectedAt, &rec.LastDetectedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &rec.Data); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Close releases the underlying database connection.
func (s *PatternStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// nowSeconds returns the current time as Unix seconds, matching the
// float-seconds timestamp convention used throughout the activity pipeline.
func nowSeconds() float64 { return float64(time.Now().UnixNano()) / 1e9 }
