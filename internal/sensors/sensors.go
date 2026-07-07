// Package sensors defines the Sensor interface and SensorManager, mirroring
// Python's simon/sensors/base.py. Platform-specific sensors (macOS
// clipboard/active-window, implemented in Python via PyObjC) are out of
// scope for this port per the migration plan: PyObjC has no Go equivalent,
// and embedding CGO/Objective-C here would break this SDK's
// no-CGO/cross-compilation story. The plan's design is a separate Swift
// satellite process talking newline-JSON over stdout, consumed through this
// same Sensor interface — deferred, not blocking the rest of Phase 4.
//
// Sensors are deliberately NOT modeled as tool.Tool: a Tool is a
// synchronous, single-invocation function the LLM decides to call; a
// Sensor is a background loop that decides for itself when there is
// something worth reporting and pushes it onto the EventBus. Routing
// sensors through the ReAct loop would mean one model round-trip per poll
// tick, at LLM latency and cost — the opposite of what a sensor is for.
package sensors

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"simon-go/internal/events"
	"simon-go/internal/privacy"
	"simon-go/pkg/simonerr"
)

// Sensor is a permanently-running observer. Poll is called on a fixed
// interval; it should do the minimum work needed to detect a change and
// return (nil, nil) when nothing changed, so unchanged state never
// produces event-log noise.
type Sensor interface {
	Name() string
	Scope() privacy.Scope
	PollInterval() time.Duration
	// Poll checks current state once, returning an event only when
	// something changed.
	Poll(ctx context.Context) (*events.ActivityEvent, error)
}

// Runner drives one Sensor's polling loop, checking permissions before
// starting and stopping cleanly via context cancellation — the Go
// analogue of Python's asyncio.Task-based Sensor.start()/stop().
type Runner struct {
	sensor Sensor

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewRunner wraps sensor for start/stop lifecycle management.
func NewRunner(sensor Sensor) *Runner {
	return &Runner{sensor: sensor}
}

// IsRunning reports whether the polling loop is currently active.
func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancel != nil
}

// Start begins the polling loop after checking permissions for the
// sensor's scope. Returns a PermissionDeniedError (and records an audit
// event) if the required scope has not been granted.
func (r *Runner) Start(ctx context.Context, bus *events.EventBus, permissions *privacy.Manager) error {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	if !permissions.IsGranted(r.sensor.Scope()) {
		if err := permissions.Deny(ctx, r.sensor.Scope(), r.sensor.Name()); err != nil {
			return err
		}
		return simonerr.NewPermissionDeniedError(
			fmt.Sprintf("%s requires the %q permission, which has not been granted.", r.sensor.Name(), r.sensor.Scope()))
	}

	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	r.mu.Lock()
	r.cancel, r.done = cancel, done
	r.mu.Unlock()

	go r.run(loopCtx, done, bus)
	return nil
}

func (r *Runner) run(ctx context.Context, done chan struct{}, bus *events.EventBus) {
	defer close(done)
	ticker := time.NewTicker(r.sensor.PollInterval())
	defer ticker.Stop()

	for {
		event, err := r.sensor.Poll(ctx)
		if err != nil {
			slog.Error("sensor poll failed", "sensor", r.sensor.Name(), "err", err)
		} else if event != nil {
			if err := bus.Publish(ctx, *event); err != nil {
				slog.Error("sensor event publish failed", "sensor", r.sensor.Name(), "err", err)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Stop cancels the polling loop and waits for it to exit. A no-op if not running.
func (r *Runner) Stop() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel, r.done = nil, nil
	r.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	<-done
}

// Manager registers sensors and starts/stops them as a group, honoring permissions.
type Manager struct {
	Bus         *events.EventBus
	Permissions *privacy.Manager

	mu      sync.Mutex
	runners map[string]*Runner
}

// NewManager builds a Manager bound to bus and permissions.
func NewManager(bus *events.EventBus, permissions *privacy.Manager) *Manager {
	return &Manager{Bus: bus, Permissions: permissions, runners: map[string]*Runner{}}
}

// Register adds sensor to the group (not yet started).
func (m *Manager) Register(sensor Sensor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runners[sensor.Name()] = NewRunner(sensor)
}

// StartAll starts every registered sensor. Returns {name: started} — a
// sensor denied permission is logged and reported as not started, rather
// than the whole call failing.
func (m *Manager) StartAll(ctx context.Context) map[string]bool {
	m.mu.Lock()
	runners := make(map[string]*Runner, len(m.runners))
	for name, r := range m.runners {
		runners[name] = r
	}
	m.mu.Unlock()

	results := make(map[string]bool, len(runners))
	for name, r := range runners {
		err := r.Start(ctx, m.Bus, m.Permissions)
		if err != nil {
			slog.Warn("sensor not started", "sensor", name, "err", err)
		}
		results[name] = err == nil
	}
	return results
}

// StopAll stops every registered sensor, concurrently.
func (m *Manager) StopAll() {
	m.mu.Lock()
	runners := make([]*Runner, 0, len(m.runners))
	for _, r := range m.runners {
		runners = append(runners, r)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, r := range runners {
		wg.Add(1)
		go func(r *Runner) { defer wg.Done(); r.Stop() }(r)
	}
	wg.Wait()
}

// Status reports which sensors are currently running.
func (m *Manager) Status() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool, len(m.runners))
	for name, r := range m.runners {
		out[name] = r.IsRunning()
	}
	return out
}
