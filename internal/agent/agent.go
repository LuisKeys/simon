// Package agent implements Simon's ReAct loop, mirroring Python's
// simon/agent/agent.py Agent. Unlike Python (which offers both a sync
// run() wrapper juggling asyncio.get_running_loop and an async
// run_async()), Run is the single, ordinary synchronous entry point:
// callers who want concurrency use `go` or errgroup explicitly.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"simon-go/internal/agent/response"
	"simon-go/internal/config"
	"simon-go/internal/memory"
	"simon-go/internal/model"
	"simon-go/internal/model/anthropic"
	"simon-go/internal/model/ollama"
	"simon-go/internal/model/openai"
	"simon-go/internal/reliability"
	"simon-go/internal/router"
	"simon-go/internal/tool"
	"simon-go/pkg/simonerr"
)

// Event is emitted through OnEvent at points matching Python's Agent._emit
// calls (model_selected, tool_called, retry_attempted, response_received).
type Event struct {
	Type string
	Data map[string]any
}

// KnowledgeSearcher is the knowledge-base contract Agent needs, matching
// the shape of *knowledge.KnowledgeBase.Search. Declared here (rather than
// importing internal/knowledge directly) so this package doesn't pull in
// knowledge's embedding/index/extraction dependencies just to support the
// optional knowledge-context feature — mirrors Python's Agent(knowledge=...)
// being optional too.
type KnowledgeSearcher interface {
	Search(ctx context.Context, query string, topK int) ([]response.KnowledgeHit, error)
}

// Agent orchestrates memory, tools, and a model provider around the ReAct
// loop (model -> tool_calls -> execute -> feed back, up to MaxSteps).
type Agent struct {
	Name         string
	SystemPrompt string
	MaxSteps     int
	TotalUsage   response.Usage

	settings  config.Settings
	router    *router.Router
	modelName string
	memory    memory.Memory
	tools     *tool.Registry
	onEvent   func(Event)
	knowledge KnowledgeSearcher
	// modelOverride bypasses router-based resolution, primarily for tests
	// that need to script a model.Model's replies deterministically.
	modelOverride model.Model
}

// Option configures an Agent at construction time.
type Option func(*Agent)

func WithMemory(m memory.Memory) Option { return func(a *Agent) { a.memory = m } }

func WithTools(tools ...tool.Tool) Option {
	return func(a *Agent) { a.tools = tool.NewRegistry(tools...) }
}

func WithSystemPrompt(prompt string) Option { return func(a *Agent) { a.SystemPrompt = prompt } }

func WithMaxSteps(n int) Option { return func(a *Agent) { a.MaxSteps = n } }

// WithModel pins the provider/model label (e.g. "openai_model"), the same
// value Python's Agent(model=...) forwards to ModelRouter.resolve.
func WithModel(name string) Option { return func(a *Agent) { a.modelName = name } }

func WithName(name string) Option { return func(a *Agent) { a.Name = name } }

func WithOnEvent(fn func(Event)) Option { return func(a *Agent) { a.onEvent = fn } }

// WithModelOverride bypasses router-based provider selection entirely,
// always using m. Intended for tests that need a scripted model.Model.
func WithModelOverride(m model.Model) Option { return func(a *Agent) { a.modelOverride = m } }

// WithKnowledge attaches a knowledge base so Run/RunStructured inject
// relevant context as a system message, mirroring Python's
// Agent(knowledge=True) default. Knowledge is opt-in here rather than
// on-by-default: internal/knowledge is a heavier dependency (embeddings,
// on-disk index) that callers who don't need it shouldn't have to pull in.
func WithKnowledge(k KnowledgeSearcher) Option { return func(a *Agent) { a.knowledge = k } }

