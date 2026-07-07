package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v2/option"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
)

// newTestModel builds a Model pointed at a local httptest.Server, matching
// contract-test fixtures against raw captured OpenAI API responses instead
// of depending on network access or a library-specific cassette format.
func newTestModel(t *testing.T, handler http.HandlerFunc) *Model {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewWithOptions("gpt-5", option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
}

func TestCompleteParsesTextAndUsage(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "chatcmpl-1", "object": "chat.completion", "created": 1,
			"model": "gpt-5",
			"choices": [{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"hello there"}}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14}
		}`))
	})

	resp, err := m.Complete(t.Context(), []model.Message{{Role: model.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "hello there" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 4 || resp.Usage.TotalTokens != 14 {
		t.Errorf("Usage = %+v", resp.Usage)
	}
	if resp.StopReason != "stop" {
		t.Errorf("StopReason = %q", resp.StopReason)
	}
}

func TestCompleteParsesToolCalls(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "chatcmpl-2", "object": "chat.completion", "created": 1,
			"model": "gpt-5",
			"choices": [{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,
				"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Madrid\"}"}}]
			}}],
			"usage": {"prompt_tokens": 20, "completion_tokens": 5, "total_tokens": 25}
		}`))
	})

	resp, err := m.Complete(t.Context(), []model.Message{{Role: model.RoleUser, Content: "weather?"}}, []model.ToolSpec{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %+v", resp.ToolCalls)
	}
	call := resp.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "get_weather" || call.Arguments["city"] != "Madrid" {
		t.Errorf("unexpected tool call: %+v", call)
	}
}

func TestToOpenAIMessagesRoundTripsToolLoop(t *testing.T) {
	var captured string
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		captured = string(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"gpt-5",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"done"}}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	})

	messages := []model.Message{
		{Role: model.RoleUser, Content: "weather?"},
		{
			Role: model.RoleAssistant,
			ToolCalls: []response.ToolCall{
				{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Madrid"}},
			},
		},
		{Role: model.RoleTool, Content: "sunny", ToolCallID: "call_1"},
	}

	if _, err := m.Complete(t.Context(), messages, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(captured, `"tool_call_id":"call_1"`) {
		t.Errorf("expected request body to include the tool result, got: %s", captured)
	}
	if !strings.Contains(captured, `"name":"get_weather"`) {
		t.Errorf("expected request body to include the prior tool call, got: %s", captured)
	}
}
