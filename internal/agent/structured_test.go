package agent

import (
	"context"
	"testing"

	"simon-go/internal/agent/response"
	"simon-go/internal/config"
)

type weatherReport struct {
	City  string  `json:"city"`
	TempC float64 `json:"temp_c"`
}

func TestRunStructuredParsesValidJSON(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{
		{Text: `{"city":"Madrid","temp_c":21.5}`},
	}}
	a := New(config.Settings{}, WithModelOverride(sm))

	parsed, resp, err := RunStructured[weatherReport](context.Background(), a, "weather in Madrid?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.City != "Madrid" || parsed.TempC != 21.5 {
		t.Errorf("parsed = %+v", parsed)
	}
	if resp.Parsed != parsed {
		t.Errorf("resp.Parsed = %+v, want %+v", resp.Parsed, parsed)
	}
}

func TestRunStructuredStripsMarkdownFences(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{
		{Text: "```json\n{\"city\":\"Lima\",\"temp_c\":18}\n```"},
	}}
	a := New(config.Settings{}, WithModelOverride(sm))

	parsed, _, err := RunStructured[weatherReport](context.Background(), a, "weather in Lima?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.City != "Lima" {
		t.Errorf("parsed = %+v", parsed)
	}
}

func TestRunStructuredRetriesOnInvalidJSONThenSucceeds(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{
		{Text: "not json at all"},
		{Text: `{"city":"Quito","temp_c":15}`},
	}}
	a := New(config.Settings{SimonStructuredRetries: 2}, WithModelOverride(sm))

	parsed, _, err := RunStructured[weatherReport](context.Background(), a, "weather in Quito?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.City != "Quito" {
		t.Errorf("parsed = %+v", parsed)
	}
	if len(sm.calls) != 2 {
		t.Errorf("expected 2 model calls (initial + 1 retry), got %d", len(sm.calls))
	}
}

func TestRunStructuredExhaustsRetriesAndReturnsStructuredOutputError(t *testing.T) {
	sm := &scriptedModel{responses: []response.AgentResponse{
		{Text: "nope"}, {Text: "still nope"},
	}}
	a := New(config.Settings{SimonStructuredRetries: 1}, WithModelOverride(sm))

	_, _, err := RunStructured[weatherReport](context.Background(), a, "weather?")
	if err == nil {
		t.Fatal("expected a StructuredOutputError")
	}
}
