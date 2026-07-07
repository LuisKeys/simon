package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type weatherArgs struct {
	City string `json:"city" jsonschema:"required,description=City name"`
	Unit string `json:"unit,omitempty" jsonschema:"enum=celsius,enum=fahrenheit"`
}

func TestNewGeneratesSchemaFromStructTags(t *testing.T) {
	tl := New("get_weather", "Get current weather", func(ctx context.Context, a weatherArgs) (string, error) {
		return "sunny in " + a.City, nil
	})

	if tl.Name != "get_weather" || tl.Description != "Get current weather" {
		t.Fatalf("unexpected identity: %+v", tl)
	}

	props, ok := tl.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %v", tl.Schema["properties"])
	}
	if _, ok := props["city"]; !ok {
		t.Errorf("expected 'city' property in schema, got %v", props)
	}
	if _, ok := props["unit"]; !ok {
		t.Errorf("expected 'unit' property in schema, got %v", props)
	}

	required, _ := tl.Schema["required"].([]any)
	if len(required) != 1 || required[0] != "city" {
		t.Errorf("expected required=[city], got %v", required)
	}
}

func TestCallUnmarshalsArgumentsAndInvokesFn(t *testing.T) {
	tl := New("get_weather", "Get current weather", func(ctx context.Context, a weatherArgs) (string, error) {
		return "sunny in " + a.City, nil
	})

	raw, _ := json.Marshal(map[string]string{"city": "Madrid"})
	out, err := tl.Call(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "sunny in Madrid" {
		t.Errorf("got %q", out)
	}
}

func TestCallReturnsToolErrorOnInvalidJSON(t *testing.T) {
	tl := New("get_weather", "Get current weather", func(ctx context.Context, a weatherArgs) (string, error) {
		return "", nil
	})

	_, err := tl.Call(context.Background(), json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected an error for malformed JSON arguments")
	}
}

func TestRegistryAddGetListSpecs(t *testing.T) {
	weather := New("get_weather", "Get current weather", func(ctx context.Context, a weatherArgs) (string, error) {
		return "", nil
	})
	type noArgs struct{}
	ping := New("ping", "Ping", func(ctx context.Context, a noArgs) (string, error) { return "pong", nil })

	reg := NewRegistry(weather, ping)

	if got, ok := reg.Get("ping"); !ok || got.Name != "ping" {
		t.Fatalf("expected to find ping tool, got %+v ok=%v", got, ok)
	}
	if _, ok := reg.Get("missing"); ok {
		t.Fatal("did not expect to find an unregistered tool")
	}

	list := reg.List()
	if len(list) != 2 || list[0].Name != "get_weather" || list[1].Name != "ping" {
		t.Errorf("expected registration order preserved, got %+v", list)
	}

	specs := reg.Specs()
	if len(specs) != 2 || specs[0].Name != "get_weather" {
		t.Errorf("unexpected specs: %+v", specs)
	}
}