// New builds an Agent bound to settings (used to resolve a model provider
// and retry/timeout knobs). Knowledge-base integration (Python's
// knowledge=True default) is deferred to Phase 2; Agents built here behave
// as if knowledge=False until that lands.
func New(settings config.Settings, opts ...Option) *Agent {
	a := &Agent{
		Name:     "Simon",
		MaxSteps: 6,
		settings: settings,
		router:   router.New(settings),
		tools:    tool.NewRegistry(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *Agent) emit(eventType string, data map[string]any) {
	if a.onEvent == nil {
		return
	}
	a.onEvent(Event{Type: eventType, Data: data})
}

func (a *Agent) trackUsage(resp response.AgentResponse) {
	if resp.Usage != nil {
		a.TotalUsage = a.TotalUsage.Add(*resp.Usage)
	}
}

func (a *Agent) retryOptions() reliability.Options {
	return reliability.Options{
		Retries:   a.settings.SimonMaxRetries,
		BaseDelay: secondsToDuration(a.settings.SimonRetryBaseDelay),
		Timeout:   secondsToDuration(a.settings.SimonRequestTimeout),
	}
}

// Run executes a single prompt through the ReAct loop: resolve a model,
// seed the conversation from memory (if configured), run tool calls the
// model requests until it stops requesting them or MaxSteps is hit, and
// persist the final answer back to memory.
func (a *Agent) Run(ctx context.Context, prompt string) (response.AgentResponse, error) {
	m, err := a.resolveModel(prompt)
	if err != nil {
		return response.AgentResponse{}, err
	}

	messages, err := a.seedMessages(ctx, prompt)
	if err != nil {
		return response.AgentResponse{}, err
	}

	toolResult, handled, err := a.maybeCallToolShorthand(prompt)
	if err != nil {
		return response.AgentResponse{}, err
	}
	if handled {
		if a.memory != nil {
			if err := a.memory.Add(ctx, "assistant", toolResult); err != nil {
				return response.AgentResponse{}, err
			}
		}
		return response.AgentResponse{Text: toolResult}, nil
	}

	knowledgeCtx, err := a.knowledgeContext(ctx, prompt)
	if err != nil {
		return response.AgentResponse{}, err
	}
	if knowledgeCtx != "" {
		messages = append(messages, model.Message{Role: model.RoleSystem, Content: knowledgeCtx})
	}

	specs := a.tools.Specs()
	resp, err := a.complete(ctx, m, messages, specs)
	if err != nil {
		return response.AgentResponse{}, err
	}

	step := 0
	for len(resp.ToolCalls) > 0 && step < a.MaxSteps {
		step++
		messages = append(messages, model.Message{
			Role: model.RoleAssistant, Content: resp.Text, ToolCalls: resp.ToolCalls,
		})
		for _, call := range resp.ToolCalls {
			result, _ := tool.RunToolCall(a.tools, call)
			a.emit("tool_called", map[string]any{"tool": call.Name, "arguments": call.Arguments, "result": truncate(result, 200)})
			messages = append(messages, model.Message{Role: model.RoleTool, Content: result, ToolCallID: call.ID})
		}
		resp, err = a.complete(ctx, m, messages, specs)
		if err != nil {
			return response.AgentResponse{}, err
		}
	}

	if a.memory != nil {
		if err := a.memory.Add(ctx, "assistant", resp.Text); err != nil {
			return response.AgentResponse{}, err
		}
	}

	a.emit("response_received", map[string]any{"usage": resp.Usage, "steps": step})
	return resp, nil
}

// complete runs a single model.Complete call with the agent's retry policy,
// tracking usage and firing retry_attempted events, mirroring Python's
// `with_retry(lambda: model.complete(...), ...)` call sites.
func (a *Agent) complete(ctx context.Context, m model.Model, messages []model.Message, specs []model.ToolSpec) (response.AgentResponse, error) {
	opts := a.retryOptions()
	opts.OnRetry = func(attempt int, err error) {
		a.emit("retry_attempted", map[string]any{"attempt": attempt, "error": err.Error()})
	}
	resp, err := reliability.Retry(ctx, opts, func(ctx context.Context) (response.AgentResponse, error) {
		return m.Complete(ctx, messages, specs)
	})
	if err != nil {
		return response.AgentResponse{}, err
	}
	a.trackUsage(resp)
	return resp, nil
}

func (a *Agent) seedMessages(ctx context.Context, prompt string) ([]model.Message, error) {
	var messages []model.Message

	if a.memory != nil {
		if err := a.memory.Add(ctx, "user", prompt); err != nil {
			return nil, err
		}
		history, err := a.memory.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, msg := range history {
			messages = append(messages, model.Message{Role: model.Role(msg.Role), Content: msg.Content})
		}
	} else {
		messages = append(messages, model.Message{Role: model.RoleUser, Content: prompt})
	}

	if a.SystemPrompt != "" {
		messages = append([]model.Message{{Role: model.RoleSystem, Content: a.SystemPrompt}}, messages...)
	}

	return messages, nil
}

// maybeCallToolShorthand supports Python's tiny explicit tool-call format:
// "tool:name {json_args}". Returns handled=false when the prompt doesn't
// use the shorthand, so the caller falls through to the normal ReAct loop.
func (a *Agent) maybeCallToolShorthand(prompt string) (result string, handled bool, err error) {
	text := strings.TrimSpace(prompt)
	if !strings.HasPrefix(text, "tool:") {
		return "", false, nil
	}
	remainder := strings.TrimSpace(text[len("tool:"):])

	var name string
	args := map[string]any{}
	if idx := strings.IndexByte(remainder, ' '); idx != -1 {
		name = remainder[:idx]
		if err := json.Unmarshal([]byte(remainder[idx+1:]), &args); err != nil {
			return "", true, simonerr.NewToolError("tool arguments must be a JSON object", err)
		}
	} else {
		name = remainder
	}

	t, ok := a.tools.Get(name)
	if !ok {
		return fmt.Sprintf("Tool '%s' not found. Available: %s", name, strings.Join(a.tools.Names(), ", ")), true, nil
	}

	argsJSON, _ := json.Marshal(args)
	out, err := t.Call(context.Background(), argsJSON)
	if err != nil {
		return "", true, err
	}
	return out, true, nil
}

// resolveModel picks a provider/model via the router and builds the live
// model.Model client, mirroring ModelRouter.resolve + the provider
// constructors Python calls inline.
func (a *Agent) resolveModel(prompt string) (model.Model, error) {
	if a.modelOverride != nil {
		return a.modelOverride, nil
	}
	choice := a.router.Resolve(router.ResolveOptions{Model: a.modelName, Task: prompt})
	a.emit("model_selected", map[string]any{"provider": string(choice.Provider), "model": choice.Model})

	switch choice.Provider {
	case router.ProviderOpenAI:
		return openai.New(a.settings.OpenAIAPIKey, choice.Model), nil
	case router.ProviderAnthropic:
		return anthropic.New(a.settings.AnthropicAPIKey, choice.Model), nil
	case router.ProviderOllama:
		return ollama.New(a.settings.OllamaHost, choice.Model)
	default:
		return model.EchoModel{}, nil
	}
}

// knowledgeContext searches the attached knowledge base (if any) for
// prompt, returning a "Relevant knowledge:" system-message body, or "" if
// no knowledge base is attached or nothing relevant was found — mirroring
// Python's Agent._knowledge_context (top_k=2).
func (a *Agent) knowledgeContext(ctx context.Context, prompt string) (string, error) {
	if a.knowledge == nil {
		return "", nil
	}
	hits, err := a.knowledge.Search(ctx, prompt, 2)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("Relevant knowledge:")
	for _, hit := range hits {
		fmt.Fprintf(&b, "\n- (%s) %s", hit.Source, hit.Text)
	}
	return b.String(), nil
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
