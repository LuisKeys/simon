package activity

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"

	"simon-go/internal/events"
)

// Transition is one edge of the activity graph: how often (and when most
// recently) the user moved from one category to another.
type Transition struct {
	FromCategory string
	ToCategory   string
	Count        int
	LastSeen     float64
}

const graphSchemaSQL = `
CREATE TABLE IF NOT EXISTS activity_transitions (
	from_category   TEXT NOT NULL,
	to_category     TEXT NOT NULL,
	count           INTEGER NOT NULL,
	last_seen       REAL NOT NULL,
	PRIMARY KEY (from_category, to_category)
);
`

// GraphStore is SQLite-backed persistence for activity transitions (edges
// of the graph), mirroring Python's ActivityGraphStore.
type GraphStore struct {
	path string

	mu sync.Mutex
	db *sql.DB
}

// NewGraphStore builds a store backed by the SQLite file at path.
func NewGraphStore(path string) *GraphStore {
	return &GraphStore{path: path}
}

func (s *GraphStore) ensureOpen() (*sql.DB, error) {
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
	if _, err := db.Exec(graphSchemaSQL); err != nil {
		db.Close()
		return nil, err
	}
	s.db = db
	return db, nil
}

// RecordTransition upserts one from->to transition, incrementing its count.
func (s *GraphStore) RecordTransition(ctx context.Context, from, to string, timestamp float64) error {
	db, err := s.ensureOpen()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO activity_transitions (from_category, to_category, count, last_seen) VALUES (?, ?, 1, ?) "+
			"ON CONFLICT(from_category, to_category) DO UPDATE SET "+
			"count = count + 1, last_seen = excluded.last_seen",
		from, to, timestamp)
	return err
}

// GetTransitions returns every recorded transition.
func (s *GraphStore) GetTransitions(ctx context.Context) ([]Transition, error) {
	db, err := s.ensureOpen()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, "SELECT from_category, to_category, count, last_seen FROM activity_transitions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransitions(rows)
}

// GetTopTransitions returns the limit most frequent transitions out of from.
func (s *GraphStore) GetTopTransitions(ctx context.Context, from string, limit int) ([]Transition, error) {
	db, err := s.ensureOpen()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		"SELECT from_category, to_category, count, last_seen FROM activity_transitions "+
			"WHERE from_category = ? ORDER BY count DESC LIMIT ?", from, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransitions(rows)
}

func scanTransitions(rows *sql.Rows) ([]Transition, error) {
	var out []Transition
	for rows.Next() {
		var t Transition
		if err := rows.Scan(&t.FromCategory, &t.ToCategory, &t.Count, &t.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Close releases the underlying database connection.
func (s *GraphStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// GraphBuilder subscribes to the EventBus and records category-to-category
// transitions live, from the same session stream the EventCompressor
// already emits.
type GraphBuilder struct {
	bus   *events.EventBus
	store *GraphStore

	mu           sync.Mutex
	lastCategory string
	hasLast      bool
}

// NewGraphBuilder builds a GraphBuilder writing transitions to store.
func NewGraphBuilder(bus *events.EventBus, store *GraphStore) *GraphBuilder {
	return &GraphBuilder{bus: bus, store: store}
}

// Attach subscribes the builder to ActivitySessionStarted events.
func (b *GraphBuilder) Attach() {
	b.bus.Subscribe(events.ActivitySessionStarted, b.onSessionStarted)
}

func (b *GraphBuilder) onSessionStarted(ctx context.Context, event events.ActivityEvent) error {
	category := stringOr(event.Data, "category", "other")

	b.mu.Lock()
	prev, hadPrev := b.lastCategory, b.hasLast
	b.lastCategory, b.hasLast = category, true
	b.mu.Unlock()

	if hadPrev && prev != category {
		return b.store.RecordTransition(ctx, prev, category, event.Timestamp)
	}
	return nil
}
