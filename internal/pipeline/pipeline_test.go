package pipeline

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"simon-go/internal/activity"
	"simon-go/internal/agent/response"
	"simon-go/internal/events"
	"simon-go/internal/habits"
	"simon-go/internal/model"
	"simon-go/internal/privacy"
	"simon-go/internal/semantic"
	"simon-go/internal/sensors"
)

// fakeWindowSensor cycles through a fixed sequence of app names, one per
// poll, standing in for a real OS window-focus sensor (out of scope for
// this port — see internal/sensors package doc) so the full pipeline can
// be exercised end-to-end in Go, without macOS.
type fakeWindowSensor struct {
	apps  []string
	polls atomic.Int32
}

func (s *fakeWindowSensor) Name() string                { return "FakeWindowSensor" }
func (s *fakeWindowSensor) Scope() privacy.Scope        { return privacy.WindowFocus }
func (s *fakeWindowSensor) PollInterval() time.Duration { return 2 * time.Millisecond }
func (s *fakeWindowSensor) Poll(context.Context) (*events.ActivityEvent, error) {
	n := int(s.polls.Add(1)) - 1
	if n >= len(s.apps) {
		return nil, nil
	}
	e := events.New(events.WindowFocusChanged, "FakeWindowSensor", map[string]any{
		"app_name": s.apps[n], "window_title": s.apps[n] + " - untitled",
	})
	return &e, nil
}

// classifyingModel maps app names to activity categories, standing in for
// a real local LLM classification call in internal/semantic.
type classifyingModel struct{}

func (classifyingModel) Complete(_ context.Context, messages []model.Message, _ []model.ToolSpec) (response.AgentResponse, error) {
	var userText string
	for _, m := range messages {
		if m.Role == model.RoleUser {
			userText = m.Content
		}
	}
	switch {
	case strings.Contains(userText, "Code"):
		return response.AgentResponse{Text: `{"category":"programming","label":"Writing Go"}`}, nil
	case strings.Contains(userText, "Slack"):
		return response.AgentResponse{Text: `{"category":"chat_messaging","label":"Chatting"}`}, nil
	default:
		return response.AgentResponse{Text: `{"category":"other","label":"Unknown"}`}, nil
	}
}

// TestFullActivityPipelineEndToEnd wires sensor -> bus -> store -> semantic
// -> activity -> habit together (the Phase 4 exit criterion from the
// migration plan) and drives it with a synthetic sensor, since macOS
// sensors are out of scope for this port.
func TestFullActivityPipelineEndToEnd(t *testing.T) {
	dbPath := t.TempDir() + "/activity.db"
	eventStore := events.NewSQLiteStore(dbPath)
	defer eventStore.Close()
	bus := events.NewEventBus(eventStore)
	ctx := context.Background()

	permStore := privacy.NewSQLiteStore(t.TempDir() + "/privacy.db")
	defer permStore.Close()
	perms := privacy.NewManager(permStore, bus)
	if err := perms.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err := perms.Grant(ctx, privacy.WindowFocus); err != nil {
		t.Fatal(err)
	}

	extractor, err := semantic.New(bus, semantic.WithModel(classifyingModel{}))
	if err != nil {
		t.Fatal(err)
	}
	extractor.Attach()

	compressor := events.NewEventCompressor(bus)
	compressor.Attach()

	graphStore := activity.NewGraphStore(t.TempDir() + "/graph.db")
	defer graphStore.Close()
	graphBuilder := activity.NewGraphBuilder(bus, graphStore)
	graphBuilder.Attach()

	contextEngine := activity.NewContextEngine(bus, 50)
	contextEngine.Attach()

	sensor := &fakeWindowSensor{apps: []string{"Code", "Code", "Slack", "Code"}}
	sensorMgr := sensors.NewManager(bus, perms)
	sensorMgr.Register(sensor)

	results := sensorMgr.StartAll(ctx)
	if !results["FakeWindowSensor"] {
		t.Fatal("expected the sensor to start (permission was granted)")
	}

	// Each sensor.Poll -> bus.Publish call fully processes the entire
	// semantic -> compression -> graph cascade synchronously (EventBus.Publish
	// waits for all handlers before returning), so once the sensor has polled
	// past the end of its fixed apps list, every event it produced has
	// already been handled. Only then is it safe to stop it: canceling the
	// sensor's context while a poll's cascade is still in flight surfaces as
	// spurious "context canceled" errors from in-progress SQL writes.
	waitFor(t, func() bool { return sensor.polls.Load() > int32(len(sensor.apps)) })
	sensorMgr.StopAll()
	if err := compressor.Flush(ctx); err != nil { // close the still-open trailing "Code" session
		t.Fatal(err)
	}

	activityStore := activity.NewStore(eventStore)
	sessions, err := activityStore.GetSessions(ctx, activity.GetSessionsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 completed sessions (programming, chat_messaging, programming again after flush), got %+v", sessions)
	}

	byCategory := map[string]bool{}
	for _, s := range sessions {
		byCategory[s.Category] = true
	}
	if !byCategory["programming"] || !byCategory["chat_messaging"] {
		t.Errorf("expected programming and chat_messaging sessions, got %+v", sessions)
	}

	transitions, err := graphStore.GetTransitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) == 0 {
		t.Error("expected at least one recorded category transition")
	}

	snap := contextEngine.Snapshot()
	if len(snap.Recent) == 0 {
		t.Error("expected ContextEngine to have recorded completed sessions")
	}

	// Feed enough synthetic multi-day history through the same
	// ActivityStore for HabitDiscoveryEngine to find a habit, proving it
	// plugs into the same pipeline the sensor/semantic/activity stages fed.
	seedDailyProgrammingSessions(t, bus, ctx)
	patternStore := habits.NewPatternStore(t.TempDir() + "/patterns.db")
	defer patternStore.Close()
	engine := habits.NewDiscoveryEngine(bus, activityStore, patternStore, habits.DefaultOptions())

	found, err := engine.RunOnce(ctx, timeAt(2024, 1, 15, 12, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].Categories[0] != "deep_focus" {
		t.Errorf("expected to find the seeded deep_focus habit, got %+v", found)
	}
}

func seedDailyProgrammingSessions(t *testing.T, bus *events.EventBus, ctx context.Context) {
	t.Helper()
	// Uses a category distinct from the live sensor-driven sessions above
	// (which are timestamped with the real wall clock, not 2024 dates): if
	// they shared a category, mining would merge both into one n-gram whose
	// start-time spread (today's wall-clock minute vs. 9:00am) exceeds
	// max_start_spread_minutes, and the habit would be rejected as noise
	// rather than found.
	for _, offset := range []int{1, 2, 3, 4, 5} {
		ts := timeAt(2024, 1, 15-offset, 9, 30)
		event := events.New(events.ActivitySessionEnded, "test", map[string]any{
			"category": "deep_focus", "label": "Deep focus block", "duration_seconds": 1800.0,
		})
		event.Timestamp = ts
		if err := bus.Publish(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
}

func timeAt(year, month, day, hour, minute int) float64 {
	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
	return float64(t.Unix())
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}
