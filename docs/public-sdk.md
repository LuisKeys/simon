# Public SDK facade: `simon`, `model`, `tool`, `knowledge`, `memory`

Packages: `simon`, `model`, `tool`, `knowledge`, `memory`, plus
`pkg/simonerr` (already public) and one new exported seam in
`internal/agent` (`BuildProviderModel`). This is the embeddable surface for
host applications that want Simon as a library rather than a CLI: build a
`Runtime`, open one or more `Session`s against it, `Run`/`Stream`/
`RunStructured` prompts, and observe `Event`s.

Everything here sits *above* the agent core, not inside it —
`simon.Session` wraps an `internal/agent.Agent` the same way `cmd/simon`
does, and the five public packages exist so a consuming application never
has to import anything under `internal/`.

```
simon         Runtime + Session — the facade itself
 ├── model    public Model/Message/ToolSpec contract (+ EchoModel)
 ├── tool     public Tool contract + Registry
 ├── knowledge public Searcher/Store/Embedder contract
 ├── memory   public Memory/Factory contract (+ NewInMemory/NewJSONFile)
 └── pkg/simonerr  session/runtime error constructors (ErrSessionBusy, ...)
```

## Why a parallel type system instead of aliasing `internal/*`

> Public types here (Response, Event, Usage, ...) intentionally do not
> alias internal/agent/response's types even where the shapes currently
> match: internal packages are free to change shape without that silently
> changing this package's contract.

Every public package repeats this rule for its own types: `simon.Response`
is hand-maintained against `internal/agent/response.AgentResponse`;
`simon.Settings` against `internal/config.Settings`; `model.Message`
against `internal/model.Message`; `memory.Message` against
`internal/memory.Message` (and adds `ToolCallID`/`CreatedAt`/`Metadata`,
none of which the internal store persists yet — see below). Each package's
`adapt.go` holds the translation in one direction (`ToInternal`) and,
where needed, the other (`knowledge.ToAgentSearcher`).

## `simon.Runtime` — shared resources across Sessions

```go
func New(opts ...Option) (*Runtime, error)
func (rt *Runtime) RegisterTool(t tool.Tool) error
func (rt *Runtime) RegisterTools(tools ...tool.Tool) error
func (rt *Runtime) NewSession(id string, opts ...SessionOption) (*Session, error)
func (rt *Runtime) Close() error
```

`New` with no options loads settings from the environment/`.env` (same
source as `cmd/simon`), starts with an empty `tool.Registry`, an
`AllowAll` approval policy, and a discarding `slog.Logger`. A `Runtime`
holds everything a `Session` needs but doesn't own alone: settings, the
internal `router.Router`, the tool registry, the `ApprovalPolicy`, the
`EventHandler`, a `MemoryFactory`, a default `knowledge.Searcher`, and an
optional concurrency semaphore. `Close` is idempotent and cancels/closes
every `Session` it created.

Construction options (`simon/options.go`):

| Option | Effect |
|---|---|
| `WithEnvironment()` | Explicit form of the default — loads settings from env/`.env`. |
| `WithSettings(simon.Settings)` | Overlays non-zero fields onto the base settings (env by default). |
| `WithModel(model.Model)` | Pins every Session to one custom `Model`, bypassing the router entirely. |
| `WithRouter(ModelRouter)` | Replaces the built-in provider-selection heuristics; resolved once per Session (at `NewSession`), not per prompt — a documented divergence from the default router's per-prompt resolution. |
| `WithMemoryFactory(MemoryFactory)` | Gives every Session its own `memory.Memory` via `factory.NewMemory(ctx, sessionID)`. |
| `WithKnowledgeBase(knowledge.Searcher)` | Default knowledge base for every Session (overridable per-session). |
| `WithToolRegistry(*tool.Registry)` | Replaces the registry outright instead of `RegisterTool`/`RegisterTools`. |
| `WithEventHandler(EventHandler)` | Observes every `Event` from every Session; panics are recovered so a bad handler can't crash a run. |
| `WithLogger(*slog.Logger)` | Default is a silent `slog.DiscardHandler` logger — errors still return normally. |
| `WithMaxConcurrentRuns(n)` | Caps concurrent Session runs runtime-wide via a buffered semaphore; `0` (default) is unlimited. |
| `WithApprovalPolicy(ApprovalPolicy)` | Gates every registered tool call; default `AllowAll` permits everything. |

Session-scoped options (`SessionOption`): `WithSystemPrompt`,
`WithMaxSteps`, `WithSessionKnowledge` (overrides the Runtime's default
knowledge base for one Session only).

