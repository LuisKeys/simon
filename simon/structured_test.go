package simon_test

import (
	"context"
	"errors"
	"testing"

	"simon-go/model"
	"simon-go/pkg/simonerr"
	"simon-go/simon"
)

type planOutput struct {
	Title string `json:"title"`
}

type badJSONModel struct{}

func (badJSONModel) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{Text: "not json at all"}, nil
}

func TestRunStructuredSurfacesStructuredOutputErrorAfterRetries(t *testing.T) {
	rt := newTestRuntime(t, simon.WithModel(badJSONModel{}), simon.WithSettings(simon.Settings{StructuredRetries: 1}))
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	_, err = simon.RunStructured[planOutput](context.Background(), sess, "plan something")
	if err == nil {
		t.Fatal("expected an error for invalid JSON output")
	}
	var structuredErr *simonerr.StructuredOutputError
	if !errors.As(err, &structuredErr) {
		t.Errorf("error = %v (%T), want *simonerr.StructuredOutputError", err, err)
	}
}

type validJSONModel struct{}

func (validJSONModel) Complete(_ context.Context, _ model.Request) (model.Response, error) {
	return model.Response{Text: `{"title": "hello"}`}, nil
}

func TestRunStructuredParsesValidJSON(t *testing.T) {
	rt := newTestRuntime(t, simon.WithModel(validJSONModel{}))
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	out, err := simon.RunStructured[planOutput](context.Background(), sess, "plan something")
	if err != nil {
		t.Fatalf("RunStructured: %v", err)
	}
	if out.Title != "hello" {
		t.Errorf("Title = %q, want %q", out.Title, "hello")
	}
}
