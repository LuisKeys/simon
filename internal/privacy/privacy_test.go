package privacy

import (
	"context"
	"testing"

	"simon-go/internal/events"
)

func newTestManager(t *testing.T, bus *events.EventBus) *Manager {
	t.Helper()
	store := NewSQLiteStore(t.TempDir() + "/privacy.db")
	t.Cleanup(func() { store.Close() })
	m := NewManager(store, bus)
	if err := m.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestIsGrantedDeniesByDefault(t *testing.T) {
	m := newTestManager(t, nil)
	if m.IsGranted(WindowFocus) {
		t.Error("expected deny-by-default")
	}
}

func TestGrantAndRevoke(t *testing.T) {
	m := newTestManager(t, nil)
	ctx := context.Background()

	if err := m.Grant(ctx, ClipboardMetadata); err != nil {
		t.Fatal(err)
	}
	if !m.IsGranted(ClipboardMetadata) {
		t.Error("expected ClipboardMetadata to be granted")
	}

	if err := m.Revoke(ctx, ClipboardMetadata); err != nil {
		t.Fatal(err)
	}
	if m.IsGranted(ClipboardMetadata) {
		t.Error("expected ClipboardMetadata to be revoked")
	}
}

func TestGrantsPersistAcrossManagerInstances(t *testing.T) {
	dir := t.TempDir() + "/privacy.db"
	store1 := NewSQLiteStore(dir)
	m1 := NewManager(store1, nil)
	_ = m1.Initialize(context.Background())
	_ = m1.Grant(context.Background(), ScreenText)
	store1.Close()

	store2 := NewSQLiteStore(dir)
	defer store2.Close()
	m2 := NewManager(store2, nil)
	if err := m2.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !m2.IsGranted(ScreenText) {
		t.Error("expected grant to persist across instances")
	}
}

func TestGrantRevokeDenyAreAudited(t *testing.T) {
	bus := events.NewEventBus(nil)
	var audited []events.ActivityEvent
	bus.Subscribe(events.Wildcard, func(ctx context.Context, e events.ActivityEvent) error {
		audited = append(audited, e)
		return nil
	})

	m := newTestManager(t, bus)
	ctx := context.Background()
	_ = m.Grant(ctx, WindowFocus)
	_ = m.Revoke(ctx, WindowFocus)
	_ = m.Deny(ctx, ClipboardContent, "ClipboardSensor")

	if len(audited) != 3 {
		t.Fatalf("expected 3 audit events, got %d: %+v", len(audited), audited)
	}
	if audited[0].Type != events.PermissionGranted || audited[1].Type != events.PermissionRevoked || audited[2].Type != events.PermissionDenied {
		t.Errorf("unexpected event types: %+v", audited)
	}
	if audited[2].Data["sensor"] != "ClipboardSensor" {
		t.Errorf("expected sensor name in deny audit, got %+v", audited[2].Data)
	}
}
