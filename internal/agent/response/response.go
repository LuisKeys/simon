// Package response defines the shared Agent/ToolRunner/Multi/Logging result
// types, mirroring Python's simon/agent/response.py. This contract is
// frozen in Phase 0 because internal/agent, internal/tool, internal/multi,
// and internal/logging all depend on it.
package response

// Usage tracks token consumption for a single model call.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Add returns the element-wise sum of two Usage values, replacing Python's
// Usage.__add__ operator overload.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		InputTokens:  u.InputTokens + other.InputTokens,
		OutputTokens: u.OutputTokens + other.OutputTokens,
		TotalTokens:  u.TotalTokens + other.TotalTokens,
	}
}

// ToolCall is a tool invocation requested by the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

// AgentResponse is the result of a single Agent.Run/ToolRunner turn.
type AgentResponse struct {
	Text string
	// Usage is nil for local/free providers (Ollama, Echo), matching
	// Python's `usage: Usage | None`.
	Usage      *Usage
	ToolCalls  []ToolCall
	StopReason string
	// Parsed holds the validated struct produced by structured output (the
	// Go analogue of Python's Pydantic `parsed` field), or nil otherwise.
	Parsed any
}

func (r AgentResponse) String() string { return r.Text }
