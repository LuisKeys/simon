// Package events implements Simon's activity-pipeline pub/sub, mirroring
// Python's simon/events/bus.py (EventBus, ActivityEvent) and
// simon/events/compression.py (EventCompressor).
package events

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event type constants, matching the dot-namespaced strings in Python's
// simon/events/bus.py.
const (
	WindowFocusChanged = "window.focus_changed"
	ClipboardChanged   = "clipboard.changed"

	PermissionGranted = "privacy.permission_granted"
	PermissionRevoked = "privacy.permission_revoked"
	PermissionDenied  = "privacy.permission_denied"

	SemanticActivityInferred = "semantic.activity_inferred"

	ActivitySessionStarted = "activity.session_started"
	ActivitySessionEnded   = "activity.session_ended"

	PatternDetected     = "pattern.detected"
	AutomationSuggested = "automation.suggested"

	// Wildcard subscribes a handler to every event type.
	Wildcard = "*"
)

// ActivityEvent is a single observation or inference flowing through the
// EventBus. Data carries event-specific key/value pairs — never raw
// images, and never full clipboard/document contents.
type ActivityEvent struct {
	Type      string
	Data      map[string]any
	Source    string
	Timestamp float64 // Unix seconds, matching Python's time.time()
	ID        string
}

// New builds an ActivityEvent with a generated ID and the current time as
// Timestamp, mirroring the `field(default_factory=...)` defaults on
// Python's @dataclass ActivityEvent.
func New(eventType, source string, data map[string]any) ActivityEvent {
	if data == nil {
		data = map[string]any{}
	}
	return ActivityEvent{
		Type:      eventType,
		Data:      data,
		Source:    source,
		Timestamp: float64(time.Now().UnixNano()) / 1e9,
		ID:        uuid.NewString(),
	}
}

// Handler processes one ActivityEvent. Handlers run concurrently per
// publish and their errors are logged, never propagated to the publisher —
// mirroring Python's `_safe_call` + `return_exceptions=True` gather.
type Handler func(ctx context.Context, event ActivityEvent) error

// EventBus is an async (here: concurrent) pub/sub bus for ActivityEvents.
// If a Store is attached, every published event is persisted before
// handlers run.
type EventBus struct {
	mu       sync.Mutex
	handlers map[string][]Handler
	store    Store
}

// NewEventBus builds an EventBus, optionally persisting every published
// event through store (pass nil for no persistence).
func NewEventBus(store Store) *EventBus {
	return &EventBus{handlers: map[string][]Handler{}, store: store}
}

// Subscribe registers handler to be called whenever eventType is
// published. Use Wildcard ("*") to receive every event regardless of type.
func (b *EventBus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish persists (if a Store is configured) and dispatches event to
// every matching handler (its own type plus any wildcard subscribers).
func (b *EventBus) Publish(ctx context.Context, event ActivityEvent) error {
	if b.store != nil {
		if err := b.store.Append(ctx, event); err != nil {
			return err
		}
	}

	b.mu.Lock()
	targets := append(append([]Handler{}, b.handlers[event.Type]...), b.handlers[Wildcard]...)
	b.mu.Unlock()

	if len(targets) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	for _, h := range targets {
		wg.Add(1)
		go func(h Handler) {
			defer wg.Done()
			if err := h(ctx, event); err != nil {
				slog.Warn("EventBus handler raised", "type", event.Type, "err", err)
			}
		}(h)
	}
	wg.Wait()
	return nil
}
