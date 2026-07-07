package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/model"
)

func TestChatEchoesResponsesUntilQuit(t *testing.T) {
	a := agent.New(config.Settings{}, agent.WithModelOverride(model.EchoModel{}), agent.WithName("Simon"))
	in := strings.NewReader("hello there\n/quit\n")
	var out bytes.Buffer

	if err := Chat(context.Background(), a, in, &out, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Simon (echo): hello there") {
		t.Errorf("expected the echoed response, got:\n%s", got)
	}
	if !strings.Contains(got, "Bye!") {
		t.Errorf("expected a goodbye message, got:\n%s", got)
	}
}

func TestChatBlankLinesAreIgnored(t *testing.T) {
	a := agent.New(config.Settings{}, agent.WithModelOverride(model.EchoModel{}))
	in := strings.NewReader("\n\nhi\n/quit\n")
	var out bytes.Buffer

	if err := Chat(context.Background(), a, in, &out, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "[You]") != 4 { // 2 blank lines + "hi" + "/quit"
		t.Errorf("expected a [You] prompt for each read attempt, got:\n%s", out.String())
	}
}

func TestChatClearInvokesCallback(t *testing.T) {
	a := agent.New(config.Settings{}, agent.WithModelOverride(model.EchoModel{}))
	in := strings.NewReader("/clear\n/quit\n")
	var out bytes.Buffer
	cleared := false

	if err := Chat(context.Background(), a, in, &out, func() { cleared = true }); err != nil {
		t.Fatal(err)
	}
	if !cleared {
		t.Error("expected /clear to invoke the clearScreen callback")
	}
}

func TestChatEOFExitsGracefully(t *testing.T) {
	a := agent.New(config.Settings{}, agent.WithModelOverride(model.EchoModel{}))
	in := strings.NewReader("hi\n") // no /quit — EOF after one line
	var out bytes.Buffer

	if err := Chat(context.Background(), a, in, &out, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Bye!") {
		t.Errorf("expected a goodbye message on EOF, got:\n%s", out.String())
	}
}
