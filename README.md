# simon-go

Go port of "Simon SDK," a lightweight AI agent framework originally
written in Python. There is no separate migration-plan document — design
rationale for hard-to-port Python idioms (reflection-based tool schemas,
`contextvars`, dual sync/async APIs, dual-inheritance exceptions,
pickle/numpy knowledge index) lives inline in package doc comments, and is
indexed in [docs/](docs/).

## Quick start

```
cp .env.example .env   # add an API key, or point OLLAMA_HOST at a local server
go build ./...
go run ./cmd/simon chat
```

See [docs/configuration.md](docs/configuration.md) for every environment
variable, and [docs/examples.md](docs/examples.md) for ~15 runnable
programs demonstrating individual features.

## Documentation

- [docs/architecture.md](docs/architecture.md) — layer map and cross-cutting design decisions (start here)
- [docs/agent-core.md](docs/agent-core.md) — the ReAct loop, model providers, tools, memory, multi-agent patterns
- [docs/knowledge-base.md](docs/knowledge-base.md) — document ingestion, embeddings, the SIDX binary index format
- [docs/activity-pipeline.md](docs/activity-pipeline.md) — sensors, events, privacy, semantic classification, habit mining
- [docs/surface.md](docs/surface.md) — the CLI, TUI, planner, and MCP client
- [docs/examples.md](docs/examples.md) — what each program under `examples/` demonstrates and how to run it
- [docs/configuration.md](docs/configuration.md) — every environment variable, with defaults

## Commands

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test -run TestName ./internal/pkg/...`
- Race-sensitive pipeline test: `go test -race ./internal/pipeline/...`
- Vet: `go vet ./...` (no golangci-lint or other configured linter; no Makefile, no CI workflow)
- Run the CLI: `go run ./cmd/simon chat|ask|index|plan`

## Status

Phase 0 (foundation) in progress: `pkg/simonerr`, `internal/config`,
`internal/reliability`, `internal/router`, `internal/agent/response`.

Phase 1 (core execution) complete: `internal/model` (+ openai/anthropic/ollama
providers), `internal/tool` (registration + ToolRunner), `internal/memory`,
`internal/agent` (ReAct loop + structured output), `internal/multi`
(Group/Pool/Triage).

Phase 2 (knowledge) complete: `internal/knowledge/embed` (OpenAI/Ollama/Voyage
embedding providers), `internal/knowledge/index` (from-scratch SIDX binary
format replacing Python's pickle+numpy), `internal/knowledge/extract`
(pdf/docx/xlsx/pptx text extraction), `internal/knowledge` (chunking +
KnowledgeBase), wired into `internal/agent` as an optional knowledge-context
system message via the KnowledgeSearcher interface.

Phase 3 (surface) complete: `internal/mcp` (official MCP Go SDK, stdio
client), `internal/planner` (goal decomposition + sequential execution),
`internal/tui` (terminal chat: Markdown→ANSI rendering + /quit /clear —
line-based input via bufio.Scanner rather than raw-mode tab-completion, a
deliberate simplification), `cmd/simon` (chat/ask/index/plan CLI via the
stdlib `flag` package). The binary builds and runs end-to-end against a
real local Ollama server.

Phase 4 (activity pipeline) complete: `internal/events` (EventBus pub/sub +
SQLite-backed Store + EventCompressor), `internal/privacy`
(deny-by-default PermissionManager, audited via the event bus),
`internal/activity` (ActivityStore query layer, ContextEngine, activity
transition graph), `internal/habits` (n-gram habit mining over session
history), `internal/semantic` (LLM-based activity classification, local
Ollama only by design), `internal/sensors` (Sensor/Manager lifecycle —
macOS sensors themselves are out of scope for this port; see package doc).
`internal/pipeline` has an end-to-end test wiring a synthetic sensor
through the full chain (sensor -> bus -> semantic -> session compression ->
activity store -> graph -> habit discovery), verified clean under `-race`.
