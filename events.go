package simon

import (
	"context"
	"time"
)

// EventType identifies a point in a run's lifecycle.
type EventType string

const (
	EventRunStarted     EventType = "run.started"
	EventModelSelected  EventType = "model.selected"
	EventResponseDelta  EventType = "response.delta"
	EventToolRequested  EventType = "tool.requested"
	EventToolStarted    EventType = "tool.started"
	EventToolCompleted  EventType = "tool.completed"
	EventToolFailed     EventType = "tool.failed"
	EventRetryAttempted EventType = "retry.attempted"
	EventRunCompleted   EventType = "run.completed"
	EventRunFailed      EventType = "run.failed"
	EventRunCancelled   EventType = "run.cancelled"
)

// isTerminal reports whether an event ends a run's lifecycle — terminal
// events are always delivered to a Stream, never dropped for buffer space.
func (t EventType) isTerminal() bool {
	switch t {
	case EventRunCompleted, EventRunFailed, EventRunCancelled:
		return true
	default:
		return false
	}
}

// Event is a single point-in-time occurrence during a Session run.
type Event struct {
	Type      EventType
	RuntimeID string
	SessionID string
	RunID     string
	Timestamp time.Time
	Data      any
}

// EventHandler observes every Event a Runtime's sessions emit. A handler
// that panics or is slow must never destabilize a run: Runtime always
// invokes handlers through a recover-guarded call.
type EventHandler func(context.Context, Event)

// safeInvoke calls handler, recovering any panic so a misbehaving consumer
// can't crash an in-flight run.
func safeInvoke(handler EventHandler, ctx context.Context, ev Event) {
	if handler == nil {
		return
	}
	defer func() { _ = recover() }()
	handler(ctx, ev)
}
