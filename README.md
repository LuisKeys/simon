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
