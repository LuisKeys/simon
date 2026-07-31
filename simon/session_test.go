package simon_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"simon-go/model"
	"simon-go/pkg/simonerr"
	"simon-go/simon"
)

func TestSessionRunReturnsErrSessionBusyWhenAlreadyRunning(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	rt := newTestRuntime(t, simon.WithModel(blockingModel{block: block, started: started}))
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	go func() {
		_, _ = sess.Run(context.Background(), "hi")
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first Run never started")
	}

	if _, err := sess.Run(context.Background(), "again"); !errors.Is(err, simonerr.ErrSessionBusy) {
		t.Errorf("second Run = %v, want ErrSessionBusy", err)
	}

	close(block)
	sess.Cancel()
}

func TestSessionDifferentSessionsRunConcurrently(t *testing.T) {
	rt := newTestRuntime(t)
	defer rt.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		i := i
		sess, err := rt.NewSession(sessionName(i))
		if err != nil {
			t.Fatalf("NewSession: %v", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := sess.Run(context.Background(), "hi")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	}
}

func TestSessionCancelDuringStreamEmitsRunCancelled(t *testing.T) {
	block := make(chan struct{})
	rt := newTestRuntime(t, simon.WithModel(blockingModel{block: block}))
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	events, err := sess.Stream(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	close(block)

	go func() {
		time.Sleep(20 * time.Millisecond)
		sess.Cancel()
	}()

	var last simon.Event
	for ev := range events {
		last = ev
	}
	if last.Type != simon.EventRunCancelled {
		t.Errorf("last event = %v, want run.cancelled", last.Type)
	}
}

func TestSessionStreamAlwaysDeliversTerminalEventUnderBufferPressure(t *testing.T) {
	rt := newTestRuntime(t, simon.WithModel(&chattyModel{steps: 150}))
	defer rt.Close()

	sess, err := rt.NewSession("s", simon.WithMaxSteps(150))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	events, err := sess.Stream(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var terminalCount int
	var last simon.Event
	for ev := range events {
		last = ev
		if ev.Type == simon.EventRunCompleted || ev.Type == simon.EventRunFailed || ev.Type == simon.EventRunCancelled {
			terminalCount++
		}
	}
	if terminalCount != 1 {
		t.Errorf("terminal events received = %d, want exactly 1", terminalCount)
	}
	if last.Type != simon.EventRunCompleted {
		t.Errorf("last event = %v, want run.completed", last.Type)
	}
}

func TestSessionEventHandlerPanicDoesNotCrashRun(t *testing.T) {
	rt := newTestRuntime(t, simon.WithEventHandler(func(context.Context, simon.Event) {
		panic("boom")
	}))
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := sess.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("Run with panicking handler: %v", err)
	}
}

func TestSessionCloseIsIdempotentAndCancelsActiveRun(t *testing.T) {
	block := make(chan struct{})
	rt := newTestRuntime(t, simon.WithModel(blockingModel{block: block}))
	defer rt.Close()

	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, runErr := sess.Run(context.Background(), "hi")
		done <- runErr
	}()
	close(block)

	if err := sess.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	select {
	case runErr := <-done:
		if runErr == nil {
			t.Error("expected Run to be cancelled by Session.Close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run to be cancelled by Session.Close")
	}
}

// chattyModel requests a trivial no-op tool call `steps` times in a row
// (via ToolCalls with an unregistered tool name, which still fires
// tool.completed events through the normal error path), to flood a
// session's event stream past its buffer size.
type chattyModel struct {
	steps int
	calls int
}

func (m *chattyModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	if m.calls >= m.steps {
		return model.Response{Text: "done"}, nil
	}
	m.calls++
	return model.Response{
		ToolCalls: []model.ToolCall{{ID: "c", Name: "noop", Arguments: nil}},
	}, nil
}
