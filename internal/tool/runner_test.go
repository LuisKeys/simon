package tool

import (
	"context"
	"testing"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
)

type scriptedModel struct {
	responses []response.AgentResponse
	calls     int
}

func (s *scriptedModel) Complete(_ context.Context, _ []model.Message, _ []model.ToolSpec) (response.AgentResponse, error) {
	resp := s.responses[s.calls]
	s.calls++
	return resp, nil
}

func TestUntilDoneRunsToolLoopToCompletion(t *testing.T) {
	weatherTool := New("get_weather", "Get weather", func(ctx context.Context, args struct {
		City string `json:"city"`
	}) (string, error) {
		return "sunny in " + args.City, nil
	})
	sm := &scriptedModel{responses: []response.AgentResponse{
		{ToolCalls: []response.ToolCall{{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Madrid"}}}},
		{Text: "It's sunny in Madrid."},
	}}

	r := NewRunner(sm, WithRunnerTools(weatherTool), WithRunnerMessages(model.Message{Role: model.RoleUser, Content: "weather?"}))

	final, err := r.UntilDone(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final.Text != "It's sunny in Madrid." {
		t.Errorf("Text = %q", final.Text)
	}
	if sm.calls != 2 {
		t.Errorf("expected 2 model calls, got %d", sm.calls)
	}

	msgs := r.Messages()
	last := msgs[len(msgs)-1] // tool result message: the final turn had no tool calls, so nothing was appended after it
	if last.Role != model.RoleTool || last.Content != "sunny in Madrid" {
		t.Errorf("expected tool result appended to history, got %+v", last)
	}
}

func TestTurnsYieldsOnePerTurnAndStopsWithoutToolCalls(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{{Text: "just an answer"}}}
	r := NewRunner(sm)

	var got []string
	for resp, err := range r.Turns(context.Background()) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, resp.Text)
	}

	if len(got) != 1 || got[0] != "just an answer" {
		t.Errorf("got %v", got)
	}
}

func TestTurnsStopsAtMaxIterations(t *testing.T) {
	loopingCall := response.ToolCall{ID: "x", Name: "noop", Arguments: map[string]any{}}
	sm := &scriptedModel{responses: []response.AgentResponse{
		{ToolCalls: []response.ToolCall{loopingCall}},
		{ToolCalls: []response.ToolCall{loopingCall}},
		{ToolCalls: []response.ToolCall{loopingCall}},
	}}
	noop := New("noop", "does nothing", func(ctx context.Context, _ struct{}) (string, error) { return "ok", nil })

	r := NewRunner(sm, WithRunnerTools(noop), WithMaxIterations(1))

	count := 0
	for range r.Turns(context.Background()) {
		count++
	}
	if count != 2 {
		t.Errorf("expected initial turn + 1 more iteration = 2 turns, got %d", count)
	}
}

func TestAppendMessagesTakesOverAndSkipsAutoAppend(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{
		{ToolCalls: []response.ToolCall{{ID: "x", Name: "noop", Arguments: map[string]any{}}}},
		{Text: "done"},
	}}
	noop := New("noop", "does nothing", func(ctx context.Context, _ struct{}) (string, error) { return "ok", nil })
	r := NewRunner(sm, WithRunnerTools(noop))

	next, _ := iterFirstTwo(r)
	_ = next

	results, ok := r.GenerateToolCallResponse()
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 tool result to inspect, got %+v ok=%v", results, ok)
	}

	before := len(r.Messages())
	r.AppendMessages(model.Message{Role: model.RoleUser, Content: "custom takeover message"})
	if len(r.Messages()) != before+1 {
		t.Errorf("expected exactly the custom message appended, got %d new messages", len(r.Messages())-before)
	}

	final, err := r.UntilDone(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final.Text != "done" {
		t.Errorf("Text = %q", final.Text)
	}
}

// iterFirstTwo advances the runner exactly one turn (the first), leaving
// LastResponse populated with tool calls pending, so AppendMessages can be
// exercised before the runner would otherwise auto-append.
func iterFirstTwo(r *Runner) (*response.AgentResponse, bool) {
	for resp, err := range r.Turns(context.Background()) {
		if err != nil {
			return nil, false
		}
		return resp, true
	}
	return nil, false
}

func TestWithRunnerSystemPromptPrepends(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{{Text: "ok"}}}
	r := NewRunner(sm,
		WithRunnerMessages(model.Message{Role: model.RoleUser, Content: "hi"}),
		WithRunnerSystemPrompt("be terse"),
	)

	msgs := r.Messages()
	if msgs[0].Role != model.RoleSystem || msgs[0].Content != "be terse" {
		t.Errorf("expected system prompt first, got %+v", msgs[0])
	}
}
