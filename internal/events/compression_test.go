package events

import (
	"context"
	"sync"
	"testing"
)

func TestCompressorCollapsesRepeatedCategoryIntoOneSession(t *testing.T) {
	bus := NewEventBus(nil)
	var mu sync.Mutex
	var published []ActivityEvent
	// The wildcard subscription below matches BOTH the raw
	// SemanticActivityInferred event published from the test and the
	// ActivitySessionStarted/Ended events the compressor publishes as a
	// synchronous side effect of handling it — EventBus.Publish dispatches
	// all of a single event's matching handlers (including this one)
	// concurrently, so this handler can genuinely run on two goroutines at
	// once and needs its own lock around the shared slice.
	bus.Subscribe(Wildcard, func(ctx context.Context, e ActivityEvent) error {
		mu.Lock()
		defer mu.Unlock()
		published = append(published, e)
		return nil
	})

	c := NewEventCompressor(bus)
	c.Attach()

	publish := func(category string, ts float64) {
		_ = bus.Publish(context.Background(), ActivityEvent{
			Type: SemanticActivityInferred, Data: map[string]any{"category": category, "label": category}, Timestamp: ts,
		})
	}

	publish("programming", 100)
	publish("programming", 101) // same category: no new session
	publish("terminal", 102)    // category change: closes "programming", opens "terminal"

	if err := c.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	var started, ended []ActivityEvent
	for _, e := range published {
		switch e.Type {
		case ActivitySessionStarted:
			started = append(started, e)
		case ActivitySessionEnded:
			ended = append(ended, e)
		}
	}

	if len(started) != 2 || started[0].Data["category"] != "programming" || started[1].Data["category"] != "terminal" {
		t.Fatalf("started = %+v", started)
	}
	if len(ended) != 2 {
		t.Fatalf("expected 2 ended sessions (programming on category change, terminal on flush), got %+v", ended)
	}
	if got := ended[0].Data["duration_seconds"]; got != float64(1) {
		t.Errorf("expected first session duration 1s (100->101), got %v", got)
	}
}

func TestCompressorDefaultsMissingCategoryToOther(t *testing.T) {
	bus := NewEventBus(nil)
	var started []ActivityEvent
	bus.Subscribe(ActivitySessionStarted, func(ctx context.Context, e ActivityEvent) error {
		started = append(started, e)
		return nil
	})

	c := NewEventCompressor(bus)
	c.Attach()

	_ = bus.Publish(context.Background(), ActivityEvent{Type: SemanticActivityInferred, Data: map[string]any{}, Timestamp: 1})

	if len(started) != 1 || started[0].Data["category"] != "other" {
		t.Errorf("started = %+v", started)
	}
}
