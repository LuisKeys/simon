package anthropic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
)

func newTestModel(t *testing.T, modelName string, handler http.HandlerFunc) *Model {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewWithOptions(modelName, option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
}

func TestCompleteParsesTextAndUsage(t *testing.T) {
	m := newTestModel(t, "claude-3-5-sonnet-latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude-3-5-sonnet-latest",
			"content":[{"type":"text","text":"hello there"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":10,"output_tokens":4}
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
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q", resp.StopReason)
	}
}

func TestCompleteParsesToolUseBlocks(t *testing.T) {
	m := newTestModel(t, "claude-3-5-sonnet-latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"msg_2","type":"message","role":"assistant","model":"claude-3-5-sonnet-latest",
			"content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Madrid"}}],
			"stop_reason":"tool_use",
			"usage":{"input_tokens":20,"output_tokens":5}
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
	if call.ID != "toolu_1" || call.Name != "get_weather" || call.Arguments["city"] != "Madrid" {
		t.Errorf("unexpected tool call: %+v", call)
	}
}

func TestSplitMessagesRoundTripsToolLoopAndSystem(t *testing.T) {
	var captured string
	m := newTestModel(t, "claude-3-5-sonnet-latest", func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		captured = string(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","type":"message","role":"assistant","model":"claude-3-5-sonnet-latest",
			"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}}`))
	})

	messages := []model.Message{
		{Role: model.RoleSystem, Content: "be nice"},
		{Role: model.RoleUser, Content: "weather?"},
		{
			Role: model.RoleAssistant,
			ToolCalls: []response.ToolCall{
				{ID: "toolu_1", Name: "get_weather", Arguments: map[string]any{"city": "Madrid"}},
			},
		},
		{Role: model.RoleTool, Content: "sunny", ToolCallID: "toolu_1"},
	}

	if _, err := m.Complete(t.Context(), messages, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(captured, `"system":[{"text":"be nice"`) {
		t.Errorf("expected system prompt in request body, got: %s", captured)
	}
	if !strings.Contains(captured, `"tool_use_id":"toolu_1"`) {
		t.Errorf("expected tool_result block in request body, got: %s", captured)
	}
}

func TestApplyThinkingConfig(t *testing.T) {
	cases := []struct {
		model        string
		wantDisabled bool
	}{
		{"claude-sonnet-4-20250514", true},
		{"claude-opus-4-1", true},
		{"claude-haiku-4-5", true},
		{"claude-3-5-sonnet-latest", false},
		{"fable-5", false},
		{"mythos-1", false},
	}
	for _, c := range cases {
		params := anthropicsdk.MessageNewParams{}
		applyThinkingConfig(&params, c.model)
		gotDisabled := params.Thinking.OfDisabled != nil
		if gotDisabled != c.wantDisabled {
			t.Errorf("model %q: disabled = %v, want %v", c.model, gotDisabled, c.wantDisabled)
		}
	}
}
