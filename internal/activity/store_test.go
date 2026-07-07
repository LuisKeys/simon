package activity

import (
	"context"
	"testing"

	"simon-go/internal/events"
)

func TestGetSessionsFiltersAndConvertsEndedEvents(t *testing.T) {
	store := events.NewSQLiteStore(t.TempDir() + "/activity.db")
	defer store.Close()
	bus := events.NewEventBus(store)
	ctx := context.Background()

	publishEnded := func(category string, ts, duration float64) {
		event := events.New(events.ActivitySessionEnded, "test",
			map[string]any{"category": category, "label": category, "duration_seconds": duration})
		event.Timestamp = ts
		if err := bus.Publish(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	publishEnded("programming", 100, 30)
	publishEnded("terminal", 200, 10)
	publishEnded("programming", 300, 20)

	s := NewStore(store)

	all, err := s.GetSessions(ctx, GetSessionsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(all))
	}
	// Most recent first.
	if all[0].Category != "programming" || all[0].StartedAt != 280 {
		t.Errorf("all[0] = %+v", all[0])
	}

	filtered, err := s.GetSessions(ctx, GetSessionsOptions{Category: "programming"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 programming sessions, got %d", len(filtered))
	}
}

func TestSummarizeByCategory(t *testing.T) {
	store := events.NewSQLiteStore(t.TempDir() + "/activity.db")
	defer store.Close()
	bus := events.NewEventBus(store)
	ctx := context.Background()

	publish := func(category string, ts, duration float64) {
		event := events.New(events.ActivitySessionEnded, "test", map[string]any{"category": category, "duration_seconds": duration})
		event.Timestamp = ts
		if err := bus.Publish(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	publish("programming", 100, 30.0)
	publish("programming", 200, 20.0)
	publish("terminal", 300, 15.0)

	s := NewStore(store)
	totals, err := s.SummarizeByCategory(ctx, GetSessionsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if totals["programming"] != 50 || totals["terminal"] != 15 {
		t.Errorf("totals = %+v", totals)
	}
}
