package habits

import (
	"context"
	"testing"
	"time"

	"simon-go/internal/activity"
	"simon-go/internal/events"
)

func timeAt(year, month, day, hour, minute int) float64 {
	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
	return float64(t.Unix())
}

func dayAt(dayTimestamp float64, hour, minute int) float64 {
	return dayTimestamp + float64(hour*3600+minute*60)
}

func timeMillis(n int) time.Duration { return time.Duration(n) * time.Millisecond }

// seedSessions publishes one ActivitySessionEnded event per session
// description, at 8:50am local time on each of the given day-offsets (0 =
// today), so HabitDiscoveryEngine has a consistent daily pattern to mine.
func seedSessions(t *testing.T, bus *events.EventBus, category string, dayOffsets []int, startHour, startMinute int, durationSeconds float64) {
	t.Helper()
	now := timeAt(2024, 1, 15, 0, 0)
	for _, offset := range dayOffsets {
		day := now - float64(offset)*secondsPerDay
		started := dayAt(day, startHour, startMinute)
		ended := started + durationSeconds
		event := events.New(events.ActivitySessionEnded, "test", map[string]any{
			"category": category, "label": category, "duration_seconds": durationSeconds,
		})
		event.Timestamp = ended
		if err := bus.Publish(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunOnceDiscoversRecurringHabit(t *testing.T) {
	store := events.NewSQLiteStore(t.TempDir() + "/activity.db")
	defer store.Close()
	bus := events.NewEventBus(store)

	// "programming" every day at 08:50 for 5 of the last 7 days.
	seedSessions(t, bus, "programming", []int{0, 1, 2, 3, 4}, 8, 50, 1800)

	activityStore := activity.NewStore(store)
	patternStore := NewPatternStore(t.TempDir() + "/patterns.db")
	defer patternStore.Close()

	engine := NewDiscoveryEngine(bus, activityStore, patternStore, DefaultOptions())
	now := timeAt(2024, 1, 15, 12, 0)

	habits, err := engine.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(habits) != 1 {
		t.Fatalf("expected 1 discovered habit, got %+v", habits)
	}
	h := habits[0]
	if len(h.Categories) != 1 || h.Categories[0] != "programming" {
		t.Errorf("Categories = %v", h.Categories)
	}
	if h.DaysObserved != 5 {
		t.Errorf("DaysObserved = %d, want 5", h.DaysObserved)
	}
}

func TestRunOnceSkipsUnchangedHabitOnSecondRun(t *testing.T) {
	store := events.NewSQLiteStore(t.TempDir() + "/activity.db")
	defer store.Close()
	bus := events.NewEventBus(store)
	seedSessions(t, bus, "terminal", []int{0, 1, 2, 3, 4}, 9, 0, 600)

	activityStore := activity.NewStore(store)
	patternStore := NewPatternStore(t.TempDir() + "/patterns.db")
	defer patternStore.Close()
	engine := NewDiscoveryEngine(bus, activityStore, patternStore, DefaultOptions())
	now := timeAt(2024, 1, 15, 12, 0)

	first, err := engine.RunOnce(context.Background(), now)
	if err != nil || len(first) != 1 {
		t.Fatalf("first run: habits=%+v err=%v", first, err)
	}

	second, err := engine.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Errorf("expected no re-detection of an unchanged habit, got %+v", second)
	}
}

func TestRunOnceRequiresMinimumDays(t *testing.T) {
	store := events.NewSQLiteStore(t.TempDir() + "/activity.db")
	defer store.Close()
	bus := events.NewEventBus(store)
	seedSessions(t, bus, "email", []int{0, 1}, 10, 0, 300) // only 2 days, min is 3

	activityStore := activity.NewStore(store)
	patternStore := NewPatternStore(t.TempDir() + "/patterns.db")
	defer patternStore.Close()
	engine := NewDiscoveryEngine(bus, activityStore, patternStore, DefaultOptions())

	habits, err := engine.RunOnce(context.Background(), timeAt(2024, 1, 15, 12, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(habits) != 0 {
		t.Errorf("expected no habit below min_days, got %+v", habits)
	}
}

func TestHabitSignatureIsStableAcrossCoarseTimeBucket(t *testing.T) {
	a := Habit{Categories: []string{"programming"}, WindowStartMinute: 530, WindowEndMinute: 560}
	b := Habit{Categories: []string{"programming"}, WindowStartMinute: 532, WindowEndMinute: 561}
	if a.Signature() != b.Signature() {
		t.Errorf("expected same 15-min bucket to produce the same signature: %s != %s", a.Signature(), b.Signature())
	}

	c := Habit{Categories: []string{"programming"}, WindowStartMinute: 700, WindowEndMinute: 730}
	if a.Signature() == c.Signature() {
		t.Error("expected a different time window to produce a different signature")
	}
}

func TestRunForeverStopStopsCleanly(t *testing.T) {
	store := events.NewSQLiteStore(t.TempDir() + "/activity.db")
	defer store.Close()
	activityStore := activity.NewStore(store)
	patternStore := NewPatternStore(t.TempDir() + "/patterns.db")
	defer patternStore.Close()

	engine := NewDiscoveryEngine(nil, activityStore, patternStore, DefaultOptions())
	engine.RunForever(context.Background(), timeMillis(10))
	engine.Stop() // must return, not hang
}
