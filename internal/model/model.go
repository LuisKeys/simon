// Package model defines the Model interface adapters implement, mirroring
// Python's simon/models/base.py BaseModel. Unlike the Python ABC, providers
// with heavy SDK dependencies live in their own subpackages
// (internal/model/openai, .../anthropic, .../ollama) so importing this
// package alone pulls in no provider SDKs.
package model

import (
	"context"

	"simon-go/internal/agent/response"
)

// Role identifies who authored a Message, mirroring the "role" key of
// Python's generic {role, content} message dicts.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is Simon's provider-agnostic chat message shape. Provider
// implementations translate a []Message into their own wire format (see
// _to_openai_messages/_split_messages in the Python source).
type Message struct {
	Role    Role
	Content string
	// ToolCalls is set on assistant messages that requested tool calls.
	ToolCalls []response.ToolCall
	// ToolCallID is set on RoleTool messages: which call this is the result of.
	ToolCallID string
}

// ToolSpec is the JSON-schema description of a callable tool, passed to
// Complete so the model may request tool calls back via
// AgentResponse.ToolCalls.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Model is the interface every provider adapter implements (the Go analogue
// of Python's BaseModel ABC).
type Model interface {
	// Complete returns the model's reply. When tools are non-empty the model
	// may request tool calls by populating AgentResponse.ToolCalls; the
	// Agent's ReAct loop runs them and feeds results back. Providers that
	// cannot call tools simply leave ToolCalls empty, so the loop runs once.
	Complete(ctx context.Context, messages []Message, tools []ToolSpec) (response.AgentResponse, error)
}

// EchoModel is the safe, network-free default used when no provider is
// configured (also the deterministic backbone of Phase 1's tests).
type EchoModel struct{}

// Complete replies with the last user message, prefixed to make it obvious
// no real model was called.
func (EchoModel) Complete(_ context.Context, messages []Message, _ []ToolSpec) (response.AgentResponse, error) {
	var lastUser string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			lastUser = messages[i].Content
			break
		}
	}
	return response.AgentResponse{Text: "Simon (echo): " + lastUser}, nil
}
