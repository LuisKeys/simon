# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

simon-go is a Go port of "Simon SDK," a lightweight AI agent framework originally written in Python. There is no separate migration-plan doc committed to the repo — design rationale for hard-to-port Python idioms lives inline in package doc comments (the comment directly above `package xxx` in the relevant file). Read those before assuming Python and Go behave the same way.

## Commands

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test -run TestName ./internal/pkg/...`
- Race-sensitive pipeline test: `go test -race ./internal/pipeline/...`
- Vet: `go vet ./...` (no golangci-lint or other configured linter; no Makefile, no CI workflow, no `go:generate` directives)
- Run the CLI: `go run ./cmd/simon chat|ask|index|plan`

## Architecture

Two-tier package layout: `pkg/` holds the one public package (`simonerr`); everything else is `internal/`.

**Agent core** (`internal/agent` → `internal/model` → `internal/tool`):
- `agent.Agent.Run(ctx, prompt)` runs the ReAct loop: seed messages, call `model.Model.Complete` with `[]tool.ToolSpec`, execute returned tool calls, loop. Knowledge-base context is injected via a `KnowledgeSearcher` interface rather than importing `internal/knowledge` directly, to avoid a cycle.
- `internal/model` defines the `Model`/`Message`/`ToolSpec`/`Role` types only; SDK-specific code lives in subpackages `model/openai`, `model/anthropic`, `model/ollama` so the base package has no third-party SDK dependency.
- `internal/router.Resolve` picks a provider+model name (`Choice`) but deliberately does not construct a `model.Model` itself — doing so would create a router→model→router import cycle.
- `internal/tool.New[P any](name, desc, fn)` builds a tool by reflecting a parameter struct's `json`/`jsonschema` tags — this replaced Python's `inspect.signature`-based `@tool` decorator, so tool params must be a struct, not arbitrary function args. `NewRaw` skips reflection; `NewRegistry` aggregates tools for the agent loop's dispatch.
- `pkg/simonerr` replaces Python's dual-inheritance exception hierarchy with sentinel errors plus a Go 1.20 multi-`Unwrap() []error`, so `errors.Is` can match either a "domain" or "stdlib" identity on the same error value.
- Python's dual `run()`/`run_async()` API collapsed to a single synchronous `Agent.Run`; callers get concurrency via `go`/goroutines, not a separate async method. `internal/multi` (AgentGroup/AgentPool/TriageAgent) likewise uses goroutines+WaitGroup instead of `asyncio.gather`.

**Knowledge base** (`internal/knowledge` + `knowledge/embed`, `knowledge/index`, `knowledge/extract`):
- `KnowledgeBase.Add` extracts text (pdf/docx/xlsx/pptx via `extract`), chunks it, embeds via `embed.Embedder` (OpenAI/Ollama/Voyage), and persists to a custom binary format (`index`, "SIDX") instead of Python's pickle+numpy. Layout is `<key>.sidx` (magic+version+dim+count+float32 vectors), `<key>.meta.json`, and a `manifest.json` used to detect embedding-model/dimension mismatches. There is no binary compatibility with a Python `.simon_knowledge/` directory.
- `Search` embeds the query, does a vector search, and returns `agent/response.KnowledgeHit` values.

**Activity pipeline** (`internal/events`, `privacy`, `activity`, `habits`, `semantic`, `sensors`, `pipeline`):
- Data flow: `sensors.Sensor.Poll` → `events.EventBus.Publish` → `internal/semantic` classifies the raw event into an activity label → `events.EventCompressor` compresses events into sessions → `internal/activity` (`ActivityStore` + transition graph) → `internal/habits.HabitDiscoveryEngine` mines n-gram patterns over session history. `internal/privacy.PermissionManager` is deny-by-default, gates sensor access, and audits its own decisions back through the event bus.
- `internal/semantic` deliberately bypasses `internal/router` and is hardcoded to a local Ollama instance — this is a privacy boundary (window titles/clipboard content must never leave the device, regardless of what API keys are configured), not an oversight.
- `sensors.Sensor` is intentionally not a `tool.Tool`: a sensor is a self-directed background poll loop, and routing it through the ReAct tool-call path would cost one LLM round-trip per tick.
- macOS sensor implementations (PyObjC in the Python original) are out of scope for this port; the intended design is a separate Swift satellite process emitting newline-delimited JSON, consumed through the same `Sensor` interface, specifically to avoid CGO breaking the no-CGO/cross-compile build.
- `internal/pipeline` has no production code — it exists solely to hold an end-to-end test wiring a synthetic sensor through the full chain (sensor → bus → semantic → activity → habits), verified clean under `-race`.

**Surface** (`internal/mcp`, `planner`, `tui`, `cmd/simon`):
- `internal/mcp` wraps an MCP stdio client's tools as `tool.Tool`, so MCP-provided tools are indistinguishable from local ones to the agent loop.
- `internal/planner` decomposes a goal via one LLM call, then runs each resulting task sequentially through its own `Agent.Run`.
- `internal/tui` uses `bufio.Scanner` line-based input rather than raw-mode (`termios`/`tty`) tab-completion — a deliberate simplification; tab-completion is the one feature knowingly dropped in the port.
- `cmd/simon` is the CLI entrypoint (stdlib `flag`, no third-party CLI framework), with subcommands `chat`/`ask`/`index`/`plan`.

`examples/` contains ~14 small runnable programs, each demonstrating one feature (memory, knowledge, MCP, planner, tool runner, parallel agents, etc.) — useful as working reference code when wiring up a new example or debugging an existing package's public API.
