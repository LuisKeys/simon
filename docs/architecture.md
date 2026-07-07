# Architecture

simon-go is a Go port of "Simon SDK," a Python AI agent framework. It keeps
the original's behavior and package boundaries wherever Go allows, and
diverges deliberately (and documents the divergence inline) wherever a
Python idiom has no direct Go equivalent — reflection-based tool schemas,
`contextvars`, dual sync/async APIs, dual-inheritance exceptions, and a
pickle+numpy on-disk format all fall in that category.

The canonical source of truth for *why* a piece of code looks the way it
does is the package doc comment directly above `package xxx` in the
relevant file. This document is a map of those decisions plus the data flow
between packages; when the two disagree, trust the code comment and update
this doc.

## Two-tier layout

```
pkg/simonerr/       the one public package: the error hierarchy
internal/           everything else
```

Only `pkg/simonerr` is importable from outside the module. Every other
package lives under `internal/` because there is no stability contract for
the rest of the SDK yet.

## The four layers

```
┌─────────────────────────────────────────────────────────────────────┐
│ Surface: cmd/simon, internal/tui, internal/planner, internal/mcp    │
├─────────────────────────────────────────────────────────────────────┤
│ Agent core: internal/agent → internal/model → internal/tool         │
│             internal/multi, internal/memory, internal/router        │
├─────────────────────────────────────────────────────────────────────┤
│ Knowledge base: internal/knowledge (+ embed, index, extract)        │
├─────────────────────────────────────────────────────────────────────┤
│ Activity pipeline: sensors → events → privacy → semantic →          │
│                     activity → habits                               │
└─────────────────────────────────────────────────────────────────────┘
```

These layers are mostly independent of each other:

- The **surface** layer depends on the agent core and (for `index`/`plan`)
  the knowledge base. It has no dependency on the activity pipeline.
- The **agent core** depends on nothing above it. `internal/knowledge` is
  wired in only through the `agent.KnowledgeSearcher` interface — the
  agent package never imports `internal/knowledge` directly, to avoid
  pulling embeddings/index/extraction dependencies into every binary that
  just wants a bare agent.
- The **knowledge base** depends only on `internal/config`,
  `internal/agent/response`, and `pkg/simonerr`.
- The **activity pipeline** is entirely separate from the other three: it
  shares no runtime state with `internal/agent`, and its one LLM-touching
  package (`internal/semantic`) deliberately bypasses `internal/router` to
  enforce a privacy boundary (see [activity-pipeline.md](activity-pipeline.md)).

Detailed docs per layer:

- [agent-core.md](agent-core.md) — the ReAct loop, model providers, tools, memory, multi-agent patterns
- [knowledge-base.md](knowledge-base.md) — ingestion, embeddings, the SIDX binary index format
- [activity-pipeline.md](activity-pipeline.md) — sensors, events, privacy, semantic classification, habit mining
- [surface.md](surface.md) — the CLI, TUI, planner, and MCP client
- [examples.md](examples.md) — what each program under `examples/` demonstrates and how to run it
- [configuration.md](configuration.md) — every environment variable `internal/config` reads, with defaults

## Cross-cutting design decisions worth knowing up front

- **No async duality.** Python offers `run()`/`run_async()` pairs almost
  everywhere. Go collapses these to one synchronous method; callers get
  concurrency via goroutines (`internal/multi` uses `sync.WaitGroup`,
  `internal/tool.Runner.Turns` uses a `range`-over-func iterator instead of
  Python's `__iter__`/`__aiter__` pair).
- **No reflection-based tool schemas.** Compiled Go binaries don't retain
  parameter names, so `internal/tool.New[P any]` requires an explicit
  parameter struct instead of introspecting a function signature. This is
  an ergonomics change, not a workaround defect (see
  `internal/tool/tool.go`'s package doc).
- **Error identity via multi-unwrap.** `pkg/simonerr.Error` implements
  `Unwrap() []error` (Go 1.20+) so `errors.Is` can match either a domain
  sentinel (`ErrProvider`) or a stdlib-convention sentinel (`ErrRuntime`)
  on the same error value, replacing Python's dual-inheritance exception
  classes.
- **No binary compatibility with Python's on-disk formats.** The knowledge
  index (`internal/knowledge/index`, "SIDX") and any local databases are
  from-scratch Go formats. There is no migration path from an existing
  Python `.simon_knowledge/` directory.
- **Local-first privacy boundary.** `internal/semantic` always talks to a
  local Ollama model, regardless of what cloud provider keys are
  configured elsewhere, because window titles and clipboard metadata must
  never leave the device. `internal/privacy` is deny-by-default and audits
  every grant/revoke/denial back through the same event bus the rest of
  the pipeline reads.
- **Sensors are not tools.** A `tool.Tool` is a synchronous function an LLM
  decides to call; a `sensors.Sensor` is a self-directed background poll
  loop. Routing sensor polls through the ReAct loop would cost one LLM
  round-trip per tick — the opposite of what a sensor is for.
- **macOS sensors are out of scope.** The Python original uses PyObjC for
  window-focus/clipboard sensing; embedding CGO/Objective-C here would
  break simon-go's no-CGO, cross-compilable build. The intended design is
  a separate Swift satellite process emitting newline-delimited JSON,
  consumed through the same `sensors.Sensor` interface — deferred, not
  implemented.
- **Tab-completion is the one UI feature knowingly dropped.** `internal/tui`
  uses `bufio.Scanner` line-based input instead of raw terminal mode, so
  the Python TUI's inline "/"-command autocomplete hints have no Go
  equivalent. Markdown rendering and command handling are ported faithfully.

## Commands

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test -run TestName ./internal/pkg/...`
- Race-sensitive pipeline test: `go test -race ./internal/pipeline/...`
- Vet: `go vet ./...` (no linter, no Makefile, no CI workflow configured)
- Run the CLI: `go run ./cmd/simon chat|ask|index|plan`
