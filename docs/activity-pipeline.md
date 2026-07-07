# Activity pipeline

Packages: `internal/sensors`, `internal/events`, `internal/privacy`,
`internal/semantic`, `internal/activity`, `internal/habits`,
`internal/pipeline` (tests only). Entirely separate from the agent core —
no runtime state is shared with `internal/agent`.

## Data flow

```
sensors.Sensor.Poll()
   │  (WindowFocusChanged / ClipboardChanged raw events)
   ▼
events.EventBus.Publish
   │  persists via events.Store (SQLite), then dispatches to subscribers
   ▼
semantic.Extractor            (local-only Ollama classification)
   │  publishes SemanticActivityInferred {category, label, ...}
   ▼
events.EventCompressor        (collapses an unbroken run of one category
   │                            into a single session)
   │  publishes ActivitySessionStarted / ActivitySessionEnded
   ├──────────────────────────────┐
   ▼                              ▼
activity.ContextEngine        activity.GraphBuilder
  (in-memory "what's           (records from→to category
   happening now" +              transitions to a SQLite-backed
   bounded recent history)       GraphStore)
   │
   ▼ (activity.Store reads completed sessions back out of events.Store)
habits.DiscoveryEngine.RunOnce
   │  mines n-gram patterns over N days of sessions
   ▼
publishes pattern.detected, persists to habits.PatternStore
```

`internal/privacy.Manager` sits alongside this flow, gating whether a
sensor's `Runner.Start` is even allowed to begin polling, and auditing
every grant/revoke/denial as an `ActivityEvent` on the same bus — so "what
did the system observe and why" is answerable from the same event log
everything else reads.

## `internal/events` — pub/sub bus + session compression

```go
type ActivityEvent struct { Type, Source string; Data map[string]any; Timestamp float64; ID string }
func New(eventType, source string, data map[string]any) ActivityEvent // stamps ID + now as Timestamp

type Handler func(ctx context.Context, event ActivityEvent) error
type EventBus struct { /* ... */ }
func NewEventBus(store Store) *EventBus // nil store = no persistence
func (b *EventBus) Subscribe(eventType string, handler Handler) // eventType="*" = all events
func (b *EventBus) Publish(ctx context.Context, event ActivityEvent) error
```

`Publish` first persists to `Store` (if configured), then fans the event
out to every matching handler **concurrently** via goroutines +
`sync.WaitGroup`; handler errors are logged (`slog.Warn`), never propagated
to the publisher — mirroring Python's `_safe_call` +
`gather(return_exceptions=True)`.

Event type constants (dot-namespaced, matching Python):
`window.focus_changed`, `clipboard.changed`, `privacy.permission_granted`,
`privacy.permission_revoked`, `privacy.permission_denied`,
`semantic.activity_inferred`, `activity.session_started`,
`activity.session_ended`, `pattern.detected`, `automation.suggested`.

### `Store` (persistence) — SQLite, pure Go

```go
type Store interface {
    Append(ctx context.Context, event ActivityEvent) error
    GetEvents(ctx context.Context, opts GetEventsOptions) ([]ActivityEvent, error)
    Close() error
}
```

`SQLiteStore` (backed by `modernc.org/sqlite`, no CGO) lazily opens/
migrates a single `activity_events` table (indexed on `type` and
`timestamp`) on first use. `database/sql`'s `*sql.DB` already pools
connections safely, so there's no extra locking beyond what it provides.
`GetEventsOptions{EventType, Since, SinceSet, Limit}` filters server-side;
default `Limit` is 100.

### `EventCompressor` — sessionizing

Subscribes only to `SemanticActivityInferred`. As long as consecutive
events report the **same category**, they extend one in-progress
`session` (updating `lastSeenAt`/`label`) rather than starting a new one —
"an unbroken run of the same category is exactly one session, however long
the poll loop keeps confirming it." A category change closes the current
session (publishing `ActivitySessionEnded` with `duration_seconds =
lastSeenAt - startedAt`) and opens a new one (`ActivitySessionStarted`).
Call `Flush` on shutdown so the last open session isn't lost.

## `internal/privacy` — deny-by-default permissions

```go
type Scope string // window_focus | clipboard_metadata | clipboard_content | screen_text
type Manager struct { /* ... */ }
func NewManager(store Store, bus *events.EventBus) *Manager
func (m *Manager) Initialize(ctx context.Context) error // loads persisted grants
func (m *Manager) IsGranted(scope Scope) bool            // false if Initialize wasn't called yet
func (m *Manager) Grant(ctx context.Context, scope Scope) error
func (m *Manager) Revoke(ctx context.Context, scope Scope) error
func (m *Manager) Deny(ctx context.Context, scope Scope, sensor string) error
```

