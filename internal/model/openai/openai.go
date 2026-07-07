// Package openai adapts the official OpenAI Go SDK to Simon's model.Model
// interface, mirroring Python's simon/models/openai.py.
package openai

import (
	"context"
	"encoding/json"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
)

// Model calls the OpenAI Chat Completions API.
type Model struct {
	Client   openaisdk.Client
	ModelStr string
}

// New builds an OpenAI-backed Model. apiKey is passed explicitly rather than
// read from the environment inside Complete, unlike Python's lazy
// `from openai import AsyncOpenAI` (Go has no optional/lazy import: the SDK
// is always compiled in, so there is nothing to guard with a try/except
// ImportError).
func New(apiKey, modelName string) *Model {
	return NewWithOptions(modelName, option.WithAPIKey(apiKey))
}

// NewWithOptions builds a Model from raw SDK request options, letting
// callers (notably tests) override the base URL or HTTP client.
func NewWithOptions(modelName string, opts ...option.RequestOption) *Model {
	if modelName == "" {
		modelName = "gpt-5"
	}
	return &Model{
		Client:   openaisdk.NewClient(opts...),
		ModelStr: modelName,
	}
}

// Complete implements model.Model.
func (m *Model) Complete(ctx context.Context, messages []model.Message, tools []model.ToolSpec) (response.AgentResponse, error) {
	params := openaisdk.ChatCompletionNewParams{
		Model:    m.ModelStr,
		Messages: toOpenAIMessages(messages),
	}
	if len(tools) > 0 {
		params.Tools = toOpenAITools(tools)
	}

	completion, err := m.Client.Chat.Completions.New(ctx, params)
	if err != nil {
		return response.AgentResponse{}, err
	}

	choice := completion.Choices[0]
	toolCalls := make([]response.ToolCall, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		var args map[string]any
		if call.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
		}
		if args == nil {
			args = map[string]any{}
		}
		toolCalls = append(toolCalls, response.ToolCall{
			ID: call.ID, Name: call.Function.Name, Arguments: args,
		})
	}

	usage := &response.Usage{
		InputTokens:  int(completion.Usage.PromptTokens),
		OutputTokens: int(completion.Usage.CompletionTokens),
		TotalTokens:  int(completion.Usage.TotalTokens),
	}

	return response.AgentResponse{
		Text:       choice.Message.Content,
		Usage:      usage,
		ToolCalls:  toolCalls,
		StopReason: choice.FinishReason,
	}, nil
}

// toOpenAIMessages translates Simon's generic messages to the OpenAI chat
// format, mirroring _to_openai_messages: assistant turns carrying
// ToolCalls, {role: tool} results, and plain {role, content} turns.
func toOpenAIMessages(messages []model.Message) []openaisdk.ChatCompletionMessageParamUnion {
	out := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch {
		case msg.Role == model.RoleAssistant && len(msg.ToolCalls) > 0:
			param := openaisdk.ChatCompletionAssistantMessageParam{}
			if msg.Content != "" {
				param.Content.OfString = openaisdk.String(msg.Content)
			}
			for _, call := range msg.ToolCalls {
				argsJSON, _ := json.Marshal(call.Arguments)
				param.ToolCalls = append(param.ToolCalls, openaisdk.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
						ID: call.ID,
						Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      call.Name,
							Arguments: string(argsJSON),
						},
					},
				})
			}
			out = append(out, openaisdk.ChatCompletionMessageParamUnion{OfAssistant: &param})
		case msg.Role == model.RoleTool:
			out = append(out, openaisdk.ToolMessage(msg.Content, msg.ToolCallID))
		case msg.Role == model.RoleSystem:
			out = append(out, openaisdk.SystemMessage(msg.Content))
		case msg.Role == model.RoleAssistant:
			out = append(out, openaisdk.AssistantMessage(msg.Content))
		default:
			out = append(out, openaisdk.UserMessage(msg.Content))
		}
	}
	return out
}

func toOpenAITools(tools []model.ToolSpec) []openaisdk.ChatCompletionToolUnionParam {
	out := make([]openaisdk.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, openaisdk.ChatCompletionToolUnionParam{
			OfFunction: &openaisdk.ChatCompletionFunctionToolParam{
				Function: openaisdk.FunctionDefinitionParam{
					Name:        t.Name,
					Description: openaisdk.String(t.Description),
					Parameters:  openaisdk.FunctionParameters(params),
				},
			},
		})
	}
	return out
}
