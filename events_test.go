package simon_test

import (
	"context"
	"sync"
	"testing"

	"github.com/LuisKeys/simon"
)

func TestSessionRunEventOrdering(t *testing.T) {
	var mu sync.Mutex
	var seen []simon.EventType

	rt := newTestRuntime(t, simon.WithEventHandler(func(_ context.Context, ev simon.Event) {
		mu.Lock()
		seen = append(seen, ev.Type)
		mu.Unlock()
	}))
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := sess.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 2 {
		t.Fatalf("expected at least run.started and run.completed, got %v", seen)
	}
	if seen[0] != simon.EventRunStarted {
		t.Errorf("first event = %v, want run.started", seen[0])
	}
	if seen[len(seen)-1] != simon.EventRunCompleted {
		t.Errorf("last event = %v, want run.completed", seen[len(seen)-1])
	}
}

func TestSessionRunEmitsModelSelectedBeforeCompleted(t *testing.T) {
	var mu sync.Mutex
	var seen []simon.EventType

	// No WithModel override here: use the default router path so
	// model.selected actually fires (a fixed WithModel override skips
	// router resolution and never emits model.selected).
	rt, err := simon.New(simon.WithEventHandler(func(_ context.Context, ev simon.Event) {
		mu.Lock()
		seen = append(seen, ev.Type)
		mu.Unlock()
	}))
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := sess.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var modelIdx, completedIdx = -1, -1
	for i, ev := range seen {
		if ev == simon.EventModelSelected {
			modelIdx = i
		}
		if ev == simon.EventRunCompleted {
			completedIdx = i
		}
	}
	if modelIdx == -1 || completedIdx == -1 || modelIdx > completedIdx {
		t.Errorf("events = %v, want model.selected before run.completed", seen)
	}
}
