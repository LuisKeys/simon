# Agent core

Packages: `internal/agent`, `internal/agent/response`, `internal/model`
(+ `model/openai`, `model/anthropic`, `model/ollama`), `internal/tool`,
`internal/router`, `internal/memory`, `internal/multi`, `internal/config`,
`internal/reliability`, `pkg/simonerr`.

## Dependency shape

```
internal/agent
 ├── internal/agent/response   (shared result types: Usage, ToolCall, AgentResponse, KnowledgeHit)
 ├── internal/config           (env-backed Settings)
 ├── internal/memory           (pluggable conversation history)
 ├── internal/model            (Model/Message/ToolSpec/Role — no provider SDKs)
 │    ├── internal/model/openai
 │    ├── internal/model/anthropic
 │    └── internal/model/ollama
 ├── internal/reliability      (generic retry helper)
 ├── internal/router           (provider/model selection, no model.Model construction)
 └── internal/tool             (registration, JSON-schema generation, dispatch)

internal/multi → internal/agent, internal/agent/response, internal/config
```

`internal/model` defines only the `Model` interface, `Message`, `ToolSpec`,
and `Role` — no third-party SDK import — so anything that only needs the
type contract (e.g. `internal/tool`) doesn't pull in the OpenAI/Anthropic/
Ollama SDKs. Each provider lives in its own subpackage instead.

`internal/router.Resolve` deliberately returns a `Choice{Provider, Model}`
value rather than constructing a `model.Model` itself: `internal/model`
already depends on `internal/router`-adjacent config, and having `router`
construct a `model.Model` would create a `router → model → router` import
cycle. `internal/agent.resolveModel` is the one place that turns a `Choice`
into a live client.

## `internal/agent.Agent` — the ReAct loop

```go
type Agent struct {
    Name         string
    SystemPrompt string
    MaxSteps     int          // default 6
    TotalUsage   response.Usage
    // unexported: settings, router, modelName, memory, tools, onEvent,
    // knowledge, modelOverride
}

func New(settings config.Settings, opts ...Option) *Agent
func (a *Agent) Run(ctx context.Context, prompt string) (response.AgentResponse, error)
func (a *Agent) ModelName() string
```

Construction options: `WithMemory`, `WithTools`, `WithSystemPrompt`,
`WithMaxSteps`, `WithModel(name)`, `WithName`, `WithOnEvent(fn)`,
`WithModelOverride(m model.Model)` (bypasses the router entirely, for
deterministic tests), `WithKnowledge(k KnowledgeSearcher)`.

### `Run` step by step (`internal/agent/agent.go`)

1. **Resolve a model.** `resolveModel` uses `modelOverride` if set;
   otherwise it calls `router.Resolve` with the pinned `modelName` (if any)
   and the prompt as the task-complexity hint, then builds a concrete
   `openai.New` / `anthropic.New` / `ollama.New` client, or falls back to
   `model.EchoModel{}`. Fires a `model_selected` event.
2. **Seed messages.** `seedMessages` appends the prompt to `Memory` (if
   configured) and replays the full history as messages; otherwise it's a
   single user message. `SystemPrompt`, if set, is prepended.
3. **Tool shorthand check.** `maybeCallToolShorthand` intercepts prompts of
   the literal form `tool:name {json_args}`, calling the named tool
   directly and returning its output as the response — bypassing the model
   entirely. This is a fixed, tiny format ported from Python; it does not
   involve the LLM.
4. **Knowledge context.** If a `KnowledgeSearcher` is attached,
   `knowledgeContext` searches it with `topK=2` and, if there are hits,
   appends a `"Relevant knowledge:\n- (source) text"` system message.
5. **First completion.** `complete` wraps `model.Complete` in
   `reliability.Retry`, tracking `TotalUsage` and firing `retry_attempted`
   on each retry.
6. **Tool-call loop.** While the response requests tool calls and
   `step < MaxSteps`: append the assistant's message (including
   `ToolCalls`), run each call via `tool.RunToolCall` (firing `tool_called`
   with a 200-char-truncated result), append a `RoleTool` message per
   result, then call `complete` again. Loop exits when the model stops
   requesting tools or `MaxSteps` is hit.
