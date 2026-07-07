package agent

import (
	"context"
	"testing"

	"simon-go/internal/agent/response"
	"simon-go/internal/config"
)

// fakeKnowledge is a minimal KnowledgeSearcher for testing the agent's
// integration point, without depending on internal/knowledge's embeddings
// or on-disk index.
type fakeKnowledge struct {
	hits []response.KnowledgeHit
}

func (f fakeKnowledge) Search(context.Context, string, int) ([]response.KnowledgeHit, error) {
	return f.hits, nil
}

func TestRunInjectsKnowledgeContextAsSystemMessage(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{{Text: "answer"}}}
	kb := fakeKnowledge{hits: []response.KnowledgeHit{{Text: "Simon SDK is a Go port", Source: "readme.md"}}}

	a := New(config.Settings{}, WithModelOverride(sm), WithKnowledge(kb))
	if _, err := a.Run(context.Background(), "what is simon sdk?"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, msg := range sm.calls[0] {
		if msg.Content == "Relevant knowledge:\n- (readme.md) Simon SDK is a Go port" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a knowledge-context system message, got messages: %+v", sm.calls[0])
	}
}

func TestRunSkipsKnowledgeContextWhenNoHits(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{{Text: "answer"}}}
	a := New(config.Settings{}, WithModelOverride(sm), WithKnowledge(fakeKnowledge{}))

	if _, err := a.Run(context.Background(), "anything"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, msg := range sm.calls[0] {
		if msg.Role == "system" {
			t.Errorf("expected no system message when knowledge search returns no hits, got %+v", msg)
		}
	}
}

func TestRunWithoutKnowledgeAttachedAddsNoContext(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{{Text: "answer"}}}
	a := New(config.Settings{}, WithModelOverride(sm))

	if _, err := a.Run(context.Background(), "anything"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sm.calls[0]) != 1 {
		t.Errorf("expected only the user message, got %+v", sm.calls[0])
	}
}
