// Package model defines the public, stable model-provider contract
// consumers of the simon SDK implement to plug a custom LLM client into a
// Runtime. It intentionally does not alias internal/model's types: internal
// types are free to evolve (new fields, renamed internals) without breaking
// external implementations of this interface.
package model

import "context"

// Role identifies who authored a Message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// Message is the provider-agnostic chat message shape passed to Complete.
type Message struct {
	Role    Role
	Content string
	// ToolCalls is set on assistant messages that requested tool calls.
	ToolCalls []ToolCall
	// ToolCallID is set on RoleTool messages: which call this is the result of.
	ToolCallID string
}

// ToolSpec is the JSON-schema description of a callable tool.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// CompletionOptions carries per-request knobs. Empty for now — reserved so
// future options (temperature, max tokens, ...) don't require an interface
// change.
type CompletionOptions struct{}

// Request bundles everything a Model needs to produce a Response.
type Request struct {
	Messages []Message
	Tools    []ToolSpec
	Options  CompletionOptions
}

// Usage reports token accounting for a single Complete call.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Response is a model's reply to a Request.
type Response struct {
	Text       string
	Usage      Usage
	ToolCalls  []ToolCall
	StopReason string
}

// Model is the interface a custom provider implements to be usable via
// simon.WithModel. Implementations should be safe for concurrent use, since
// a Runtime may drive multiple Sessions against the same Model.
type Model interface {
	Complete(ctx context.Context, request Request) (Response, error)
}

// EchoModel is a network-free Model that replies with the last user
// message, prefixed to make it obvious no real provider was called. Useful
// for tests and examples that don't need a live API key.
type EchoModel struct{}

// Complete implements Model.
func (EchoModel) Complete(_ context.Context, req Request) (Response, error) {
	var lastUser string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == RoleUser {
			lastUser = req.Messages[i].Content
			break
		}
	}
	return Response{Text: "Simon (echo): " + lastUser}, nil
}
