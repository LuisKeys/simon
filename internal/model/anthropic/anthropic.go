// Package anthropic adapts the official Anthropic Go SDK to Simon's
// model.Model interface, mirroring Python's simon/models/anthropic.py.
package anthropic

import (
	"context"
	"encoding/json"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
)

// Model calls the Anthropic Messages API.
type Model struct {
	Client   anthropicsdk.Client
	ModelStr string
}

// New builds an Anthropic-backed Model.
func New(apiKey, modelName string) *Model {
	return NewWithOptions(modelName, option.WithAPIKey(apiKey))
}

// NewWithOptions builds a Model from raw SDK request options, letting
// callers (notably tests) override the base URL or HTTP client.
func NewWithOptions(modelName string, opts ...option.RequestOption) *Model {
	if modelName == "" {
		modelName = "claude-3-5-sonnet-latest"
	}
	return &Model{
		Client:   anthropicsdk.NewClient(opts...),
		ModelStr: modelName,
	}
}

// Complete implements model.Model.
func (m *Model) Complete(ctx context.Context, messages []model.Message, tools []model.ToolSpec) (response.AgentResponse, error) {
	system, converted := splitMessages(messages)

	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(m.ModelStr),
		MaxTokens: 1024,
		Messages:  converted,
	}
	if system != "" {
		params.System = []anthropicsdk.TextBlockParam{{Text: system}}
	}
	if len(tools) > 0 {
		params.Tools = toAnthropicTools(tools)
	}
	applyThinkingConfig(&params, m.ModelStr)

	msg, err := m.Client.Messages.New(ctx, params)
	if err != nil {
		return response.AgentResponse{}, err
	}

	var textParts []string
	var toolCalls []response.ToolCall
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			var args map[string]any
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &args)
			}
			if args == nil {
				args = map[string]any{}
			}
			toolCalls = append(toolCalls, response.ToolCall{ID: block.ID, Name: block.Name, Arguments: args})
		}
	}

	usage := response.Usage{
		InputTokens:  int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
		TotalTokens:  int(msg.Usage.InputTokens + msg.Usage.OutputTokens),
	}

	return response.AgentResponse{
		Text:       strings.TrimSpace(strings.Join(textParts, "\n")),
		Usage:      &usage,
		ToolCalls:  toolCalls,
		StopReason: string(msg.StopReason),
	}, nil
}

// splitMessages returns (system_prompt, anthropic_messages), mirroring
// _split_messages: system messages concatenate into one string; assistant
// turns with ToolCalls become tool_use content blocks; RoleTool messages
// become tool_result blocks on a user turn.
func splitMessages(messages []model.Message) (string, []anthropicsdk.MessageParam) {
	var systemParts []string
	converted := make([]anthropicsdk.MessageParam, 0, len(messages))

	for _, msg := range messages {
		switch {
		case msg.Role == model.RoleSystem:
			systemParts = append(systemParts, msg.Content)
		case msg.Role == model.RoleAssistant && len(msg.ToolCalls) > 0:
			var blocks []anthropicsdk.ContentBlockParamUnion
			if msg.Content != "" {
				blocks = append(blocks, anthropicsdk.NewTextBlock(msg.Content))
			}
			for _, call := range msg.ToolCalls {
				blocks = append(blocks, anthropicsdk.NewToolUseBlock(call.ID, call.Arguments, call.Name))
			}
			converted = append(converted, anthropicsdk.NewAssistantMessage(blocks...))
		case msg.Role == model.RoleTool:
			converted = append(converted, anthropicsdk.NewUserMessage(
				anthropicsdk.NewToolResultBlock(msg.ToolCallID, msg.Content, false),
			))
		case msg.Role == model.RoleAssistant:
			converted = append(converted, anthropicsdk.NewAssistantMessage(anthropicsdk.NewTextBlock(msg.Content)))
		default:
			converted = append(converted, anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(msg.Content)))
		}
	}
	return strings.Join(systemParts, "\n"), converted
}

// applyThinkingConfig mirrors _thinking_config: thinking is left untouched
// (Fable/Mythos always think, older models don't support the field) except
// for the 4.x family (sonnet-4/opus-4/haiku-4), where it's explicitly
// disabled to avoid unexpectedly long/expensive responses.
func applyThinkingConfig(params *anthropicsdk.MessageNewParams, modelName string) {
	m := strings.ToLower(modelName)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return
	}
	for _, tag := range []string{"sonnet-4", "opus-4", "haiku-4"} {
		if strings.Contains(m, tag) {
			params.Thinking = anthropicsdk.ThinkingConfigParamUnion{
				OfDisabled: &anthropicsdk.ThinkingConfigDisabledParam{},
			}
			return
		}
	}
}

func toAnthropicTools(tools []model.ToolSpec) []anthropicsdk.ToolUnionParam {
	out := make([]anthropicsdk.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := anthropicsdk.ToolInputSchemaParam{}
		if t.Parameters != nil {
			if props, ok := t.Parameters["properties"]; ok {
				schema.Properties = props
			}
			if required, ok := t.Parameters["required"].([]string); ok {
				schema.Required = required
			} else if requiredAny, ok := t.Parameters["required"].([]any); ok {
				for _, r := range requiredAny {
					if s, ok := r.(string); ok {
						schema.Required = append(schema.Required, s)
					}
				}
			}
		}
		out = append(out, anthropicsdk.ToolUnionParam{
			OfTool: &anthropicsdk.ToolParam{
				Name:        t.Name,
				Description: anthropicsdk.String(t.Description),
				InputSchema: schema,
			},
		})
	}
	return out
}
