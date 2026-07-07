package model

import (
	"context"
	"testing"
)

func TestEchoModelRepliesWithLastUserMessage(t *testing.T) {
	m := EchoModel{}
	messages := []Message{
		{Role: RoleSystem, Content: "be nice"},
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "hi there"},
		{Role: RoleUser, Content: "how are you?"},
	}

	resp, err := m.Complete(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "Simon (echo): how are you?"; resp.Text != want {
		t.Errorf("Text = %q, want %q", resp.Text, want)
	}
	if resp.Usage != nil {
		t.Error("expected nil Usage for EchoModel, matching Python's free-provider convention")
	}
}

func TestEchoModelWithNoUserMessage(t *testing.T) {
	m := EchoModel{}
	resp, err := m.Complete(context.Background(), []Message{{Role: RoleSystem, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Simon (echo): " {
		t.Errorf("Text = %q, want empty echo", resp.Text)
	}
}
