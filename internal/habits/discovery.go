package habits

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"simon-go/internal/activity"
	"simon-go/internal/events"
)

const secondsPerDay = 86400

// Habit is a recurring activity (length 1) or sequence of activities
// (length 2-3), mirroring Python's frozen @dataclass Habit.
type Habit struct {
	Categories         []string
	DaysObserved       int
	WindowStartMinute  int
	WindowEndMinute    int
	AvgDurationSeconds float64
	Confidence         float64
}

// Signature is a stable identity for dedup: categories + a coarse
// (15-minute) time bucket.
func (h Habit) Signature() string {
	roundedStart := (h.WindowStartMinute / 15) * 15
	roundedEnd := (h.WindowEndMinute / 15) * 15
	raw := fmt.Sprintf("%s::%d-%d", strings.Join(h.Categories, "|"), roundedStart, roundedEnd)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:16]
}

// ToMap renders the habit as a plain map, the Go analogue of Python's
// Habit.to_dict() (used both for persistence and for the pattern.detected
// event payload).
func (h Habit) ToMap() map[string]any {
	return map[string]any{
		"categories":           h.Categories,
		"days_observed":        h.DaysObserved,
		"window_start_minute":  h.WindowStartMinute,
		"window_end_minute":    h.WindowEndMinute,
		"avg_duration_seconds": h.AvgDurationSeconds,
		"confidence":           h.Confidence,
		"signature":            h.Signature(),
	}
}

// Options configures a DiscoveryEngine, mirroring HabitDiscoveryEngine's
// constructor keyword arguments.
type Options struct {
	LookbackDays          int
	MinDays               int
	MaxStartSpreadMinutes int
	NgramSizes            []int
}

// DefaultOptions mirrors Python's defaults (lookback_days=14, min_days=3,
// max_start_spread_minutes=30, ngram_sizes=(1, 2, 3)).
func DefaultOptions() Options {
	return Options{LookbackDays: 14, MinDays: 3, MaxStartSpreadMinutes: 30, NgramSizes: []int{1, 2, 3}}
}

