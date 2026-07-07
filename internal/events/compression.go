package events

import (
	"context"
	"fmt"
	"sync"
)

// session tracks the in-progress activity classification EventCompressor is
// currently collapsing into one session.
type session struct {
	category   string
	label      string
	context    map[string]any
	startedAt  float64
	lastSeenAt float64
}

// EventCompressor subscribes to SemanticActivityInferred events and
// republishes them as ActivitySessionStarted/Ended sessions, mirroring
// Python's EventCompressor: an unbroken run of the same category is
// exactly one session, however long the poll loop keeps confirming it.
type EventCompressor struct {
	bus *EventBus

	mu      sync.Mutex
	current *session
}

// NewEventCompressor builds a compressor bound to bus. Call Attach to
// start consuming events.
func NewEventCompressor(bus *EventBus) *EventCompressor {
	return &EventCompressor{bus: bus}
}

// Attach subscribes the compressor to SemanticActivityInferred events.
func (c *EventCompressor) Attach() {
	c.bus.Subscribe(SemanticActivityInferred, c.onSemanticEvent)
}

func (c *EventCompressor) onSemanticEvent(ctx context.Context, event ActivityEvent) error {
	category := stringOr(event.Data, "category", "other")
	label := stringOr(event.Data, "label", category)
	now := event.Timestamp

	c.mu.Lock()
	if c.current != nil && c.current.category == category {
		c.current.lastSeenAt = now
		c.current.label = label
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if err := c.closeCurrent(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	c.current = &session{category: category, label: label, context: copyMap(event.Data), startedAt: now, lastSeenAt: now}
	c.mu.Unlock()

	data := mergeMaps(map[string]any{"category": category, "label": label}, event.Data)
	started := New(ActivitySessionStarted, "EventCompressor", data)
	started.Timestamp = now
	return c.bus.Publish(ctx, started)
}

func (c *EventCompressor) closeCurrent(ctx context.Context) error {
	c.mu.Lock()
	s := c.current
	c.current = nil
	c.mu.Unlock()

	if s == nil {
		return nil
	}
	duration := s.lastSeenAt - s.startedAt
	data := mergeMaps(map[string]any{
		"category": s.category, "label": s.label, "duration_seconds": duration,
	}, s.context)
	ended := New(ActivitySessionEnded, "EventCompressor", data)
	ended.Timestamp = s.lastSeenAt
	return c.bus.Publish(ctx, ended)
}

// Flush closes any open session. Call on shutdown so the last session
// isn't lost.
func (c *EventCompressor) Flush(ctx context.Context) error {
	return c.closeCurrent(ctx)
}

func stringOr(data map[string]any, key, def string) string {
	if v, ok := data[key]; ok {
		return fmt.Sprint(v)
	}
	return def
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// mergeMaps returns base overlaid with extra, extra's keys winning on
// conflict — mirroring Python's `{"category": category, "label": label,
// **event.data}` dict-literal ordering, where a later **spread can
// overwrite the explicit keys that came before it.
func mergeMaps(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
