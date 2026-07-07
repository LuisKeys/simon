package sensors

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"simon-go/internal/events"
	"simon-go/internal/privacy"
)

// fakeSensor emits one WindowFocusChanged event per poll, up to maxEvents,
// then reports no further changes — a synthetic stand-in for a real OS
// sensor, letting the pipeline be exercised end-to-end without macOS.
type fakeSensor struct {
	polls     atomic.Int32
	maxEvents int32
}

func (s *fakeSensor) Name() string                { return "FakeSensor" }
func (s *fakeSensor) Scope() privacy.Scope        { return privacy.WindowFocus }
func (s *fakeSensor) PollInterval() time.Duration { return time.Millisecond }
func (s *fakeSensor) Poll(context.Context) (*events.ActivityEvent, error) {
	n := s.polls.Add(1)
	if n > s.maxEvents {
		return nil, nil
	}
	e := events.New(events.WindowFocusChanged, "FakeSensor", map[string]any{"n": n})
	return &e, nil
}

func newTestManager(t *testing.T, bus *events.EventBus) *Manager {
	t.Helper()
	store := privacy.NewSQLiteStore(t.TempDir() + "/privacy.db")
	t.Cleanup(func() { store.Close() })
	perms := privacy.NewManager(store, bus)
	if err := perms.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewManager(bus, perms)
}

func TestStartAllDeniesUngrantedSensor(t *testing.T) {
	bus := events.NewEventBus(nil)
	mgr := newTestManager(t, bus)
	mgr.Register(&fakeSensor{maxEvents: 100})

	results := mgr.StartAll(context.Background())
	if results["FakeSensor"] {
		t.Error("expected FakeSensor to be denied (permission not granted)")
	}
	if mgr.Status()["FakeSensor"] {
		t.Error("expected FakeSensor to not be running")
	}
}

func TestStartAllRunsGrantedSensorAndPublishesEvents(t *testing.T) {
	bus := events.NewEventBus(nil)
	mgr := newTestManager(t, bus)
	if err := mgr.Permissions.Grant(context.Background(), privacy.WindowFocus); err != nil {
		t.Fatal(err)
	}

	var count atomic.Int32
	bus.Subscribe(events.WindowFocusChanged, func(ctx context.Context, e events.ActivityEvent) error {
		count.Add(1)
		return nil
	})

	sensor := &fakeSensor{maxEvents: 3}
	mgr.Register(sensor)

	results := mgr.StartAll(context.Background())
	if !results["FakeSensor"] {
		t.Fatal("expected FakeSensor to start")
	}
	if !mgr.Status()["FakeSensor"] {
		t.Error("expected FakeSensor to report as running")
	}

	waitFor(t, func() bool { return count.Load() >= 3 })
	mgr.StopAll()

	if mgr.Status()["FakeSensor"] {
		t.Error("expected FakeSensor to report as stopped after StopAll")
	}
}

func TestRunnerStopIsIdempotent(t *testing.T) {
	r := NewRunner(&fakeSensor{maxEvents: 0})
	r.Stop() // never started: must not panic or block
	r.Stop()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}
