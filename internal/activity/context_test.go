package activity

import (
	"context"
	"testing"

	"simon-go/internal/events"
)

func TestContextEngineTracksCurrentAndRecent(t *testing.T) {
	bus := events.NewEventBus(nil)
	ce := NewContextEngine(bus, 50)
	ce.Attach()
	ctx := context.Background()

	_ = bus.Publish(ctx, events.ActivityEvent{
		Type: events.ActivitySessionStarted, Timestamp: 100,
		Data: map[string]any{"category": "programming", "label": "Writing Go"},
	})

	snap := ce.Snapshot()
	if snap.Current == nil || snap.Current.Category != "programming" || snap.Current.Label != "Writing Go" {
		t.Fatalf("expected an in-progress session, got %+v", snap.Current)
	}
	if len(snap.Recent) != 0 {
		t.Errorf("expected no recent sessions yet, got %+v", snap.Recent)
	}

	_ = bus.Publish(ctx, events.ActivityEvent{
		Type: events.ActivitySessionEnded, Timestamp: 130,
		Data: map[string]any{"category": "programming", "label": "Writing Go", "duration_seconds": 30.0},
	})

	snap = ce.Snapshot()
	if snap.Current != nil {
		t.Errorf("expected no in-progress session after end, got %+v", snap.Current)
	}
	if len(snap.Recent) != 1 || snap.Recent[0].DurationSeconds != 30 {
		t.Fatalf("expected 1 recent session, got %+v", snap.Recent)
	}
}

func TestContextEngineHistoryIsBounded(t *testing.T) {
	bus := events.NewEventBus(nil)
	ce := NewContextEngine(bus, 2)
	ce.Attach()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_ = bus.Publish(ctx, events.ActivityEvent{
			Type: events.ActivitySessionEnded, Timestamp: float64(100 + i),
			Data: map[string]any{"category": "cat", "duration_seconds": float64(i)},
		})
	}

	snap := ce.Snapshot()
	if len(snap.Recent) != 2 {
		t.Fatalf("expected history bounded to 2, got %d", len(snap.Recent))
	}
	// Oldest of the 3 (duration 0) should have been evicted.
	if snap.Recent[0].DurationSeconds != 1 || snap.Recent[1].DurationSeconds != 2 {
		t.Errorf("expected [1, 2] retained oldest-first, got %+v", snap.Recent)
	}
}
