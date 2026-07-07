// Package activity implements read models over the session stream the
// events.EventCompressor produces, mirroring Python's simon/activity
// package (ActivityStore, ContextEngine, ActivityGraphStore/Builder).
package activity

import (
	"context"

	"simon-go/internal/events"
)

// Session is one completed activity, with everything an
// ActivitySessionEnded event already carries — no separate schema, just a
// read model over events.Store.
type Session struct {
	Category        string
	Label           string
	StartedAt       float64
	EndedAt         float64
	DurationSeconds float64
	Context         map[string]any
}

// Store provides high-level, read-only queries over completed activity
// sessions, mirroring Python's ActivityStore.
type Store struct {
	events events.Store
}

// NewStore builds a Store reading from eventStore.
func NewStore(eventStore events.Store) *Store {
	return &Store{events: eventStore}
}

// GetSessionsOptions filters GetSessions.
type GetSessionsOptions struct {
	Since    float64
	SinceSet bool
	Until    float64
	UntilSet bool
	Category string // "" means no category filter
	Limit    int    // 0 defaults to 100
}

// GetSessions returns completed sessions, most recent first.
func (s *Store) GetSessions(ctx context.Context, opts GetSessionsOptions) ([]Session, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}
	// Fetch generously when filtering client-side (category/until), since
	// events.Store.GetEvents only filters by type + since + limit natively.
	fetchLimit := limit
	if opts.Category != "" || opts.UntilSet {
		fetchLimit = max(limit*5, limit)
	}

	raw, err := s.events.GetEvents(ctx, events.GetEventsOptions{
		EventType: events.ActivitySessionEnded, Since: opts.Since, SinceSet: opts.SinceSet, Limit: fetchLimit,
	})
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, event := range raw {
		endedAt := event.Timestamp
		if opts.UntilSet && endedAt > opts.Until {
			continue
		}
		category := stringOr(event.Data, "category", "other")
		if opts.Category != "" && category != opts.Category {
			continue
		}
		duration := floatOr(event.Data, "duration_seconds", 0)
		sessions = append(sessions, Session{
			Category:        category,
			Label:           stringOr(event.Data, "label", category),
			StartedAt:       endedAt - duration,
			EndedAt:         endedAt,
			DurationSeconds: duration,
			Context:         omitKeys(event.Data, "category", "label", "duration_seconds"),
		})
		if len(sessions) >= limit {
			break
		}
	}
	return sessions, nil
}

// SummarizeByCategory returns total seconds spent per category over the
// given range.
func (s *Store) SummarizeByCategory(ctx context.Context, opts GetSessionsOptions) (map[string]float64, error) {
	opts.Limit = 10_000
	sessions, err := s.GetSessions(ctx, opts)
	if err != nil {
		return nil, err
	}
	totals := map[string]float64{}
	for _, session := range sessions {
		totals[session.Category] += session.DurationSeconds
	}
	return totals, nil
}

func stringOr(data map[string]any, key, def string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func floatOr(data map[string]any, key string, def float64) float64 {
	v, ok := data[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	default:
		return def
	}
}

func omitKeys(data map[string]any, keys ...string) map[string]any {
	skip := make(map[string]bool, len(keys))
	for _, k := range keys {
		skip[k] = true
	}
	out := map[string]any{}
	for k, v := range data {
		if !skip[k] {
			out[k] = v
		}
	}
	return out
}
