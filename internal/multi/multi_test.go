package multi

import (
	"context"
	"errors"
	"testing"

	"simon-go/internal/agent"
	"simon-go/internal/agent/response"
	"simon-go/internal/config"
	"simon-go/internal/model"
)

func echoAgent() *agent.Agent {
	return agent.New(config.Settings{}, agent.WithModelOverride(model.EchoModel{}))
}

// failingModel always errors, for testing Pool's return_exceptions behavior.
type failingModel struct{}

func (failingModel) Complete(context.Context, []model.Message, []model.ToolSpec) (response.AgentResponse, error) {
	return response.AgentResponse{}, errors.New("provider unavailable")
}

// scriptedModel replies with a fixed text, for testing Triage routing
// without depending on a real provider.
type scriptedModel struct{ text string }

func (s *scriptedModel) Complete(context.Context, []model.Message, []model.ToolSpec) (response.AgentResponse, error) {
	return response.AgentResponse{Text: s.text}, nil
}

func TestGroupRunAllFansOutToAllAgents(t *testing.T) {
	g := NewGroup(map[string]*agent.Agent{
		"a": echoAgent(),
		"b": echoAgent(),
	})

	results, err := g.RunAll(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 || results["a"].Text != "Simon (echo): hi" || results["b"].Text != "Simon (echo): hi" {
		t.Errorf("results = %+v", results)
	}
}

func TestPoolRunsHeterogeneousTasksInOrder(t *testing.T) {
	p := NewPool(false)
	results, err := p.Run(context.Background(), []Task{
		{Agent: echoAgent(), Prompt: "first"},
		{Agent: echoAgent(), Prompt: "second"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Response.Text != "Simon (echo): first" || results[1].Response.Text != "Simon (echo): second" {
		t.Errorf("results out of order or wrong content: %+v", results)
	}
}

func TestPoolReturnExceptionsFalsePropagatesFirstError(t *testing.T) {
	failing := agent.New(config.Settings{SimonMaxRetries: 0}, agent.WithModelOverride(failingModel{}))
	p := NewPool(false)

	_, err := p.Run(context.Background(), []Task{
		{Agent: failing, Prompt: "boom"},
		{Agent: echoAgent(), Prompt: "fine"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPoolReturnExceptionsTrueReportsPerTaskErrors(t *testing.T) {
	failing := agent.New(config.Settings{SimonMaxRetries: 0}, agent.WithModelOverride(failingModel{}))
	p := NewPool(true)

	results, err := p.Run(context.Background(), []Task{
		{Agent: failing, Prompt: "boom"},
		{Agent: echoAgent(), Prompt: "fine"},
	})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if results[0].Err == nil {
		t.Error("expected results[0].Err to be set")
	}
	if results[1].Err != nil || results[1].Response.Text != "Simon (echo): fine" {
		t.Errorf("expected results[1] to succeed, got %+v", results[1])
	}
}

func TestNewTriageValidatesAgentsAndDescriptions(t *testing.T) {
	agents := map[string]*agent.Agent{"support": echoAgent()}

	if _, err := NewTriage(config.Settings{}, map[string]*agent.Agent{}, map[string]string{}); err == nil {
		t.Error("expected error for empty agents map")
	}
	if _, err := NewTriage(config.Settings{}, agents, map[string]string{}); err == nil {
		t.Error("expected error for missing description")
	}
	if _, err := NewTriage(config.Settings{}, agents, map[string]string{"support": "handles support"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTriageDelegatesToMatchedAgent(t *testing.T) {
	sm := &scriptedModel{text: "Support"}
	agents := map[string]*agent.Agent{
		"Support": echoAgent(),
		"Sales":   echoAgent(),
	}
	tr, err := NewTriage(config.Settings{}, agents,
		map[string]string{"Support": "handles support", "Sales": "handles sales"},
		agent.WithModelOverride(sm),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := tr.Run(context.Background(), "my order is broken")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Simon (echo): my order is broken" {
		t.Errorf("expected delegation to the Support agent's echo response, got %q", resp.Text)
	}
}

func TestTriageReturnsErrorOnUnknownAgentName(t *testing.T) {
	sm := &scriptedModel{text: "NoSuchAgent"}
	agents := map[string]*agent.Agent{"Support": echoAgent()}
	tr, err := NewTriage(config.Settings{}, agents, map[string]string{"Support": "handles support"}, agent.WithModelOverride(sm))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := tr.Run(context.Background(), "whatever"); err == nil {
		t.Error("expected an error for an unrecognized routed agent name")
	}
}
