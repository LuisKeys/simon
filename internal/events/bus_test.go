package events

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestPublishDispatchesToMatchingAndWildcardHandlers(t *testing.T) {
	bus := NewEventBus(nil)
	var mu sync.Mutex
	var got []string

	bus.Subscribe(WindowFocusChanged, func(ctx context.Context, e ActivityEvent) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, "specific:"+e.Type)
		return nil
	})
	bus.Subscribe(Wildcard, func(ctx context.Context, e ActivityEvent) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, "wildcard:"+e.Type)
		return nil
	})

	if err := bus.Publish(context.Background(), New(WindowFocusChanged, "test", nil)); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected both handlers to fire, got %v", got)
	}
}

func TestPublishSwallowsHandlerErrors(t *testing.T) {
	bus := NewEventBus(nil)
	bus.Subscribe(WindowFocusChanged, func(ctx context.Context, e ActivityEvent) error {
		return errors.New("boom")
	})

	if err := bus.Publish(context.Background(), New(WindowFocusChanged, "test", nil)); err != nil {
		t.Fatalf("expected Publish to swallow handler errors, got %v", err)
	}
}

func TestPublishPersistsToStore(t *testing.T) {
	store := NewSQLiteStore(t.TempDir() + "/events.db")
	defer store.Close()
	bus := NewEventBus(store)

	event := New(WindowFocusChanged, "test", map[string]any{"app": "Terminal"})
	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetEvents(context.Background(), GetEventsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != event.ID || got[0].Data["app"] != "Terminal" {
		t.Errorf("got %+v", got)
	}
}

func TestNewFillsIDAndTimestamp(t *testing.T) {
	e := New(WindowFocusChanged, "test", nil)
	if e.ID == "" {
		t.Error("expected a generated ID")
	}
	if e.Timestamp == 0 {
		t.Error("expected a non-zero timestamp")
	}
	if e.Data == nil {
		t.Error("expected Data to default to an empty map, not nil")
	}
}
