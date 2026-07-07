package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/invopop/jsonschema"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
	"simon-go/internal/tool"
	"simon-go/pkg/simonerr"
)

// schemaInstruction builds a system message instructing the model to
// return JSON matching T's schema, mirroring Python's schema_instruction
// (which calls Pydantic's model_json_schema()).
func schemaInstruction[T any]() string {
	var zero T
	schema := jsonschema.Reflect(zero)
	encoded, _ := json.MarshalIndent(schema, "", "  ")
	return "Respond with ONLY a JSON object matching this JSON schema. " +
		"No prose, no markdown fences, no explanation — just the raw JSON object:\n" +
		string(encoded)
}

// parseStructured parses text into T, stripping common LLM decorations
// (```json fences, leading/trailing prose) before unmarshalling, mirroring
// Python's parse_structured.
func parseStructured[T any](text string) (T, error) {
	var zero T
	candidate := strings.TrimSpace(text)

	if strings.HasPrefix(candidate, "```") {
		lines := strings.Split(candidate, "\n")
		inner := lines
		if len(lines) > 0 && strings.HasPrefix(lines[0], "```") {
			inner = lines[1:]
		}
		if len(inner) > 0 && strings.TrimSpace(inner[len(inner)-1]) == "```" {
			inner = inner[:len(inner)-1]
		}
		candidate = strings.TrimSpace(strings.Join(inner, "\n"))
	}

	if start := strings.IndexByte(candidate, '{'); start != -1 {
		if end := strings.LastIndexByte(candidate, '}'); end != -1 && end >= start {
			candidate = candidate[start : end+1]
		}
	}

	var out T
	if err := json.Unmarshal([]byte(candidate), &out); err != nil {
		return zero, err
	}
	return out, nil
}

// RunStructured runs prompt through the ReAct loop like Run, then parses
// the final answer into T, retrying with a corrective message up to
// settings.SimonStructuredRetries times on validation failure — mirroring
// the output_model branch of Python's run_async.
func RunStructured[T any](ctx context.Context, a *Agent, prompt string) (T, response.AgentResponse, error) {
	var zero T

	m, err := a.resolveModel(prompt)
	if err != nil {
		return zero, response.AgentResponse{}, err
	}
	messages, err := a.seedMessages(ctx, prompt)
	if err != nil {
		return zero, response.AgentResponse{}, err
	}
	messages = append(messages, model.Message{Role: model.RoleSystem, Content: schemaInstruction[T]()})

	specs := a.tools.Specs()
	resp, err := a.complete(ctx, m, messages, specs)
	if err != nil {
		return zero, response.AgentResponse{}, err
	}

	step := 0
	for len(resp.ToolCalls) > 0 && step < a.MaxSteps {
		step++
		messages = append(messages, model.Message{Role: model.RoleAssistant, Content: resp.Text, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			result, _ := tool.RunToolCall(a.tools, call)
			messages = append(messages, model.Message{Role: model.RoleTool, Content: result, ToolCallID: call.ID})
		}
		resp, err = a.complete(ctx, m, messages, specs)
		if err != nil {
			return zero, response.AgentResponse{}, err
		}
	}

	maxAttempts := a.settings.SimonStructuredRetries + 1
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		parsed, err := parseStructured[T](resp.Text)
		if err == nil {
			resp.Parsed = parsed
			if a.memory != nil {
				if memErr := a.memory.Add(ctx, "assistant", resp.Text); memErr != nil {
					return zero, response.AgentResponse{}, memErr
				}
			}
			return parsed, resp, nil
		}
		lastErr = err
		if attempt+1 >= maxAttempts {
			break
		}
		messages = append(messages,
			model.Message{Role: model.RoleAssistant, Content: resp.Text},
			model.Message{Role: model.RoleUser, Content: "That was not valid JSON for the schema. Error: " + err.Error() +
				". Reply with ONLY the corrected JSON object, no prose."},
		)
		resp, err = a.complete(ctx, m, messages, specs)
		if err != nil {
			return zero, response.AgentResponse{}, err
		}
	}

	return zero, response.AgentResponse{}, simonerr.NewStructuredOutputError(
		"model output did not match the requested schema after "+strconv.Itoa(maxAttempts)+" attempt(s): "+lastErr.Error(),
		resp.Text, maxAttempts,
	)
}