7. **Persist + emit.** The final response text is written back to `Memory`
   (if configured), and a `response_received` event fires with usage and
   step count.

### Events

`Agent.Event{Type, Data}` fires at exactly four points, matching Python's
`Agent._emit` call sites: `model_selected`, `tool_called`,
`retry_attempted`, `response_received`. Register a handler with
`WithOnEvent`. `examples/hooks_agent` demonstrates consuming all four.

### Structured output — `RunStructured[T]`

`internal/agent/structured.go` runs the same seed/knowledge/tool-loop
machinery as `Run`, then appends a schema-instruction system message
(reflected from `T` via `github.com/invopop/jsonschema`) and parses the
final text into `T`:

- `parseStructured[T]` strips ```` ``` ````-fenced code blocks and slices
  from the first `{` to the last `}` before `json.Unmarshal`, tolerating
  minor LLM formatting noise.
- On parse failure, it retries up to `settings.SimonStructuredRetries`
  times (env `SIMON_STRUCTURED_RETRIES`, default 2), appending a corrective
  user message quoting the JSON error each time.
- After exhausting retries, it returns a
  `simonerr.NewStructuredOutputError(msg, rawText, attempts)` — recoverable
  via `errors.As` to inspect the raw model output.

See `examples/structured_output_agent` for a `Recipe` struct example (the
Go analogue of a Pydantic model).

### `maybeCallToolShorthand` vs. the LLM tool-call path

There are two distinct ways a tool ends up called:

1. **Model-initiated**: the LLM's response includes `ToolCalls`, handled by
   the ReAct loop's step 6 above.
2. **Caller-initiated shorthand**: the user's literal prompt text is
   `tool:name {json}`, handled before the model is ever invoked. This
   exists for scripting/testing convenience and mirrors a small explicit
   format Python also supports.

## `internal/model` — provider abstraction

```go
type Role string // "system" | "user" | "assistant" | "tool"

type Message struct {
    Role       Role
    Content    string
    ToolCalls  []response.ToolCall // set on assistant messages requesting tools
    ToolCallID string               // set on RoleTool messages
}

type ToolSpec struct { Name, Description string; Parameters map[string]any }

type Model interface {
    Complete(ctx context.Context, messages []Message, tools []ToolSpec) (response.AgentResponse, error)
}
```

`EchoModel{}` is the network-free fallback: it replies with
`"Simon (echo): " + <last user message>`. It backs the router's ultimate
fallback and Phase 1's deterministic tests, and is used directly (no API
key needed) by `examples/tool_runner_example`.

### Provider adapters

| Package | Backing SDK | Tool-call support | Notes |
|---|---|---|---|
| `model/openai` | `github.com/openai/openai-go/v2` | yes | |
| `model/anthropic` | `github.com/anthropics/anthropic-sdk-go` | yes | |
| `model/ollama` | `github.com/ollama/ollama/api` | **no** — `tools` param is ignored | `ollama.New(host, model)`; empty `host` resolves via `OLLAMA_HOST`/SDK environment default; empty `model` defaults to `"llama3.1"`. Non-streaming (`Stream: false`), `Think` field defaults to `false`. |

## `internal/router` — provider/model selection

```go
type Provider string // "openai" | "anthropic" | "ollama" | "echo"
type Choice struct { Provider Provider; Model string }
type ResolveOptions struct { Model string; Task string; ComplexTask *bool }

