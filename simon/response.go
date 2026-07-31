package simon

import (
	"simon-go/internal/agent/response"
)

// StopReason describes why a run stopped producing more tool calls.
type StopReason string

// Usage tracks token consumption for a Run.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// ToolCall is a single tool invocation the model requested during a run.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// Response is the result of Session.Run/RunStructured. It is a distinct
// type from internal/agent/response.AgentResponse (even though the shapes
// currently overlap) so internal refactors can't silently change the
// public contract.
type Response struct {
	Text       string
	Usage      Usage
	ToolCalls  []ToolCall
	Steps      int
	Model      string
	Provider   string
	StopReason StopReason
	Metadata   map[string]any
}

func fromAgentResponse(r response.AgentResponse, modelName, provider string, steps int) Response {
	var usage Usage
	if r.Usage != nil {
		usage = Usage{
			InputTokens:  r.Usage.InputTokens,
			OutputTokens: r.Usage.OutputTokens,
			TotalTokens:  r.Usage.TotalTokens,
		}
	}
	calls := make([]ToolCall, len(r.ToolCalls))
	for i, c := range r.ToolCalls {
		calls[i] = ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
	}
	return Response{
		Text:       r.Text,
		Usage:      usage,
		ToolCalls:  calls,
		Steps:      steps,
		Model:      modelName,
		Provider:   provider,
		StopReason: StopReason(r.StopReason),
	}
}
