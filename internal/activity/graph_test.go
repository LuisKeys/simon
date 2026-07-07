package activity

import (
	"context"
	"testing"

	"simon-go/internal/events"
)

func TestGraphBuilderRecordsTransitionsOnCategoryChange(t *testing.T) {
	bus := events.NewEventBus(nil)
	store := NewGraphStore(t.TempDir() + "/graph.db")
	defer store.Close()

	b := NewGraphBuilder(bus, store)
	b.Attach()
	ctx := context.Background()

	publish := func(category string, ts float64) {
		_ = bus.Publish(ctx, events.ActivityEvent{
			Type: events.ActivitySessionStarted, Timestamp: ts, Data: map[string]any{"category": category},
		})
	}
	publish("programming", 100)
	publish("terminal", 110)    // programming -> terminal
	publish("terminal", 120)    // same category: no transition
	publish("programming", 130) // terminal -> programming

	transitions, err := store.GetTransitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 2 {
		t.Fatalf("expected 2 distinct transitions, got %+v", transitions)
	}

	byPair := map[string]Transition{}
	for _, tr := range transitions {
		byPair[tr.FromCategory+"->"+tr.ToCategory] = tr
	}
	if byPair["programming->terminal"].Count != 1 {
		t.Errorf("programming->terminal = %+v", byPair["programming->terminal"])
	}
	if byPair["terminal->programming"].Count != 1 {
		t.Errorf("terminal->programming = %+v", byPair["terminal->programming"])
	}
}

func TestGraphStoreRecordTransitionIncrementsCount(t *testing.T) {
	store := NewGraphStore(t.TempDir() + "/graph.db")
	defer store.Close()
	ctx := context.Background()

	_ = store.RecordTransition(ctx, "a", "b", 1)
	_ = store.RecordTransition(ctx, "a", "b", 2)
	_ = store.RecordTransition(ctx, "a", "b", 3)

	top, err := store.GetTopTransitions(ctx, "a", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Count != 3 || top[0].LastSeen != 3 {
		t.Fatalf("top = %+v", top)
	}
}
