package ollama

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"simon-go/internal/model"
)

func newTestModel(t *testing.T, handler http.HandlerFunc) *Model {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	m, err := New(srv.URL, "llama3.1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

func TestCompleteReturnsMessageContent(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"hi there"},"done":true}`))
	})

	resp, err := m.Complete(t.Context(), []model.Message{{Role: model.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "hi there" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.Usage != nil {
		t.Error("expected nil Usage, matching Python's free-provider convention")
	}
}

func TestCompleteIgnoresToolsLikePython(t *testing.T) {
	m := newTestModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"llama3.1","created_at":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":"ok"},"done":true}`))
	})

	resp, err := m.Complete(t.Context(), []model.Message{{Role: model.RoleUser, Content: "hi"}}, []model.ToolSpec{
		{Name: "ignored_tool", Description: "should be ignored"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls (Ollama provider ignores tools), got %+v", resp.ToolCalls)
	}
}