Scopes are deliberately fine-grained — e.g. `clipboard_metadata` vs.
`clipboard_content` — so a user can allow "clipboard activity happened"
without ever allowing the system to see what was actually copied.
`IsGranted` logs a warning and denies if called before `Initialize`.
Every `Grant`/`Revoke`/`Deny` publishes an audit `ActivityEvent` on `bus`
(if configured) — `privacy.permission_granted` /
`_revoked` / `_denied`. Backed by a `SQLiteStore` sharing the same
`permissions` table pattern as `events.SQLiteStore` (often the same
physical database file).

## `internal/semantic` — local-only activity classification

> Deliberately bypasses `internal/router`: window titles and clipboard
> metadata are private activity data, and the router's complexity
> heuristic could pick a cloud provider if API keys happen to be
> configured for other parts of the app. This extractor talks directly to
> a `model.Model` that defaults to a local Ollama model, so classification
> never leaves the device regardless of what else is configured.

```go
func New(bus *events.EventBus, opts ...Option) (*Extractor, error)
func WithModel(m model.Model) Option       // override the local-only default
func WithCategories(categories []string) Option
```

Without `WithModel`, defaults to `ollama.New("", "")` — empty host
resolves via `OLLAMA_HOST`/the SDK's environment default, empty model
defaults to `"llama3.1"`.

Subscribes to `WindowFocusChanged` and `ClipboardChanged`. Maintains a
small in-memory `context` map (`app_name`, `window_title`,
`clipboard_kind`) updated per event; a clipboard event alone (with no
`app_name` seen yet) is ignored — "clipboard metadata alone rarely
justifies a new classification." On each qualifying event, it sends the
current context snapshot as JSON to the model with a fixed system prompt
asking for `{"category": "<one of: ...>", "label": "..."}`, using
`DefaultCategories` (`programming`, `terminal`, `reading_docs`,
`chat_messaging`, `email`, `web_browsing`, `reading_news`,
`meeting_video`, `watching_media`, `file_management`, `design`, `other`)
unless overridden. Parsing tries a direct JSON decode first, then falls
back to slicing from the first `{` to the last `}` to tolerate prose
wrapping. Only text signals are ever used — no screenshots. Classification
failures are logged and swallowed (return `nil`), not propagated as
pipeline errors.

## `internal/activity` — read models over the session stream

No separate schema of its own — everything is derived from
`ActivitySessionStarted`/`Ended` events already on the bus/store.

- **`Store`** (`NewStore(eventStore events.Store)`) — `GetSessions(ctx,
  GetSessionsOptions{Since, Until, Category, Limit})` reads
  `ActivitySessionEnded` events back out of `events.Store`, filtering
  client-side for `Category`/`Until` (over-fetching 5x when those filters
  are set, since the underlying store only filters by type+since+limit
  natively) and reconstructing `Session{Category, Label, StartedAt,
  EndedAt, DurationSeconds, Context}`. `SummarizeByCategory` sums
  `DurationSeconds` per category over a range.
- **`ContextEngine`** (`NewContextEngine(bus, historySize)`) — a purely
  in-memory read model of "what's happening right now": subscribes to
  session started/ended events, tracks one `CurrentActivity` plus a
  bounded FIFO (`boundedQueue`, the Go analogue of
  `collections.deque(maxlen=...)`) of the last `historySize` completed
  sessions. `Snapshot()` returns both. Downstream consumers (habit
  discovery) read history via `activity.Store` directly rather than
  `ContextEngine`, though both derive from the same event stream.
- **`GraphStore`** / **`GraphBuilder`** — a SQLite-backed
  `activity_transitions` table (`from_category, to_category, count,
  last_seen`, upserted with `count = count + 1` on repeat). `GraphBuilder`
  subscribes to `ActivitySessionStarted` and records a transition whenever
  the category differs from the previous session's.

## `internal/habits` — n-gram habit mining

