# Configuration

`internal/config.Load()` first loads any `KEY=VALUE` pairs from a `.env`
file in the working directory — blank lines and `#`-comments skipped,
values may be quoted, and a key already present in the real environment is
never overwritten by the file — then reads the process environment into a
flat `Settings` struct. Copy `.env.example` to `.env` to get started.

| Variable | Field | Default | Notes |
|---|---|---|---|
| `OPENAI_API_KEY` | `OpenAIAPIKey` | `""` | |
| `OPENAI_MODEL` | `OpenAIModel` | `""` (router default `"gpt-5"` when selected) | |
| `ANTHROPIC_API_KEY` | `AnthropicAPIKey` | `""` | |
| `ANTHROPIC_MODEL` | `AnthropicModel` | `""` (router default `"claude-3-5-sonnet-latest"`) | |
| `OLLAMA_HOST` | `OllamaHost` | `http://localhost:11434` | The **raw** env var (not this defaulted field) is what `router.hasOllama` checks, so Ollama only counts as "configured" if you set this explicitly. |
| `OLLAMA_MODEL` | `OllamaModel` | `""` (router default `"llama3.1"`) | |
| `DEFAULT_MODEL` | `DefaultModel` | `auto` | `auto`, or an explicit label like `openai_model` / `anthropic_model` / `ollama_model` — see [agent-core.md](agent-core.md#internal-router--providermodel-selection). |
| `KNOWLEDGE_STORE_PATH` | `KnowledgeStorePath` | `""` (CLI falls back to `.simon_knowledge`) | |
| `EMBEDDING_PROVIDER` | `EmbeddingProvider` | `OPENAI` | `OPENAI` \| `OLLAMA` \| `ANTHROPIC` (→ Voyage) |
| `EMBEDDING_MODEL` | `EmbeddingModel` | `text-embedding-3-small` | |
| `ENABLE_DIR_DOCUMENTS` | `EnableDirDocuments` | `true` | |
| `ENABLE_DIR_DOWNLOADS` | `EnableDirDownloads` | `true` | |
| `ENABLE_DIR_PICTURES` | `EnableDirPictures` | `false` | |
| `ENABLE_DIR_DESKTOP` | `EnableDirDesktop` | `false` | |
| `SIMON_LOGGING_ENABLED` | `SimonLoggingEnabled` | `false` | |
| `SIMON_LOG_LEVEL` | `SimonLogLevel` | `INFO` | `DEBUG` \| `INFO` \| `WARNING` |
| `SIMON_LOG_DIR` | `SimonLogDir` | `logs` | |
| `SIMON_MAX_RETRIES` | `SimonMaxRetries` | `2` | extra attempts after the first; feeds `reliability.Options.Retries` |
| `SIMON_REQUEST_TIMEOUT` | `SimonRequestTimeout` | `60.0` (seconds) | per-attempt timeout |
| `SIMON_RETRY_BASE_DELAY` | `SimonRetryBaseDelay` | `0.5` (seconds) | exponential backoff base |
| `SIMON_STRUCTURED_RETRIES` | `SimonStructuredRetries` | `2` | extra attempts for `agent.RunStructured` schema-validation retries |
| `SIMON_ACTIVITY_STORE_PATH` | `SimonActivityStorePath` | `.simon_activity/activity.db` | SQLite file for the activity pipeline |
| `SIMON_SENSOR_POLL_INTERVAL` | `SimonSensorPollInterval` | `1.0` (seconds) | |

Booleans are parsed with `strconv.ParseBool` (`true`/`false`/`1`/`0`/etc.);
an unparseable or empty value silently falls back to the default rather
than erroring. Ints/floats behave the same way via `strconv.Atoi` /
`strconv.ParseFloat`.

## Other on-disk locations (not env-configurable)

| Path | Written by | Purpose |
|---|---|---|
| `.simon_chats/<name>.json` | `memory.JSONFile` | one JSON file per persistent conversation |
| `.simon_knowledge/` (or `KNOWLEDGE_STORE_PATH`) | `internal/knowledge/index` | `<key>.sidx`, `<key>.meta.json`, `manifest.json` per indexed source |
