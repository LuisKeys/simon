package activity

import (
	"context"
	"sync"

	"simon-go/internal/events"
)

// CurrentActivity is the in-progress session ContextEngine is tracking.
type CurrentActivity struct {
	Category  string
	Label     string
	StartedAt float64
	Context   map[string]any
}

// Snapshot is what ContextEngine.Snapshot returns: the in-progress
// activity (nil if none) plus a bounded window of recently completed ones,
// oldest first.
type Snapshot struct {
	Current *CurrentActivity
	Recent  []Session
}

// ContextEngine is an in-memory read model of "what is the user doing
// right now", derived purely from the session stream on the EventBus — no
// separate storage. Downstream consumers (Habit Discovery) read Snapshot
// instead of re-deriving this from the Activity Store on every call.
type ContextEngine struct {
	bus *events.EventBus

	mu      sync.Mutex
	current *CurrentActivity
	recent  *boundedQueue
}

// NewContextEngine builds a ContextEngine bound to bus, keeping the last
// historySize completed sessions in memory.
func NewContextEngine(bus *events.EventBus, historySize int) *ContextEngine {
	return &ContextEngine{bus: bus, recent: newBoundedQueue(historySize)}
}

// Attach subscribes the engine to ActivitySessionStarted/Ended events.
func (c *ContextEngine) Attach() {
	c.bus.Subscribe(events.ActivitySessionStarted, c.onStarted)
	c.bus.Subscribe(events.ActivitySessionEnded, c.onEnded)
}

func (c *ContextEngine) onStarted(_ context.Context, event events.ActivityEvent) error {
	category := stringOr(event.Data, "category", "other")
	c.mu.Lock()
	c.current = &CurrentActivity{
		Category:  category,
		Label:     stringOr(event.Data, "label", category),
		StartedAt: event.Timestamp,
		Context:   omitKeys(event.Data, "category", "label"),
	}
	c.mu.Unlock()
	return nil
}

func (c *ContextEngine) onEnded(_ context.Context, event events.ActivityEvent) error {
	category := stringOr(event.Data, "category", "other")
	duration := floatOr(event.Data, "duration_seconds", 0)
	endedAt := event.Timestamp

	session := Session{
		Category:        category,
		Label:           stringOr(event.Data, "label", category),
		StartedAt:       endedAt - duration,
		EndedAt:         endedAt,
		DurationSeconds: duration,
		Context:         omitKeys(event.Data, "category", "label", "duration_seconds"),
	}

	c.mu.Lock()
	c.recent.push(session)
	c.current = nil
	c.mu.Unlock()
	return nil
}

// Snapshot returns the current activity (if any) and the bounded recent
// history, oldest first.
func (c *ContextEngine) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Snapshot{Current: c.current, Recent: c.recent.snapshot()}
}

// boundedQueue is a fixed-capacity FIFO of Sessions, the Go analogue of
// Python's collections.deque(maxlen=...): pushing past capacity silently
// drops the oldest item.
type boundedQueue struct {
	items    []Session
	capacity int
}

func newBoundedQueue(capacity int) *boundedQueue {
	if capacity <= 0 {
		capacity = 1
	}
	return &boundedQueue{capacity: capacity}
}

func (q *boundedQueue) push(s Session) {
	q.items = append(q.items, s)
	if len(q.items) > q.capacity {
		q.items = q.items[len(q.items)-q.capacity:]
	}
}

// snapshot returns a defensive copy of the queue's current contents,
// oldest first.
func (q *boundedQueue) snapshot() []Session {
	out := make([]Session, len(q.items))
	copy(out, q.items)
	return out
}
