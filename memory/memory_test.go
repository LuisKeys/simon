package memory_test

import (
	"context"
	"os"
	"testing"

	"github.com/LuisKeys/simon/memory"
)

func TestInMemoryAddListClear(t *testing.T) {
	m := memory.NewInMemory()
	ctx := context.Background()

	if err := m.Add(ctx, memory.Message{Role: memory.RoleUser, Content: "hi"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	history, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(history) != 1 || history[0].Content != "hi" {
		t.Errorf("history = %+v, want one message with content %q", history, "hi")
	}

	if err := m.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	history, err = m.List(ctx)
	if err != nil {
		t.Fatalf("List after Clear: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("history after Clear = %+v, want empty", history)
	}
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestJSONFilePersistsAcrossInstances(t *testing.T) {
	name := "memory_pkg_test.json"
	defer os.RemoveAll(".simon_chats")

	ctx := context.Background()
	first := memory.NewJSONFile(name)
	if err := first.Add(ctx, memory.Message{Role: memory.RoleAssistant, Content: "stored"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	second := memory.NewJSONFile(name)
	history, err := second.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(history) != 1 || history[0].Content != "stored" {
		t.Errorf("history = %+v, want one message with content %q", history, "stored")
	}
}

func TestToInternalRoundTrips(t *testing.T) {
	m := memory.NewInMemory()
	internal := memory.ToInternal(m)
	ctx := context.Background()

	if err := internal.Add(ctx, "user", "hello"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	history, err := internal.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(history) != 1 || history[0].Role != "user" || history[0].Content != "hello" {
		t.Errorf("history = %+v, want one user/hello message", history)
	}
}
