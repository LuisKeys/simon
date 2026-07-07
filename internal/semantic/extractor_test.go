package semantic

import (
	"context"
	"testing"

	"simon-go/internal/agent/response"
	"simon-go/internal/events"
	"simon-go/internal/model"
)

type scriptedModel struct {
	text  string
	calls []string // captures the user-message JSON passed to Complete
}

func (s *scriptedModel) Complete(_ context.Context, messages []model.Message, _ []model.ToolSpec) (response.AgentResponse, error) {
	for _, m := range messages {
		if m.Role == model.RoleUser {
			s.calls = append(s.calls, m.Content)
		}
	}
	return response.AgentResponse{Text: s.text}, nil
}

func TestWindowEventTriggersClassificationAndPublishes(t *testing.T) {
	sm := &scriptedModel{text: `{"category":"programming","label":"Writing Go"}`}
	bus := events.NewEventBus(nil)
	ex, err := New(bus, WithModel(sm))
	if err != nil {
		t.Fatal(err)
	}
	ex.Attach()

	var published []events.ActivityEvent
	bus.Subscribe(events.SemanticActivityInferred, func(ctx context.Context, e events.ActivityEvent) error {
		published = append(published, e)
		return nil
	})

	err = bus.Publish(context.Background(), events.New(events.WindowFocusChanged, "test", map[string]any{
		"app_name": "Code", "window_title": "extractor.go",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if len(published) != 1 {
		t.Fatalf("expected 1 published semantic event, got %d", len(published))
	}
	if published[0].Data["category"] != "programming" || published[0].Data["label"] != "Writing Go" {
		t.Errorf("data = %+v", published[0].Data)
	}
	if published[0].Data["app_name"] != "Code" {
		t.Errorf("expected app_name context preserved, got %+v", published[0].Data)
	}
}

func TestClipboardEventIgnoredWithoutPriorWindowContext(t *testing.T) {
	sm := &scriptedModel{text: `{"category":"other","label":"?"}`}
	bus := events.NewEventBus(nil)
	ex, err := New(bus, WithModel(sm))
	if err != nil {
		t.Fatal(err)
	}
	ex.Attach()

	if err := bus.Publish(context.Background(), events.New(events.ClipboardChanged, "test", map[string]any{"kind": "text"})); err != nil {
		t.Fatal(err)
	}
	if len(sm.calls) != 0 {
		t.Errorf("expected no classification without prior window context, got %d calls", len(sm.calls))
	}
}

func TestClipboardEventRefinesExistingWindowContext(t *testing.T) {
	sm := &scriptedModel{text: `{"category":"programming","label":"copying code"}`}
	bus := events.NewEventBus(nil)
	ex, err := New(bus, WithModel(sm))
	if err != nil {
		t.Fatal(err)
	}
	ex.Attach()

	var published []events.ActivityEvent
	bus.Subscribe(events.SemanticActivityInferred, func(ctx context.Context, e events.ActivityEvent) error {
		published = append(published, e)
		return nil
	})

	_ = bus.Publish(context.Background(), events.New(events.WindowFocusChanged, "test", map[string]any{"app_name": "Code"}))
	_ = bus.Publish(context.Background(), events.New(events.ClipboardChanged, "test", map[string]any{"kind": "text"}))

	if len(published) != 2 {
		t.Fatalf("expected window event + clipboard event to both classify, got %d", len(published))
	}
	if published[1].Data["clipboard_kind"] != "text" {
		t.Errorf("expected clipboard_kind in refined context, got %+v", published[1].Data)
	}
}

func TestUnparseableModelOutputIsIgnored(t *testing.T) {
	sm := &scriptedModel{text: "not json at all"}
	bus := events.NewEventBus(nil)
	ex, err := New(bus, WithModel(sm))
	if err != nil {
		t.Fatal(err)
	}
	ex.Attach()

	var published []events.ActivityEvent
	bus.Subscribe(events.SemanticActivityInferred, func(ctx context.Context, e events.ActivityEvent) error {
		published = append(published, e)
		return nil
	})

	_ = bus.Publish(context.Background(), events.New(events.WindowFocusChanged, "test", map[string]any{"app_name": "Code"}))
	if len(published) != 0 {
		t.Errorf("expected unparseable output to be dropped, got %+v", published)
	}
}

func TestUnknownCategoryIsRejected(t *testing.T) {
	sm := &scriptedModel{text: `{"category":"not_a_real_category","label":"x"}`}
	bus := events.NewEventBus(nil)
	ex, err := New(bus, WithModel(sm))
	if err != nil {
		t.Fatal(err)
	}
	ex.Attach()

	var published []events.ActivityEvent
	bus.Subscribe(events.SemanticActivityInferred, func(ctx context.Context, e events.ActivityEvent) error {
		published = append(published, e)
		return nil
	})

	_ = bus.Publish(context.Background(), events.New(events.WindowFocusChanged, "test", map[string]any{"app_name": "Code"}))
	if len(published) != 0 {
		t.Errorf("expected an unknown category to be rejected, got %+v", published)
	}
}

func TestExtractJSONObjectToleratesProseWrapping(t *testing.T) {
	got := extractJSONObject(`Sure, here you go: {"category":"programming","label":"x"} hope that helps!`)
	if got == nil || got["category"] != "programming" {
		t.Errorf("got %+v", got)
	}
}