## `simon.Session` — one conversation or task run

```go
func (rt *Runtime) NewSession(id string, opts ...SessionOption) (*Session, error)
func (s *Session) ID() string
func (s *Session) Run(ctx context.Context, prompt string) (Response, error)
func (s *Session) Stream(ctx context.Context, prompt string) (<-chan Event, error)
func (s *Session) Cancel()
func (s *Session) Clear(ctx context.Context) error
func (s *Session) Close() error
func RunStructured[T any](ctx context.Context, s *Session, prompt string) (T, error)
```

> A Runtime may host many concurrent Sessions; within a single Session,
> only one Run/Stream/RunStructured may be active at a time — a second
> call while one is in flight returns simonerr.ErrSessionBusy.

`newSession` wires an `internal/agent.Agent` from the Runtime's shared
resources: `agent.WithOnEvent(sess.handleAgentEvent)` always; memory via
`rt.memoryFactory` → `memory.ToInternal`; knowledge via
`knowledge.ToAgentSearcher` (session override beats the runtime default);
every registered tool via `tool.ToInternal(t, sess.approve)`, which is
where `ApprovalPolicy` gets its hook — `internal/tool`/`internal/agent`
have no native call-interception point, so `tool.ToInternal`'s `approve`
callback parameter is that seam; and a model override via
`rt.buildModelOverride` when `WithModel`/`WithRouter` was set.

- **`Run`** blocks until the agent loop finishes, emitting
  `run.started` and a terminal `run.completed`/`run.failed`/
  `run.cancelled` event around it.
- **`Stream`** does the same work on a goroutine and returns a buffered
  (64) `<-chan Event` immediately; `sendEvent` drops non-terminal events
  under backpressure but always delivers the terminal one, evicting an
  older buffered event if necessary — a slow or absent consumer can never
  lose the final outcome or stall the run.
- **`Cancel`** cancels the session's active run (no-op if none); a
  cancelled context surfaces as `run.cancelled` (detected via
  `errors.Is(err, context.Canceled)`), not `run.failed`.