func New(settings config.Settings) *Router
func (r *Router) Resolve(opts ResolveOptions) Choice
```

Resolution priority, mirroring Python's `ModelRouter.resolve`:

1. An explicit provider label (`"openai_model"`, `"anthropic_model"`,
   `"ollama_model"`, case-insensitive, normalized via `labelMap`) **wins
   outright** if that provider is configured — no fallback if it isn't.
2. Otherwise, task complexity decides ordering. `isComplexTask` does a
   case-insensitive substring match against a fixed keyword list
   (`complex`, `difficult`, `hard`, `multi-step`, `reasoning`, `analyze`,
   `analysis`, plus several Spanish equivalents) unless
   `ResolveOptions.ComplexTask` overrides it explicitly.
   - **Complex** → try OpenAI, then Anthropic, then Ollama (cloud-first).
   - **Simple** → try Ollama, then OpenAI, then Anthropic (local-first,
     since Ollama is free/fast for easy tasks).
3. If nothing is configured, `Choice{ProviderEcho, ""}`.

A provider counts as "configured" via `hasOpenAI`/`hasAnthropic`
(non-empty API key *and* model name) or `hasOllama` — the latter checks
the **raw** `OLLAMA_HOST` environment variable, not the defaulted
`Settings.OllamaHost` field, so Ollama only counts as explicitly configured
when the user set a host themselves, not merely because of the built-in
`http://localhost:11434` default.

## `internal/tool` — registration and dispatch

```go
type Tool struct { Name, Description string; Schema map[string]any /* + fn */ }
func New[P any](name, description string, fn func(ctx, P) (string, error)) Tool
func NewRaw(name, description string, schema map[string]any, fn func(ctx, json.RawMessage) (string, error)) Tool

type Registry struct { /* ... */ }
func NewRegistry(tools ...Tool) *Registry
func RunToolCall(reg *Registry, call response.ToolCall) (result string, isError bool)
```

`New[P any]` reflects `P`'s `json`/`jsonschema` struct tags into a JSON
schema via `github.com/invopop/jsonschema` (flattened, no `$ref`,
`additionalProperties: false`), then wraps a typed `fn` so a model's raw
JSON arguments unmarshal into `P` before calling it. This replaces Python's
`inspect.signature`-based `@tool` decorator: Go binaries don't retain
parameter names at runtime, so the parameter shape must be an explicit
struct, not arbitrary function args (see `examples/builtin_tools_agent`).

`NewRaw` skips reflection entirely, for tools whose schema is only known at
runtime — this is what `internal/mcp` uses, since an MCP server's JSON
Schema comes from the remote process, not a Go type.

`RunToolCall` is the single dispatch function shared by both `Agent.Run`
and `tool.Runner`, so a missing tool, malformed arguments, or tool-function
error all come back as `(errorText, true)` — fed to the model as an error
tool result instead of crashing the run.

### `tool.Runner` — the standalone turn-by-turn loop

`internal/tool/runner.go` exposes the same model↔tool loop `Agent.Run`
bakes in, but turn by turn, for callers that want to inspect or intervene
between turns:

```go
func NewRunner(m model.Model, opts ...RunnerOption) *Runner
func (r *Runner) Turns(ctx context.Context) iter.Seq2[*response.AgentResponse, error]
func (r *Runner) UntilDone(ctx context.Context) (response.AgentResponse, error)
func (r *Runner) GenerateToolCallResponse() ([]ToolResult, bool)
func (r *Runner) AppendMessages(msgs ...model.Message) // take over history for this turn
```

Python exposes this as dual `__iter__`/`__aiter__` protocols because
`asyncio` has no synchronous concurrency primitive; Go collapses both to a
single `iter.Seq2` range-over-func iterator (`Turns`). `UntilDone` drives
it to completion for callers who just want the final answer.
`GenerateToolCallResponse`/`AppendMessages` let a caller compute tool
results, inspect/log them, and hand back a possibly-modified message list
before the next turn — see `examples/tool_runner_example`.

Unlike `Agent`, `Runner` always takes a concrete `model.Model` — it never
resolves a provider itself, keeping provider selection a single
responsibility of `internal/agent`.

## `internal/memory` — pluggable conversation history

```go
type Message struct { Role, Content string }
type Memory interface {
    Add(ctx context.Context, role, content string) error
    List(ctx context.Context) ([]Message, error)
    Clear(ctx context.Context) error
}
```

Two implementations:

