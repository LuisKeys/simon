package response

import "testing"

func TestUsageAddSumsFieldsElementwise(t *testing.T) {
	a := Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}
	b := Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}

	got := a.Add(b)
	want := Usage{InputTokens: 13, OutputTokens: 7, TotalTokens: 20}

	if got != want {
		t.Errorf("Add() = %+v, want %+v", got, want)
	}
}

func TestAgentResponseStringReturnsText(t *testing.T) {
	r := AgentResponse{Text: "hello"}
	if r.String() != "hello" {
		t.Errorf("String() = %q, want %q", r.String(), "hello")
	}
}
