package simonerr

import (
	"errors"
	"testing"
)

func TestProviderErrorMatchesBothSentinels(t *testing.T) {
	cause := errors.New("package not installed")
	err := NewProviderError("openai unavailable", cause)

	if !errors.Is(err, ErrSimon) {
		t.Error("expected errors.Is(err, ErrSimon) to hold")
	}
	if !errors.Is(err, ErrProvider) {
		t.Error("expected errors.Is(err, ErrProvider) to hold")
	}
	if !errors.Is(err, ErrRuntime) {
		t.Error("expected errors.Is(err, ErrRuntime) to hold (dual-inheritance analogue)")
	}
	if !errors.Is(err, cause) {
		t.Error("expected errors.Is(err, cause) to hold")
	}
	if errors.Is(err, ErrTool) {
		t.Error("did not expect errors.Is(err, ErrTool) to hold")
	}
}

func TestToolErrorMatchesValueSentinel(t *testing.T) {
	err := NewToolError("bad arguments", nil)
	if !errors.Is(err, ErrTool) || !errors.Is(err, ErrValue) {
		t.Error("expected ToolError to match ErrTool and ErrValue")
	}
}

func TestStructuredOutputErrorCarriesFields(t *testing.T) {
	err := NewStructuredOutputError("never valid", `{"bad": true}`, 3)

	var structuredErr *StructuredOutputError
	if !errors.As(err, &structuredErr) {
		t.Fatal("expected errors.As to recover *StructuredOutputError")
	}
	if structuredErr.RawText != `{"bad": true}` || structuredErr.Attempts != 3 {
		t.Errorf("unexpected fields: %+v", structuredErr)
	}
	if !errors.Is(err, ErrStructured) || !errors.Is(err, ErrSimon) {
		t.Error("expected StructuredOutputError to match ErrStructured and ErrSimon")
	}
}