- **`InMemory`** — mutex-guarded slice, process-lifetime only. The mutex
  exists because, unlike Python's single-threaded asyncio loop, Go callers
  (e.g. `internal/multi.Pool`'s goroutines) may call `Add`/`List`/`Clear`
  concurrently.
- **`JSONFile`** — one human-readable JSON file per conversation, rooted at
  `.simon_chats/<basename>`. Only the base filename is used
  (`filepath.Base`), so callers cannot path-traverse out of the chats
  directory. Lazily loads on first use; every `Add`/`Clear` rewrites the
  whole file. See `examples/persistent_memory_agent` for cross-process
  recall.

## `internal/multi` — multi-agent patterns

Python's `asyncio.gather` becomes plain goroutines + `sync.WaitGroup`
throughout; there is no `run`/`run_async` duality here either.

- **`Group`** (`NewGroup(map[string]*agent.Agent)` → `RunAll(ctx, prompt)`)
  — runs every named agent concurrently over the **same** prompt. Unlike
  `asyncio.gather`, Go's goroutines always run to completion even if one
  errors; `RunAll` returns the first error by map-key order once all
  finish. See `examples/parallel_agents`.
- **`Pool`** (`NewPool(returnExceptions bool)` → `Run(ctx, []Task)`) — runs
  heterogeneous `(agent, prompt)` pairs concurrently, preserving input
  order in the result slice. `ReturnExceptions=true` mirrors
  `asyncio.gather(return_exceptions=True)`: per-task errors are reported in
  each `Result` instead of failing the whole call. See
  `examples/agent_pool_example`.
- **`Triage`** (`NewTriage(settings, agents, descriptions, routerOpts...)`
  → `Run(ctx, prompt)`) — an internal router agent (memory- and
  tools-free, matching Python's `Agent(memory=False, tools=None)`) picks
  the best-fit specialist by name via one LLM call; the reply is matched
  case-insensitively with trailing punctuation stripped. See
  `examples/triage_agent`.

## `internal/reliability` — retry helper

```go
type Options struct {
    Retries   int           // extra attempts after the first; default 2 (3 total)
    BaseDelay time.Duration // exponential backoff base; default 500ms
    Timeout   time.Duration // per-attempt timeout; default 60s
    OnRetry   func(attempt int, err error)
}
func Retry[T any](ctx context.Context, opts Options, fn func(ctx context.Context) (T, error)) (T, error)
```

Backoff is `BaseDelay * 2^attempt` between attempts. Unlike Python's
`with_retry`, which takes a coroutine *factory* because a coroutine object
can only be awaited once, `fn` here is an ordinary Go function that can
simply be called again — no factory indirection needed.

`Agent.retryOptions()` derives `Options` from
`config.Settings.SimonMaxRetries` / `SimonRetryBaseDelay` /
`SimonRequestTimeout`.

## `pkg/simonerr` — the error hierarchy

The one exported package. Python's `simon/exceptions.py` uses multiple
inheritance so callers can catch by domain (`ProviderError`) or by stdlib
convention (`RuntimeError`) on the same exception. Go has no inheritance,
so `simonerr.Error` implements `Unwrap() []error` (Go 1.20+), exposing both
a domain sentinel and a stdlib-convention sentinel plus the wrapped cause —
`errors.Is` matches any of them:

| Constructor | Domain sentinel | Stdlib-convention sentinel | Mirrors |
|---|---|---|---|
| `NewProviderError(msg, cause)` | `ErrProvider` | `ErrRuntime` | `ProviderError(SimonError, RuntimeError)` |
| `NewToolError(msg, cause)` | `ErrTool` | `ErrValue` | `ToolError(SimonError, ValueError)` |
| `NewKnowledgeError(msg, cause)` | `ErrKnowledge` | `ErrRuntime` | `KnowledgeError(SimonError, RuntimeError)` |
| `NewPermissionDeniedError(msg)` | `ErrPermission` | `ErrPermOS` | `PermissionDeniedError(SimonError, PermissionError)` |

`StructuredOutputError{Msg, RawText, Attempts}` is a distinct concrete type
(not built via `newError`) so callers can `errors.As` it to recover the raw
model output and attempt count.

## `internal/config` — settings

`config.Load()` reads a `.env` file in the working directory (values never
override real environment variables already set) plus the process
environment into a flat `Settings` struct. See
[configuration.md](configuration.md) for the full variable list and
defaults.
