// Package integration_test proves Simon works as a true external module
// consumer: it imports only the public import paths a separate Go module
// would use (never anything under internal/), exercising a Runtime, a
// Session, a typed tool, a memory factory, a custom knowledge.Searcher, and
// event delivery end to end.
package integration_test

import (
	"context"
	"testing"
	"time"

	simon "github.com/LuisKeys/simon"
	"github.com/LuisKeys/simon/knowledge"
	"github.com/LuisKeys/simon/memory"
	"github.com/LuisKeys/simon/model"
	"github.com/LuisKeys/simon/tool"
)

// AddParams is the parameter struct for the "add" tool used below; its
// json/jsonschema tags drive tool.New's generated JSON schema.
type AddParams struct {
	A int `json:"a" jsonschema:"required"`
	B int `json:"b" jsonschema:"required"`
}

// scriptedModel requests the "add" tool once, then answers using its
// result — enough to exercise the tool-call round trip deterministically,
// without a live provider API key.
type scriptedModel struct{}

func (scriptedModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	for _, msg := range req.Messages {
		if msg.Role == model.RoleTool {
			return model.Response{Text: "The sum is " + msg.Content}, nil
		}
	}
	return model.Response{
		ToolCalls: []model.ToolCall{
			{ID: "call-1", Name: "add", Arguments: map[string]any{"a": 2, "b": 3}},
		},
	}, nil
}

// staticSearcher is a minimal custom knowledge.Searcher implementation, the
// shape a host application would supply.
type staticSearcher struct{}

func (staticSearcher) Search(_ context.Context, query string, topK int) ([]knowledge.Hit, error) {
	return []knowledge.Hit{{Text: "Simon is a lightweight agent framework.", Source: "static", Score: 1}}, nil
}

func TestExternalConsumer_BasicRun(t *testing.T) {
	rt, err := simon.New(simon.WithModel(model.EchoModel{}))
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	session, err := rt.NewSession("main", simon.WithSystemPrompt("You are a test assistant."))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if session.ID() != "main" {
		t.Fatalf("Session.ID() = %q, want %q", session.ID(), "main")
	}

	resp, err := session.Run(context.Background(), "Hello Simon")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Text == "" {
		t.Fatal("Run: empty response text")
	}
}

func TestExternalConsumer_ToolRoundTrip(t *testing.T) {
	addTool := tool.New("add", "Add two integers", func(_ context.Context, p AddParams) (any, error) {
		return p.A + p.B, nil
	})

	rt, err := simon.New(simon.WithModel(scriptedModel{}))
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	if err := rt.RegisterTool(addTool); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	if err := rt.RegisterTools(addTool); err != nil {
		t.Fatalf("RegisterTools: %v", err)
	}

	session, err := rt.NewSession("tools")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	resp, err := session.Run(context.Background(), "What is 2 + 3?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Text != "The sum is 5" {
		t.Fatalf("Run: got %q, want %q", resp.Text, "The sum is 5")
	}
}

func TestExternalConsumer_MemoryFactory(t *testing.T) {
	factory := memory.FactoryFunc(func(_ context.Context, _ string) (memory.Memory, error) {
		return memory.NewInMemory(), nil
	})

	rt, err := simon.New(simon.WithModel(model.EchoModel{}), simon.WithMemoryFactory(factory))
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	session, err := rt.NewSession("memory")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if _, err := session.Run(context.Background(), "remember this"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := session.Clear(context.Background()); err != nil {
		t.Fatalf("Clear: %v", err)
	}
}

func TestExternalConsumer_JSONFileMemoryFactory(t *testing.T) {
	dir := t.TempDir()
	factory := memory.FactoryFunc(func(_ context.Context, sessionID string) (memory.Memory, error) {
		return memory.NewJSONFile(dir + "/" + sessionID + ".json"), nil
	})

	rt, err := simon.New(simon.WithModel(model.EchoModel{}), simon.WithMemoryFactory(factory))
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	session, err := rt.NewSession("jsonfile")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if _, err := session.Run(context.Background(), "persist this"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestExternalConsumer_CustomKnowledgeSearcher(t *testing.T) {
	rt, err := simon.New(simon.WithModel(model.EchoModel{}), simon.WithKnowledgeBase(staticSearcher{}))
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	session, err := rt.NewSession("knowledge", simon.WithSessionKnowledge(staticSearcher{}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if _, err := session.Run(context.Background(), "What is Simon?"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestExternalConsumer_Events(t *testing.T) {
	var mu chanEvents
	mu.init()

	rt, err := simon.New(
		simon.WithModel(model.EchoModel{}),
		simon.WithEventHandler(func(_ context.Context, ev simon.Event) {
			mu.add(ev)
		}),
	)
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	session, err := rt.NewSession("events")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if _, err := session.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !mu.hasType(simon.EventRunStarted) || !mu.hasType(simon.EventRunCompleted) {
		t.Fatalf("Events: missing expected run.started/run.completed events, got %v", mu.types())
	}
}

func TestExternalConsumer_Stream(t *testing.T) {
	rt, err := simon.New(simon.WithModel(model.EchoModel{}))
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	defer rt.Close()

	session, err := rt.NewSession("stream")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	ch, err := session.Stream(context.Background(), "hi there")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sawCompleted bool
	timeout := time.After(5 * time.Second)
	for !sawCompleted {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("Stream: channel closed before run.completed event")
			}
			if ev.Type == simon.EventRunCompleted {
				sawCompleted = true
			}
		case <-timeout:
			t.Fatal("Stream: timed out waiting for run.completed event")
		}
	}
}

// chanEvents is a tiny concurrency-safe event collector for
// TestExternalConsumer_Events, standing in for whatever aggregation a host
// application would do in its own EventHandler.
type chanEvents struct {
	ch chan simon.Event
	ev []simon.Event
}

func (c *chanEvents) init() { c.ch = make(chan simon.Event, 64) }

func (c *chanEvents) add(ev simon.Event) {
	select {
	case c.ch <- ev:
	default:
	}
}

func (c *chanEvents) drain() {
	for {
		select {
		case ev := <-c.ch:
			c.ev = append(c.ev, ev)
		default:
			return
		}
	}
}

func (c *chanEvents) hasType(t simon.EventType) bool {
	c.drain()
	for _, ev := range c.ev {
		if ev.Type == t {
			return true
		}
	}
	return false
}

func (c *chanEvents) types() []simon.EventType {
	c.drain()
	out := make([]simon.EventType, len(c.ev))
	for i, ev := range c.ev {
		out[i] = ev.Type
	}
	return out
}
