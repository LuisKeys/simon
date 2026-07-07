# Examples

`examples/` holds ~15 small runnable programs, each demonstrating one
feature. Every example except `tool_runner_example` (which uses
`model.EchoModel` deterministically, no network) and, partially,
`activity_pipeline_example` (degrades gracefully without a local Ollama
server) needs at least one configured provider — an API key or a running
Ollama instance — via `.env` in the repo root (see
[configuration.md](configuration.md)). Run each with:

```
go run ./examples/<name>
```

from the repo root (several examples reference relative paths or spawn
subprocesses assuming that working directory).

| Example | Demonstrates | Key packages | Prerequisites / notes |
|---|---|---|---|
| `basic_agent` | Minimal possible agent: defaults only, single `Run` call. | `agent`, `config` | LLM credentials. |
| `builtin_tools_agent` | Manually-declared tools (`datetime_now`, `fs_list`, `fs_read`, `fs_write`, `shell_run`, a stubbed `web_search`) invoked via the `tool:name {json_args}` shorthand — Go can't introspect function param names/docstrings like Python's `@tool` decorator, so tools are explicit structs. | `agent`, `config`, `tool` | LLM credentials; writes `/tmp/simon_test.txt`; shells out via `os/exec`. |
| `chat_tui` | Interactive TUI chat with a named, personality-driven agent ("Luke", an expert chef), memory enabled. | `agent`, `config`, `memory`, `tui` | LLM credentials; interactive terminal. |
| `hooks_agent` | Observability via `agent.WithOnEvent`, handling all four event types (`model_selected`, `tool_called`, `retry_attempted`, `response_received`); prints total usage after the run. | `agent`, `config` | LLM credentials. |
| `memory_agent` | In-memory conversation recall across two sequential `Run` calls. | `agent`, `config`, `memory` | LLM credentials. |
| `persistent_memory_agent` | Cross-process persistent memory via `memory.NewJSONFile`. Run it twice to see recall survive a process restart. | `agent`, `config`, `memory` | LLM credentials; writes `.simon_chats/robotics_chat.json`. |
| `structured_output_agent` | Typed structured output via `agent.RunStructured[Recipe]` (Go generics) — the analogue of a Pydantic model. | `agent`, `config` | LLM credentials; provider must support JSON/structured output. |
| `run_context_example` | Go-idiomatic replacement for Python's `contextvars`-based per-request tool state: `internal/tool.RunToolCall` always calls tools with `context.Background()`, so context values can't reach tools. Uses per-request closures (`buildTools(user, tier)`) + a fresh `Agent` per request/goroutine instead, shown sequentially and concurrently. | `agent`, `config`, `tool` | LLM credentials. |
| `tool_runner_example` | The standalone turn-by-turn loop: `tool.NewRunner`, `tool.Turns` (an `iter.Seq2` iterator), `UntilDone`, plus manual intervention (`GenerateToolCallResponse`, `AppendMessages`) between turns. Uses `model.EchoModel{}` explicitly. | `model`, `tool` | **None** — no API key or network needed. |
| `parallel_agents` | Homogeneous parallel agents via `multi.NewGroup(...).RunAll` — three identically-configured agents (analyst/critic/summarizer) over the *same* prompt. | `agent`, `config`, `multi` | LLM credentials. |
| `agent_pool_example` | Heterogeneous parallel agents via `multi.NewPool` — three specialized agents each with its own prompt; wall-clock = slowest agent. Contrast with `parallel_agents`. | `agent`, `config`, `multi` | LLM credentials. |
| `triage_agent` | Routing tasks to specialist agents via `multi.NewTriage` — an internal router agent picks which of code/math/writing agents handles each of 3 sample tasks. | `agent`, `config`, `multi` | LLM credentials (one extra call for the routing decision itself). |
| `knowledge_agent` | RAG: indexes a PDF (`examples/knowledge_agent/docs/attention_paper.pdf`) into a knowledge base, then asks the agent 5 questions answerable only from that document. | `agent`, `config`, `knowledge`, `knowledge/embed` | LLM + embedding credentials (default provider `OPENAI`); creates/reuses a `.simon_knowledge` store; must run from repo root so the PDF's relative path resolves. |
| `planner_agent` | Goal decomposition + sequential execution via `internal/planner`, printing a live checklist on every status transition (`planner.WithOnUpdate`). | `agent`, `config`, `planner` | LLM credentials (one call to plan, one per task). |
| `mcp_agent` | Consuming tools from an external MCP server inside an agent (`internal/mcp.New(...).Tools(ctx)`); chains two tool calls (`add_numbers` then `reverse_string`). | `agent`, `config`, `mcp` | LLM credentials; launches `go run ./examples/mcp_agent/server` as a subprocess — `go` must be on `PATH`, run from repo root. |
| `mcp_agent/server` | Standalone MCP stdio server (companion to the above) exposing `add_numbers`/`reverse_string` via `sdk.AddTool` + `sdk.StdioTransport`. | `github.com/modelcontextprotocol/go-sdk/mcp` | None — pure stdio server, no LLM calls; not meant to be run standalone. |
| `activity_pipeline_example` | The full local-first activity pipeline: `privacy.Manager` (deny-by-default) → `events.EventBus` (SQLite-backed) → `semantic.Extractor` → `events.EventCompressor` (sessions) → `activity.GraphBuilder`/`ContextEngine` → `habits.DiscoveryEngine`, seeded with 5 days of synthetic history to trigger pattern detection. | `activity`, `events`, `habits`, `privacy`, `semantic` | None strictly required — degrades gracefully if no local Ollama server is running (semantic classification unavailable, rest of the pipeline still runs on synthetic data). Uses a throwaway SQLite file in a temp dir. Live macOS window-focus sensing is skipped entirely (no macOS `Sensor` implementation exists in this port — see [activity-pipeline.md](activity-pipeline.md)). |

## Notable cross-cutting points

- Virtually every example calls `config.Load()`, which reads `.env` from
  the working directory — run examples from the repo root.
- Two examples' own file-level doc comments call out intentional
  deviations from the Python original: `builtin_tools_agent` (no built-in
  tool library/introspection in Go) and `run_context_example` (no
  context-var equivalent, replaced by closures + goroutines).
- `tool_runner_example` and `activity_pipeline_example` are the only two
  designed to run with zero external LLM credentials by default.