- **`RunStructured[T]`** is a generic top-level function, not a method
  (Go doesn't allow generic methods): it mirrors `Run`'s event bookkeeping
  but delegates to `internal/agent.RunStructured[T]`. On exhaustion it
  returns a `*simonerr.StructuredOutputError` (`errors.As`-recoverable)
  carrying the raw text and attempt count.
- **`Clear`** erases the session's memory, if any. **`Close`** cancels any
  active run and closes the session's memory; idempotent.

## `simon.Response`, `simon.Event`, `simon.ApprovalPolicy`

```go
type Response struct {
    Text string; Usage Usage; ToolCalls []ToolCall; Steps int
    Model string; Provider string; StopReason StopReason; Metadata map[string]any
}
```

`Event.Type` is one of: `run.started`, `model.selected`, `tool.requested`,
`tool.started`, `tool.completed`, `retry.attempted`, and the three
terminal types `run.completed` / `run.failed` / `run.cancelled`.
`response.delta` and `tool.failed` are declared `EventType` constants but
not currently emitted by `translateEventType` — reserved for future
streaming/error-detail support, not a gap in the docs. The internal
agent's `response_received` event is intentionally not forwarded; its
data (step count) is folded into the `run.completed` event Session
synthesizes itself.

```go
type ApprovalPolicy interface {
    Approve(ctx context.Context, request ApprovalRequest) (bool, error)
}
```

`Approve` returning `(false, nil)` denies silently; a non-nil error denies
and surfaces why. `AllowAll{}` (the default) always allows. See
`examples/public_tool_approval` for a policy that denies one named tool
and allows everything else.

## `model` — pluggable model providers

```go
type Model interface { Complete(ctx context.Context, request Request) (Response, error) }
type EchoModel struct{}   // replies "Simon (echo): <last user message>" — no network
func ToInternal(m Model) internalmodel.Model
```

`Request`/`Response`/`Message`/`ToolSpec`/`Role` mirror
`internal/model`'s shapes field-for-field but are a separate type family
per the no-aliasing rule above. `ToInternal` is exported (not kept
package-private) specifically so `simon` — a different directory, same
module — can reuse the translation instead of duplicating it. Custom
providers plug in via `simon.WithModel(myModel{})`.

## `tool` — pluggable tools with approval support

```go
type Tool interface {
    Name() string; Description() string; Schema() map[string]any
    Execute(ctx context.Context, arguments json.RawMessage) (any, error)
}
func New[P any](name, description string, fn func(ctx context.Context, params P) (any, error)) Tool
func NewRaw(name, description string, schema map[string]any, handler Handler) Tool
func NewRegistry(tools ...Tool) *Registry
func ToInternal(t Tool, approve func(ctx context.Context, name string, arguments json.RawMessage) error) internaltool.Tool
```

`tool.New[P]` reuses `internal/tool`'s reflection-based schema generation
(`SchemaFor[P]()`) over `P`'s `json`/`jsonschema` struct tags — same
approach and same reason as `internal/tool.New[P any]` (see
[agent-core.md](agent-core.md)), just re-exposed publicly. `Execute`
returns `(any, error)` instead of `internal/tool.Tool`'s `(string,
error)`, so handlers can return structured results without pre-serializing
— `tool.ToInternal`'s `stringify` helper JSON-encodes anything that isn't
already a string when bridging to the internal loop. `NewRaw` exists for
schemas only known at runtime (MCP tools, per `internal/mcp`); register
via `Runtime.RegisterTool`/`RegisterTools` or `simon.WithToolRegistry`.

## `knowledge` — pluggable retrieval

```go
type Searcher interface { Search(ctx context.Context, query string, topK int) ([]Hit, error) }
type Store interface { Searcher; Add(...) (AddResult, error); Remove(ctx, source string) error; Close() error }
type Embedder interface { Embed(ctx, text string) ([]float32, error); EmbedBatch(ctx, texts []string) ([][]float32, error) }
func Open(embedder Embedder, storePath string, opts ...Option) (Store, error)
func OpenFromEnv(storePath string, opts ...Option) (Store, error)
func ToAgentSearcher(s Searcher) *searcherAdapter
```

`Open`/`OpenFromEnv` wrap `internal/knowledge.KnowledgeBase` (defaults:
`chunkSize=500`, `overlap=50`, overridable via `WithChunkSize`/
`WithOverlap`); `OpenFromEnv` additionally selects an embedding provider
from `EMBEDDING_PROVIDER`/`EMBEDDING_MODEL`/API-key env vars the same way
the CLI's `index` subcommand does. The adapter's `Remove` always returns a
`simonerr.NewKnowledgeError` stub ("Remove is not supported by this store
yet") until the SIDX index format supports deletion — see
[knowledge-base.md](knowledge-base.md). `ToAgentSearcher` bridges a public
`Searcher` into `internal/agent`'s unexported `KnowledgeSearcher`
interface via Go's structural typing (no exported internal type needed).

## `memory` — pluggable conversation history

```go
type Memory interface {
    Add(ctx context.Context, message Message) error
    List(ctx context.Context) ([]Message, error)
    Clear(ctx context.Context) error
    Close() error
}
type Factory interface { NewMemory(ctx context.Context, sessionID string) (Memory, error) }
type FactoryFunc func(ctx context.Context, sessionID string) (Memory, error) // functional Factory adapter
func NewInMemory() Memory
func NewJSONFile(path string) Memory   // .simon_chats/<basename of path>
func ToInternal(m Memory) internalmemory.Memory
```

`memory.Message` adds `ToolCallID`, `CreatedAt`, and `Metadata` beyond
`internal/memory.Message`'s role/content — but `NewInMemory`, `NewJSONFile`,
and `ToInternal` all currently accept and discard those three fields; only
role/content persist, matching what `internal/memory`'s backends support
today. A `Runtime` gets per-session storage via
`simon.WithMemoryFactory(memory.FactoryFunc(...))`, most commonly wrapping
`memory.NewJSONFile(sessionID + "-" + basePath)` so each session ID gets
its own file.

## `internal/agent.BuildProviderModel` — the one new internal seam

```go
func BuildProviderModel(settings config.Settings, choice router.Choice) (model.Model, error)
```

Exported specifically so the `simon` facade can turn a `router.Choice`
into a live provider client (`openai.New`/`anthropic.New`/`ollama.New`,
falling back to the *internal* `model.EchoModel` — distinct from the
*public* `model.EchoModel` example providers use) without duplicating
`internal/agent.resolveModel`'s switch statement. Used by
`Runtime.buildModelOverride` when a custom `simon.ModelRouter` is
configured via `WithRouter`.

## The 10 `examples/public_*` programs

See [examples.md](examples.md) for the full table; all ten use only the
`simon`/`model`/`tool`/`knowledge`/`memory` packages (never `internal/`
directly) and are wired into `.vscode/launch.json` under "Simon-Go: Public
SDK - \<name\>".
