// Command activity_pipeline_example mirrors Python's
// examples/activity_pipeline_example.py — the local-first activity
// observation pipeline (Fases 0-4). Wires together every piece built for
// the "understand digital behavior locally" project: PermissionManager
// (privacy, deny-by-default) -> EventBus (persisted to SQLite) ->
// SemanticEventExtractor -> EventCompressor (sessions) -> ActivityGraph +
// ContextEngine (structure) -> HabitDiscoveryEngine (patterns).
//
// Everything is scoped to a throwaway SQLite file under a temp dir, so
// running this program never touches a real .simon_activity/activity.db.
//
// Adaptation note: Python's step 2/3 also start a live macOS
// ActiveWindowSensor (simon.sensors.macos, pyobjc-only) and poll it for a
// few seconds before falling through to the synthetic habit history.
// simon-go's internal/sensors package has no macOS window-focus sensor
// implementation (only the generic Sensor/Runner/Manager scaffolding), so
// that live-observation step is skipped entirely here — everything else
// (privacy grant, semantic extractor + event compressor + activity graph +
// context engine wiring, synthetic 5-day habit seeding, and habit
// discovery) is ported as-is.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"simon-go/internal/activity"
	"simon-go/internal/events"
	"simon-go/internal/habits"
	"simon-go/internal/privacy"
	"simon-go/internal/semantic"
)

// grantPrivacy builds and initializes a privacy.Manager, then grants
// window-focus access — nothing observes anything until explicitly
// granted, mirroring Python's deny-by-default PermissionManager.
func grantPrivacy(ctx context.Context, dbPath string, bus *events.EventBus) *privacy.Manager {
	manager := privacy.NewManager(privacy.NewSQLiteStore(dbPath), bus)
	if err := manager.Initialize(ctx); err != nil {
		log.Fatal(err)
	}
	if err := manager.Grant(ctx, privacy.WindowFocus); err != nil {
		log.Fatal(err)
	}
	return manager
}

// seedHabitHistory seeds 5 days of a real recurring pattern directly into
// the event store (bypassing the live bus, since this represents past
// activity), ending yesterday so it falls inside HabitDiscoveryEngine's
// default 14-day lookback.
func seedHabitHistory(ctx context.Context, store events.Store) {
	base := time.Now().AddDate(0, 0, -5)
	base = time.Date(base.Year(), base.Month(), base.Day(), 8, 45, 0, 0, base.Location())

	for day := 0; day < 5; day++ {
		morning := base.AddDate(0, 0, day)

		email := events.New(events.ActivitySessionEnded, "seed", map[string]any{
			"category": "email", "label": "leyendo email", "duration_seconds": 600.0,
		})
		email.Timestamp = float64(morning.Add(10 * time.Minute).Unix())
		if err := store.Append(ctx, email); err != nil {
			log.Fatal(err)
		}

		slack := events.New(events.ActivitySessionEnded, "seed", map[string]any{
			"category": "chat_messaging", "label": "revisando Slack", "duration_seconds": 900.0,
		})
		slack.Timestamp = float64(morning.Add(25 * time.Minute).Unix())
		if err := store.Append(ctx, slack); err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "simon_activity_demo_")
	if err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(tmpDir, "activity.db")

	eventStore := events.NewSQLiteStore(dbPath)
	bus := events.NewEventBus(eventStore)

	fmt.Println("=== 1. Privacy ===")
	permissions := grantPrivacy(ctx, dbPath, bus)
	fmt.Printf("  window_focus granted: %v\n", permissions.IsGranted(privacy.WindowFocus))

	fmt.Println("\n=== 2. Semantic extraction + structure, attached to the live bus ===")
	extractor, err := semantic.New(bus)
	if err != nil {
		// No local Ollama server running (or another init failure) — the
		// rest of the pipeline still runs fine on the synthetic history
		// seeded below, mirroring Python's graceful degradation.
		fmt.Printf("  semantic extractor unavailable, continuing without it: %v\n", err)
	} else {
		extractor.Attach()
	}

	compressor := events.NewEventCompressor(bus)
	compressor.Attach()

	graphStore := activity.NewGraphStore(dbPath)
	graphBuilder := activity.NewGraphBuilder(bus, graphStore)
	graphBuilder.Attach()

	contextEngine := activity.NewContextEngine(bus, 20)
	contextEngine.Attach()

	fmt.Println("\n=== 3. Live sensor observation ===")
	fmt.Println("  skipped: simon-go has no macOS active-window sensor implementation yet")
	if err := compressor.Flush(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  ContextEngine snapshot: %+v\n", contextEngine.Snapshot())

	fmt.Println("\n=== 4. Habit Discovery over 5 days of synthetic history ===")
	seedHabitHistory(ctx, eventStore)
	activityStore := activity.NewStore(eventStore)
	patternStore := habits.NewPatternStore(dbPath)

	var foundPatterns []events.ActivityEvent
	bus.Subscribe(events.PatternDetected, func(_ context.Context, event events.ActivityEvent) error {
		foundPatterns = append(foundPatterns, event)
		return nil
	})

	engine := habits.NewDiscoveryEngine(bus, activityStore, patternStore, habits.DefaultOptions())
	found, err := engine.RunOnce(ctx, 0)
	if err != nil {
		log.Fatal(err)
	}

	for _, habit := range found {
		startH, startM := habit.WindowStartMinute/60, habit.WindowStartMinute%60
		endH, endM := habit.WindowEndMinute/60, habit.WindowEndMinute%60
		fmt.Printf("  habit detected: %v | %d days | %02d:%02d-%02d:%02d | confidence=%.2f\n",
			habit.Categories, habit.DaysObserved, startH, startM, endH, endM, habit.Confidence)
	}
	fmt.Printf("  pattern.detected events published: %d\n", len(foundPatterns))

	fmt.Printf("\nDemo database: %s\n", dbPath)

	if err := eventStore.Close(); err != nil {
		log.Fatal(err)
	}
	if err := graphStore.Close(); err != nil {
		log.Fatal(err)
	}
	if err := patternStore.Close(); err != nil {
		log.Fatal(err)
	}
	if err := permissions.Close(); err != nil {
		log.Fatal(err)
	}
}
