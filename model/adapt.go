package model

import (
	"context"

	"simon-go/internal/agent/response"
	internalmodel "simon-go/internal/model"
)

// ToInternal adapts a public Model into internal/model.Model, so it can be
// passed to agent.WithModelOverride. Exported (rather than kept private) so
// the simon facade package — which lives in a different directory but the
// same module — can reuse it without duplicating the translation.
func ToInternal(m Model) internalmodel.Model {
	return &internalAdapter{m: m}
}

type internalAdapter struct {
	m Model
}

func (a *internalAdapter) Complete(ctx context.Context, messages []internalmodel.Message, tools []internalmodel.ToolSpec) (response.AgentResponse, error) {
	req := Request{
		Messages: make([]Message, len(messages)),
		Tools:    make([]ToolSpec, len(tools)),
	}
	for i, msg := range messages {
		req.Messages[i] = Message{
			Role:       Role(msg.Role),
			Content:    msg.Content,
			ToolCalls:  fromInternalToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		}
	}
	for i, spec := range tools {
		req.Tools[i] = ToolSpec{Name: spec.Name, Description: spec.Description, Parameters: spec.Parameters}
	}

	resp, err := a.m.Complete(ctx, req)
	if err != nil {
		return response.AgentResponse{}, err
	}

	usage := response.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  resp.Usage.TotalTokens,
	}
	return response.AgentResponse{
		Text:       resp.Text,
		Usage:      &usage,
		ToolCalls:  toInternalToolCalls(resp.ToolCalls),
		StopReason: resp.StopReason,
	}, nil
}

func fromInternalToolCalls(calls []response.ToolCall) []ToolCall {
	out := make([]ToolCall, len(calls))
	for i, c := range calls {
		out[i] = ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
	}
	return out
}

func toInternalToolCalls(calls []ToolCall) []response.ToolCall {
	out := make([]response.ToolCall, len(calls))
	for i, c := range calls {
		out[i] = response.ToolCall{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
	}
	return out
}