// DiscoveryEngine mines completed sessions for recurring category n-grams.
// Unlike the EventBus subscribers in earlier pipeline stages, this is a
// batch analysis over history: publishing PatternDetected is its only
// point of contact with the rest of the pipeline.
type DiscoveryEngine struct {
	bus           *events.EventBus
	activityStore *activity.Store
	patternStore  *PatternStore
	opts          Options

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewDiscoveryEngine builds a DiscoveryEngine with opts (use
// DefaultOptions() for Python's defaults).
func NewDiscoveryEngine(bus *events.EventBus, activityStore *activity.Store, patternStore *PatternStore, opts Options) *DiscoveryEngine {
	return &DiscoveryEngine{bus: bus, activityStore: activityStore, patternStore: patternStore, opts: opts}
}

// RunOnce analyzes history once and publishes newly-found or
// materially-changed habits. now defaults to time.Now() when zero.
func (e *DiscoveryEngine) RunOnce(ctx context.Context, now float64) ([]Habit, error) {
	if now == 0 {
		now = nowSeconds()
	}
	since := now - float64(e.opts.LookbackDays)*secondsPerDay

	sessions, err := e.activityStore.GetSessions(ctx, activity.GetSessionsOptions{Since: since, SinceSet: true, Limit: 10_000})
	if err != nil {
		return nil, err
	}

	byDay := groupByDay(sessions)
	habits := mineHabits(byDay, e.opts)

	var fresh []Habit
	for _, habit := range habits {
		existing, err := e.patternStore.Get(ctx, habit.Signature())
		if err != nil {
			return fresh, err
		}
		if existing != nil && !materiallyChanged(existing.Data, habit) {
			continue
		}
		if err := e.patternStore.Upsert(ctx, habit.Signature(), habit.ToMap(), now); err != nil {
			return fresh, err
		}
		if e.bus != nil {
			event := events.New(events.PatternDetected, "HabitDiscoveryEngine", habit.ToMap())
			event.Timestamp = now
			if err := e.bus.Publish(ctx, event); err != nil {
				return fresh, err
			}
		}
		fresh = append(fresh, habit)
	}
	return fresh, nil
}

// RunForever runs RunOnce on a fixed interval until Stop is called,
// mirroring run_forever/stop — using a goroutine + context.CancelFunc
// instead of Python's asyncio.Task/CancelledError.
func (e *DiscoveryEngine) RunForever(ctx context.Context, interval time.Duration) {
	e.mu.Lock()
	if e.cancel != nil {
		e.mu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	e.cancel = cancel
	e.done = done
	e.mu.Unlock()

	// done is captured by value here rather than read back from e.done
	// inside the goroutine: if Stop() runs before this goroutine is
	// actually scheduled, it nils out e.done, and closing that stale field
	// read would panic with "close of nil channel".
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := e.RunOnce(loopCtx, 0); err != nil && loopCtx.Err() == nil {
				slog.Error("HabitDiscoveryEngine.RunOnce failed", "err", err)
			}
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop cancels the running loop started by RunForever and waits for it to exit.
func (e *DiscoveryEngine) Stop() {
	e.mu.Lock()
	cancel, done := e.cancel, e.done
	e.cancel, e.done = nil, nil
	e.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func minuteOfDay(timestamp float64) int {
	t := time.Unix(int64(timestamp), 0)
	return t.Hour()*60 + t.Minute()
}

// groupByDay groups sessions by local calendar date, chronological within
// each day.
func groupByDay(sessions []activity.Session) map[string][]activity.Session {
	ordered := make([]activity.Session, len(sessions))
	copy(ordered, sessions)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].StartedAt < ordered[j].StartedAt })

	groups := map[string][]activity.Session{}
	for _, s := range ordered {
		day := time.Unix(int64(s.StartedAt), 0).Format("2006-01-02")
		groups[day] = append(groups[day], s)
	}
	return groups
}

type occurrence struct {
	day         string
	startMinute int
	duration    float64
}

func mineHabits(byDay map[string][]activity.Session, opts Options) []Habit {
	occurrences := map[string][]occurrence{}
	categoriesByKey := map[string][]string{}

	for day, daySessions := range byDay {
		for _, n := range opts.NgramSizes {
			for i := 0; i+n <= len(daySessions); i++ {
				window := daySessions[i : i+n]
				categories := make([]string, n)
				var totalDuration float64
				for j, s := range window {
					categories[j] = s.Category
					totalDuration += s.DurationSeconds
				}
				key := strings.Join(categories, "\x00")
				categoriesByKey[key] = categories
				occurrences[key] = append(occurrences[key], occurrence{
					day: day, startMinute: minuteOfDay(window[0].StartedAt), duration: totalDuration,
				})
			}
		}
	}

	var habits []Habit
	for key, entries := range occurrences {
		perDayStart := map[string]int{}
		perDayDuration := map[string]float64{}
		for _, e := range entries {
			if start, ok := perDayStart[e.day]; !ok || e.startMinute < start {
				perDayStart[e.day] = e.startMinute
				perDayDuration[e.day] = e.duration
			}
		}

		if len(perDayStart) < opts.MinDays {
			continue
		}

		minStart, maxStart := minMaxInts(perDayStart)
		if maxStart-minStart > opts.MaxStartSpreadMinutes {
			continue
		}

		var sumStart, sumDuration float64
		for day, start := range perDayStart {
			sumStart += float64(start)
			sumDuration += perDayDuration[day]
		}
		n := float64(len(perDayStart))
		avgStart := int(sumStart/n + 0.5)
		avgDuration := sumDuration / n

		habits = append(habits, Habit{
			Categories:         categoriesByKey[key],
			DaysObserved:       len(perDayStart),
			WindowStartMinute:  avgStart,
			WindowEndMinute:    avgStart + int(avgDuration/60+0.5),
			AvgDurationSeconds: avgDuration,
			Confidence:         n / float64(opts.LookbackDays),
		})
	}
	return habits
}

func minMaxInts(m map[string]int) (min, max int) {
	first := true
	for _, v := range m {
		if first {
			min, max = v, v
			first = false
			continue
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return
}

func materiallyChanged(existing map[string]any, habit Habit) bool {
	if toIntOr(existing["days_observed"], habit.DaysObserved) != habit.DaysObserved {
		return true
	}
	if abs(toIntOr(existing["window_start_minute"], habit.WindowStartMinute)-habit.WindowStartMinute) > 5 {
		return true
	}
	if abs(toIntOr(existing["window_end_minute"], habit.WindowEndMinute)-habit.WindowEndMinute) > 5 {
		return true
	}
	return false
}

// toIntOr mirrors Python's dict.get(key, default): returns def when v is
// nil (key absent), rather than silently coercing a missing value to 0.
func toIntOr(v any, def int) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return def
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
