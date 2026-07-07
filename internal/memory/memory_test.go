package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInMemoryAddListClear(t *testing.T) {
	ctx := context.Background()
	m := NewInMemory()

	if err := m.Add(ctx, "user", "hi"); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(ctx, "assistant", "hello"); err != nil {
		t.Fatal(err)
	}

	got, err := m.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("List() = %+v, want %+v", got, want)
	}

	if err := m.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ = m.List(ctx)
	if len(got) != 0 {
		t.Errorf("expected empty history after Clear, got %+v", got)
	}
}

func TestInMemoryListReturnsACopy(t *testing.T) {
	ctx := context.Background()
	m := NewInMemory()
	_ = m.Add(ctx, "user", "hi")

	got, _ := m.List(ctx)
	got[0].Content = "mutated"

	fresh, _ := m.List(ctx)
	if fresh[0].Content != "hi" {
		t.Error("List() should return a defensive copy, internal state was mutated")
	}
}

func TestJSONFilePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	restore := chdirTo(t, dir)
	defer restore()

	ctx := context.Background()
	m1 := NewJSONFile("support.json")
	if err := m1.Add(ctx, "user", "help me"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, chatsDir, "support.json")); err != nil {
		t.Fatalf("expected file under .simon_chats/, got: %v", err)
	}

	m2 := NewJSONFile("support.json")
	got, err := m2.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "help me" {
		t.Errorf("expected persisted message to be reloaded, got %+v", got)
	}
}

func TestJSONFileNameIsSanitizedToBasename(t *testing.T) {
	dir := t.TempDir()
	restore := chdirTo(t, dir)
	defer restore()

	m := NewJSONFile("../../etc/passwd.json")
	if err := m.Add(context.Background(), "user", "x"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, chatsDir, "passwd.json")); err != nil {
		t.Fatalf("expected path-traversal name to collapse to basename under .simon_chats/, got: %v", err)
	}
}

func TestJSONFileDefaultName(t *testing.T) {
	dir := t.TempDir()
	restore := chdirTo(t, dir)
	defer restore()

	m := NewJSONFile("")
	_ = m.Add(context.Background(), "user", "x")

	if _, err := os.Stat(filepath.Join(dir, chatsDir, "conversation.json")); err != nil {
		t.Fatalf("expected default filename conversation.json, got: %v", err)
	}
}

func TestJSONFileClearPersists(t *testing.T) {
	dir := t.TempDir()
	restore := chdirTo(t, dir)
	defer restore()

	ctx := context.Background()
	m := NewJSONFile("c.json")
	_ = m.Add(ctx, "user", "x")
	if err := m.Clear(ctx); err != nil {
		t.Fatal(err)
	}

	m2 := NewJSONFile("c.json")
	got, _ := m2.List(ctx)
	if len(got) != 0 {
		t.Errorf("expected cleared history to persist as empty, got %+v", got)
	}
}

func chdirTo(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { os.Chdir(old) }
}
