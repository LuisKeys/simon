package tool

import (
	"context"
	"iter"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
	"simon-go/internal/reliability"
)

// Runner drives Simon's standalone, composable tool-use loop, mirroring
// Python's simon/tools/runner.py ToolRunner. Where the Agent bakes the
// ReAct loop into one call, Runner exposes it turn by turn: drive
// model.Complete + tool execution one step at a time, inspect/intervene
// between turns, or just call UntilDone for the final answer.
//
// Python exposes this as a dual sync (__iter__/__next__, its own event
// loop) / async (__aiter__/__anext__) iterator because asyncio has no
// synchronous concurrency primitive. Go has no such duality — everything is
// synchronous by default — so Turns uses a single range-over-func iterator
// (iter.Seq2) instead of two parallel protocols.
type Runner struct {
	model         model.Model
	registry      *Registry
	specs         []model.ToolSpec
	messages      []model.Message
	maxIterations int
	retryOpts     reliability.Options

	totalUsage   response.Usage
	lastResponse *response.AgentResponse
	iterations   int
	started      bool
	done         bool
	tookOver     bool
}

// RunnerOption configures a Runner at construction time.
type RunnerOption func(*Runner)

func WithRunnerTools(tools ...Tool) RunnerOption {
	return func(r *Runner) {
		r.registry = NewRegistry(tools...)
		r.specs = r.registry.Specs()
	}
}

func WithRunnerMessages(msgs ...model.Message) RunnerOption {
	return func(r *Runner) { r.messages = append(r.messages, msgs...) }
}

func WithRunnerSystemPrompt(prompt string) RunnerOption {
	return func(r *Runner) {
		if prompt != "" {
			r.messages = append([]model.Message{{Role: model.RoleSystem, Content: prompt}}, r.messages...)
		}
	}
}

func WithMaxIterations(n int) RunnerOption { return func(r *Runner) { r.maxIterations = n } }

func WithRetryOptions(opts reliability.Options) RunnerOption {
	return func(r *Runner) { r.retryOpts = opts }
}

// NewRunner builds a Runner that drives m directly. Unlike Python's
// tool_runner(model=None), which can resolve a provider via ModelRouter
// itself, Runner always takes a concrete model.Model: provider selection
// stays a single responsibility of internal/agent, so this package doesn't
// duplicate that logic.
func NewRunner(m model.Model, opts ...RunnerOption) *Runner {
	r := &Runner{
		model:         m,
		registry:      NewRegistry(),
		maxIterations: 10,
		retryOpts:     reliability.DefaultOptions(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// LastResponse is the most recent turn's response, or nil before the first turn.
func (r *Runner) LastResponse() *response.AgentResponse { return r.lastResponse }

// TotalUsage accumulates Usage across every turn so far.
func (r *Runner) TotalUsage() response.Usage { return r.totalUsage }

// Messages returns the runner's current mutable message history.
func (r *Runner) Messages() []model.Message { return r.messages }

// ToolResult pairs a tool's textual output with whether running it failed,
// the Go analogue of Python's `{"role": "tool", ..., "is_error": bool}`.
type ToolResult struct {
	Message model.Message
	IsError bool
}

// GenerateToolCallResponse computes tool results for the current turn
// without appending them, returning ok=false if the last response requested
// no tools. Use this to inspect or log results before deciding how to
// continue.
func (r *Runner) GenerateToolCallResponse() ([]ToolResult, bool) {
	if r.lastResponse == nil || len(r.lastResponse.ToolCalls) == 0 {
		return nil, false
	}
	return r.toolResults(*r.lastResponse), true
}

// AppendMessages takes over the history: appends msgs and skips the
// runner's automatic append for this turn (mirrors the Anthropic SDK's
// take-over pattern).
func (r *Runner) AppendMessages(msgs ...model.Message) {
	r.messages = append(r.messages, msgs...)
	r.tookOver = true
}

func (r *Runner) toolResults(resp response.AgentResponse) []ToolResult {
	results := make([]ToolResult, 0, len(resp.ToolCalls))
	for _, call := range resp.ToolCalls {
		content, isError := RunToolCall(r.registry, call)
		results = append(results, ToolResult{
			Message: model.Message{Role: model.RoleTool, Content: content, ToolCallID: call.ID},
			IsError: isError,
		})
	}
	return results
}

func (r *Runner) autoAppend(resp response.AgentResponse) {
	r.messages = append(r.messages, model.Message{
		Role: model.RoleAssistant, Content: resp.Text, ToolCalls: resp.ToolCalls,
	})
	for _, result := range r.toolResults(resp) {
		r.messages = append(r.messages, result.Message)
	}
}

func (r *Runner) step(ctx context.Context) (response.AgentResponse, error) {
	resp, err := reliability.Retry(ctx, r.retryOpts, func(ctx context.Context) (response.AgentResponse, error) {
		return r.model.Complete(ctx, r.messages, r.specs)
	})
	if err != nil {
		return response.AgentResponse{}, err
	}
	if resp.Usage != nil {
		r.totalUsage = r.totalUsage.Add(*resp.Usage)
	}
	return resp, nil
}

// next runs one turn, mirroring __anext__: the first call always runs; each
// subsequent call auto-appends the previous turn's assistant/tool messages
// (unless AppendMessages already took over) and stops (more=false) once
// the model stops requesting tools or maxIterations is hit.
func (r *Runner) next(ctx context.Context) (resp response.AgentResponse, more bool, err error) {
	if r.done {
		return response.AgentResponse{}, false, nil
	}

	if !r.started {
		r.started = true
		r.tookOver = false
		resp, err = r.step(ctx)
		if err != nil {
			return response.AgentResponse{}, false, err
		}
		r.lastResponse = &resp
		return resp, true, nil
	}

	prev := r.lastResponse
	if prev == nil || len(prev.ToolCalls) == 0 || r.iterations >= r.maxIterations {
		r.done = true
		return response.AgentResponse{}, false, nil
	}

	if !r.tookOver {
		r.autoAppend(*prev)
	}
	r.iterations++
	r.tookOver = false

	resp, err = r.step(ctx)
	if err != nil {
		return response.AgentResponse{}, false, err
	}
	r.lastResponse = &resp
	return resp, true, nil
}

// Turns iterates one AgentResponse per turn, auto-executing any requested
// tools and feeding results back before the next turn — the single
// synchronous replacement for Python's dual __iter__/__aiter__ protocols.
func (r *Runner) Turns(ctx context.Context) iter.Seq2[*response.AgentResponse, error] {
	return func(yield func(*response.AgentResponse, error) bool) {
		for {
			resp, more, err := r.next(ctx)
			if err != nil {
				yield(nil, err)
				return
			}
			if !more {
				return
			}
			if !yield(&resp, nil) {
				return
			}
		}
	}
}

// UntilDone drives the loop to completion and returns the final response,
// mirroring until_done/until_done_async.
func (r *Runner) UntilDone(ctx context.Context) (response.AgentResponse, error) {
	var last response.AgentResponse
	for resp, err := range r.Turns(ctx) {
		if err != nil {
			return response.AgentResponse{}, err
		}
		last = *resp
	}
	return last, nil
}
