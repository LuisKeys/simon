package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"simon-go/tool"
)

type addParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

func TestNewGeneratesSchemaAndExecutes(t *testing.T) {
	add := tool.New("add", "adds two ints", func(_ context.Context, p addParams) (any, error) {
		return p.A + p.B, nil
	})

	if add.Name() != "add" {
		t.Errorf("Name() = %q, want add", add.Name())
	}
	schema := add.Schema()
	if schema["type"] != "object" {
		t.Errorf("Schema()[type] = %v, want object", schema["type"])
	}

	result, err := add.Execute(context.Background(), json.RawMessage(`{"a":2,"b":3}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != 5 {
		t.Errorf("result = %v, want 5", result)
	}
}

func TestToInternalStringifiesNonStringResults(t *testing.T) {
	add := tool.New("add", "adds two ints", func(_ context.Context, p addParams) (any, error) {
		return p.A + p.B, nil
	})
	internal := tool.ToInternal(add, nil)

	result, err := internal.Call(context.Background(), json.RawMessage(`{"a":2,"b":3}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result != "5" {
		t.Errorf("result = %q, want %q", result, "5")
	}
}

func TestToInternalApprovalDenial(t *testing.T) {
	add := tool.New("add", "adds two ints", func(_ context.Context, p addParams) (any, error) {
		return p.A + p.B, nil
	})
	denyErr := errors.New("denied")
	internal := tool.ToInternal(add, func(context.Context, string, json.RawMessage) error {
		return denyErr
	})

	_, err := internal.Call(context.Background(), json.RawMessage(`{"a":2,"b":3}`))
	if !errors.Is(err, denyErr) {
		t.Errorf("Call error = %v, want %v", err, denyErr)
	}
}

func TestNewRawDefaultsSchemaWhenNil(t *testing.T) {
	raw := tool.NewRaw("noop", "does nothing", nil, func(context.Context, json.RawMessage) (any, error) {
		return "ok", nil
	})
	schema := raw.Schema()
	if schema["type"] != "object" {
		t.Errorf("Schema()[type] = %v, want object", schema["type"])
	}
}
