package agent

import (
	"context"
	"errors"
	"testing"

	"simon-go/internal/agent/response"
	"simon-go/internal/config"
	"simon-go/internal/memory"
	"simon-go/internal/model"
	"simon-go/internal/tool"
)

// scriptedModel replays a fixed sequence of responses, one per Complete
// call, matching the "table of ReAct scenarios" testing strategy from the
// migration plan: assert on the *sequence* of tool calls/messages rather
// than depending on a real provider.
type scriptedModel struct {
	responses []response.AgentResponse
	errs      []error
	calls     [][]model.Message
	i         int
}

func (s *scriptedModel) Complete(_ context.Context, messages []model.Message, _ []model.ToolSpec) (response.AgentResponse, error) {
	s.calls = append(s.calls, messages)
	if s.i >= len(s.responses) {
		panic("scriptedModel: ran out of scripted responses")
	}
	resp, err := s.responses[s.i], errorAt(s.errs, s.i)
	s.i++
	return resp, err
}

func errorAt(errs []error, i int) error {
	if i < len(errs) {
		return errs[i]
	}
	return nil
}

func TestRunSimplePromptWithEcho(t *testing.T) {
	a := New(config.Settings{}, WithModelOverride(model.EchoModel{}))

	resp, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Simon (echo): hello" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.Usage != nil {
		t.Error("expected nil usage for EchoModel")
	}
}

func TestRunPersistsToMemory(t *testing.T) {
	mem := memory.NewInMemory()
	a := New(config.Settings{}, WithModelOverride(model.EchoModel{}), WithMemory(mem))

	if _, err := a.Run(context.Background(), "hi there"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	history, _ := mem.List(context.Background())
	if len(history) != 2 || history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("expected [user, assistant] turns persisted, got %+v", history)
	}
}

func TestRunExecutesToolCallLoopUntilFinalAnswer(t *testing.T) {
	weatherTool := tool.New("get_weather", "Get weather", func(ctx context.Context, args struct {
		City string `json:"city"`
	}) (string, error) {
		return "sunny in " + args.City, nil
	})

	sm := &scriptedModel{
		responses: []response.AgentResponse{
			{
				Text: "",
				ToolCalls: []response.ToolCall{
					{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Madrid"}},
				},
			},
			{Text: "It's sunny in Madrid."},
		},
	}
	a := New(config.Settings{}, WithModelOverride(sm), WithTools(weatherTool))

	resp, err := a.Run(context.Background(), "what's the weather in Madrid?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "It's sunny in Madrid." {
		t.Errorf("Text = %q", resp.Text)
	}
	if len(sm.calls) != 2 {
		t.Fatalf("expected 2 model calls (initial + after tool result), got %d", len(sm.calls))
	}

	secondCallMessages := sm.calls[1]
	last := secondCallMessages[len(secondCallMessages)-1]
	if last.Role != model.RoleTool || last.Content != "sunny in Madrid" || last.ToolCallID != "call_1" {
		t.Errorf("expected tool result fed back as last message, got %+v", last)
	}
}

func TestRunStopsAtMaxSteps(t *testing.T) {
	loopingCall := response.ToolCall{ID: "call_x", Name: "noop", Arguments: map[string]any{}}
	sm := &scriptedModel{responses: []response.AgentResponse{
		{ToolCalls: []response.ToolCall{loopingCall}},
		{ToolCalls: []response.ToolCall{loopingCall}},
		{ToolCalls: []response.ToolCall{loopingCall}},
	}}
	noop := tool.New("noop", "does nothing", func(ctx context.Context, _ struct{}) (string, error) { return "ok", nil })

	a := New(config.Settings{}, WithModelOverride(sm), WithTools(noop), WithMaxSteps(2))

	resp, err := a.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sm.calls) != 3 {
		t.Fatalf("expected initial call + 2 steps = 3 model calls, got %d", len(sm.calls))
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected the loop to stop while tool calls were still pending (max steps hit)")
	}
}

func TestRunReturnsErrorFromModel(t *testing.T) {
	sentinel := errors.New("provider down")
	sm := &scriptedModel{
		responses: []response.AgentResponse{{}},
		errs:      []error{sentinel},
	}
	a := New(config.Settings{SimonMaxRetries: 0}, WithModelOverride(sm))

	_, err := a.Run(context.Background(), "hi")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestToolShorthandBypassesModel(t *testing.T) {
	greet := tool.New("greet", "Greets", func(ctx context.Context, args struct {
		Name string `json:"name"`
	}) (string, error) {
		return "hello, " + args.Name, nil
	})
	sm := &scriptedModel{}
	a := New(config.Settings{}, WithModelOverride(sm), WithTools(greet))

	resp, err := a.Run(context.Background(), `tool:greet {"name":"Ada"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "hello, Ada" {
		t.Errorf("Text = %q", resp.Text)
	}
	if len(sm.calls) != 0 {
		t.Error("expected the tool shorthand to bypass the model entirely")
	}
}

func TestWithSystemPromptPrependsSystemMessage(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{{Text: "ok"}}}
	a := New(config.Settings{}, WithModelOverride(sm), WithSystemPrompt("be terse"))

	if _, err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	first := sm.calls[0][0]
	if first.Role != model.RoleSystem || first.Content != "be terse" {
		t.Errorf("expected system prompt as first message, got %+v", first)
	}
}