```go
type Habit struct {
    Categories         []string
    DaysObserved       int
    WindowStartMinute  int
    WindowEndMinute    int
    AvgDurationSeconds float64
    Confidence         float64
}
func (h Habit) Signature() string // sha256(categories + 15-min-rounded window)[:16]

type Options struct { LookbackDays, MinDays, MaxStartSpreadMinutes int; NgramSizes []int }
func DefaultOptions() Options // 14, 3, 30, [1,2,3]

func NewDiscoveryEngine(bus, activityStore, patternStore, opts) *DiscoveryEngine
func (e *DiscoveryEngine) RunOnce(ctx context.Context, now float64) ([]Habit, error)
func (e *DiscoveryEngine) RunForever(ctx context.Context, interval time.Duration)
func (e *DiscoveryEngine) Stop()
```

This is a **batch analysis over history**, not an event-driven stage like
the rest of the pipeline — publishing `pattern.detected` is its only point
of contact with the bus.

### `RunOnce` algorithm

1. Pull all sessions since `now - LookbackDays*86400` from
   `activity.Store`.
2. `groupByDay` — bucket sessions by local calendar date, chronological
   within each day.
3. `mineHabits`:
   - For each day, for each n-gram size in `NgramSizes` (default 1, 2, 3),
     slide a window of `n` consecutive same-day sessions, joining their
     categories with `\x00` as a dedup key and summing their durations.
   - For each distinct key, take the **earliest** occurrence per day
     (`perDayStart`/`perDayDuration`).
   - Require the pattern to appear on at least `MinDays` distinct days.
   - Require the spread between the earliest and latest start-of-day
     minute across all observed days to be within `MaxStartSpreadMinutes`
     (default 30) — i.e., it has to happen at roughly the same time of day
     to count as a "habit."
   - Average the start minute and duration across observed days;
     `Confidence = daysObserved / LookbackDays`.
4. For each mined habit, look it up in `PatternStore` by `Signature()`
   (categories + 15-minute-rounded window, so near-identical habits dedup
   to the same signature). Skip publishing if it already exists and hasn't
   `materiallyChanged` (>5 minute drift in start/end window, or a changed
   `DaysObserved` count) — otherwise upsert and publish `pattern.detected`.

`RunForever`/`Stop` run this on a fixed ticker via a goroutine +
`context.CancelFunc`, the Go analogue of an `asyncio.Task` you can cancel.

`PatternStore` is a SQLite-backed `detected_patterns` table
(`signature` primary key, JSON `data`, `first_detected_at`,
`last_detected_at`) — its whole purpose is avoiding re-publishing the
same finding, unchanged, on every run.

## `internal/sensors` — the Sensor interface

> Platform-specific sensors (macOS clipboard/active-window, implemented in
> Python via PyObjC) are out of scope for this port: PyObjC has no Go
> equivalent, and embedding CGO/Objective-C here would break this SDK's
> no-CGO/cross-compilation story. The intended design is a separate Swift
> satellite process talking newline-JSON over stdout, consumed through this
> same `Sensor` interface — deferred, not blocking the rest of Phase 4.
>
> Sensors are deliberately **not** modeled as `tool.Tool`: a `Tool` is a
> synchronous, single-invocation function the LLM decides to call; a
> `Sensor` is a background loop that decides for itself when there is
> something worth reporting and pushes it onto the `EventBus`. Routing
> sensors through the ReAct loop would mean one model round-trip per poll
> tick, at LLM latency and cost.

```go
type Sensor interface {
    Name() string
    Scope() privacy.Scope
    PollInterval() time.Duration
    Poll(ctx context.Context) (*events.ActivityEvent, error) // nil, nil = nothing changed
}

type Manager struct { Bus *events.EventBus; Permissions *privacy.Manager }
func (m *Manager) Register(sensor Sensor)
func (m *Manager) StartAll(ctx context.Context) map[string]bool // name -> started
func (m *Manager) StopAll()
func (m *Manager) Status() map[string]bool
```

There is currently **no concrete `Sensor` implementation shipped** in this
port — only the interface and lifecycle manager. `Runner.Start` checks
`permissions.IsGranted(sensor.Scope())` before beginning to poll; if
denied, it audits a `Deny` event and returns
`simonerr.NewPermissionDeniedError(...)` rather than starting. A sensor
denied permission is logged and reported not-started in `StartAll`'s
result map, rather than failing the whole call.

## `internal/pipeline` — end-to-end wiring test only

No production code. Exists solely to hold a test that wires a synthetic
sensor through the full chain (sensor → bus → semantic → session
compression → activity store → graph → habit discovery) and verifies it
clean under `go test -race`.

See `examples/activity_pipeline_example` for a runnable demonstration —
it seeds 5 days of synthetic history to trigger pattern detection and
degrades gracefully if no local Ollama server is running.
