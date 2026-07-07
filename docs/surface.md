# Surface: CLI, TUI, planner, MCP client

Packages: `cmd/simon`, `internal/tui`, `internal/planner`, `internal/mcp`.
This layer depends on the agent core (and, for `index`/`plan`, the
knowledge base); it has no dependency on the activity pipeline.

## `cmd/simon` — the CLI entrypoint

> Command simon is Simon SDK's command-line interface, mirroring Python's
> `simon/cli.py` (`chat | ask | index | plan`). It uses the standard
> library's `flag` package with manual subcommands rather than a
> third-party CLI framework — consistent with the small size of the
> original argparse setup (62 lines) and the project's
> minimal-dependencies philosophy.

```
go run ./cmd/simon chat
go run ./cmd/simon ask "<prompt>"
go run ./cmd/simon index <path>
go run ./cmd/simon plan "<goal>"
```

| Subcommand | Args | What it does |
|---|---|---|
| `chat` | none | Builds an `agent.Agent` with `WithMemory(memory.NewInMemory())`, runs `tui.Chat` against stdin/stdout. `clearScreen` is passed as `nil`, so `/clear` is a silent no-op in the CLI specifically. |
| `ask <prompt>` | 1 positional | Single agent, single `Run`, prints `resp.Text`. |
| `index <path>` | 1 positional | Builds an embedder via `embed.Default(settings)`, opens/creates a `knowledge.New` store at `settings.KnowledgeStorePath` (default `.simon_knowledge`), calls `kb.Add(ctx, path, force=false)`, prints the chunk count added. |
| `plan <goal>` | 1 positional | Builds an exec agent, `planner.New(settings, execAgent, planner.WithOnUpdate(...))` printing the checklist after every task-status update, then `p.Run(ctx, goal)`. |

Global flag `-m` / `--model <name>` ("Force a provider (e.g.
OPENAI_MODEL)") can appear before or after the subcommand name, mirroring
argparse's parent-parser flag placement; `splitCommand` manually skips
`-m`'s value argument so it isn't mistaken for the subcommand name.

All error paths print `simon: <err>` to stderr; exit code 1 for runtime
errors, 2 for usage errors (missing args / no command).

## `internal/mcp` — MCP tool bridging

> Package mcp connects to an MCP server over stdio and exposes its tools
> as Simon `tool.Tool` values, mirroring Python's
> `simon/tools/mcp_client.py MCPClient`.

```go
func New(command ...string) *Client   // command[0] = executable, rest = args
func (c *Client) Tools(ctx context.Context) ([]tool.Tool, error)
```

`Tools` opens a stdio session (`exec.Command` + the official MCP Go SDK's
`sdk.CommandTransport`, identifying itself as `{Name: "simon", Version:
"0.1.0"}`), calls `ListTools`, and wraps each `sdk.Tool` via `tool.NewRaw`
(since an MCP tool's JSON Schema is only known at runtime, not a static Go
type) with a `fn` that delegates to `Client.call`.

> **Important:** like Python's `MCPClient`, **each subsequent `Call` opens
> a fresh stdio connection** rather than keeping the one from `Tools`
> open — a new process + handshake per tool invocation. This trades
> performance for statelessness, deliberately matching the Python
> reference implementation.

Connection and tool-call failures wrap in `simonerr.NewProviderError`;
malformed arguments wrap in `simonerr.NewToolError`. See
`examples/mcp_agent` (client) and `examples/mcp_agent/server` (a
standalone companion MCP server exposing `add_numbers` and
`reverse_string`, launched by the client example via `go run
./examples/mcp_agent/server`).

## `internal/planner` — goal decomposition + sequential execution

> Package planner decomposes a goal into an ordered task list via an LLM
> call, then runs each task through an Agent, mirroring Python's
> `simon/planner/planner.py Planner`.

```go
type Status string // Pending "○" | InProgress "◐" | Done "✓"
type Task struct { Description string; Status Status; Result string }

type Planner struct { Agent, PlannerAgent *agent.Agent; OnUpdate func([]Task); Tasks []Task }
func New(settings config.Settings, execAgent *agent.Agent, opts ...Option) *Planner
func WithOnUpdate(fn func([]Task)) Option
func (p *Planner) Plan(ctx context.Context, goal string) ([]Task, error)
func (p *Planner) Run(ctx context.Context, goal string) ([]Task, error)
func RenderTasks(tasks []Task) string // "<icon> <description>" per line
```

`New` builds a dedicated `PlannerAgent` sharing `execAgent`'s model
(`agent.WithModel(execAgent.ModelName())`) but with no tools/memory,
pinned to a fixed system prompt instructing it to reply with **only** a
JSON array of task-description strings.

- **`Plan`** — runs the goal through `PlannerAgent`, then `parseTasks`:
  first tries to regex-extract a `[...]` JSON array and unmarshal it;
  falling back to splitting the raw text into lines and stripping
  leading bullets/numbering if that fails or yields nothing. Sets all
  tasks to `Pending` and emits an update.
- **`Run`** — calls `Plan`, then executes tasks **sequentially** (not in
  parallel — see `internal/multi` for that), marking each `InProgress`
  before running (`p.Agent.Run(ctx, task.Description)`) and `Done` after,
  emitting an update at each transition. Aborts remaining tasks on the
  first execution error, returning the partially-completed task list plus
  that error.

`WithOnUpdate` mirrors "Python's default of printing `render_tasks`" —
`cmd/simon`'s `plan` subcommand and `examples/planner_agent` both use it
to print a live checklist.

## `internal/tui` — chat loop and Markdown rendering

> Python hand-rolls raw terminal mode (`termios`/`tty`) for inline
> tab-complete hints on "/" commands, matching the project's "no external
> dependencies" educational style. This Go port keeps the same
> no-heavy-dependency spirit but reads lines with `bufio.Scanner` instead
> of raw mode: implementing character-by-character raw input handling
> (autocomplete hints redrawn per keystroke) is substantial,
> hard-to-test terminal-specific code for a cosmetic feature.
> `RenderMarkdown` and the command handling are ported faithfully; the
> tab-completion hint UI is the one deliberately dropped feature.

```go
func Chat(ctx context.Context, a *agent.Agent, in io.Reader, out io.Writer, clearScreen func()) error
func RenderMarkdown(text string) string
```

`Chat`'s loop: print a `[You] ` prompt, read one line, trim, skip empty
input. `/quit` (case-insensitive) prints "Bye!" and returns. `/clear`
calls `clearScreen` if non-nil, else is a silent no-op — this is why the
CLI's `chat` subcommand, which passes `nil`, makes `/clear` do nothing.
Any other input runs through `a.Run`; errors print in red and the loop
continues (it does not exit on a failed turn). Successful responses render
through `RenderMarkdown`, indent 2 spaces, print under a `[<AgentName>]`
header; if `resp.Usage != nil`, a dim `tokens — input: N output: N total:
N` line follows. EOF/scanner error also prints "Bye!" before returning.

`RenderMarkdown` converts a subset of Markdown to ANSI escapes, line by
line: fenced code blocks (dimmed, separated by a 40-char rule),
`#`/`##`/`###` headers (yellow-bold / bold / bold-dim), `-`/`*` bullets
normalized to `•`, `**bold**`/`__bold__`, `*italic*`/`_italic_` (dimmed),
and `` `code` `` (cyan) — no external terminal/markdown library.
