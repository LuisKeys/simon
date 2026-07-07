# simon-go

Go port of [Simon SDK](https://github.com/<org>/simon_sdk), a lightweight AI agent framework.

See the migration plan for scope, phased delivery, and design decisions for the
hard-to-port Python idioms (reflection-based tool schemas, contextvars, dual
sync/async APIs, dual-inheritance exceptions, pickle/numpy knowledge index).

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
