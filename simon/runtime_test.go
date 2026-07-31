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

func newTestRuntime(t *testing.T, opts ...simon.Option) *simon.Runtime {
	t.Helper()
	rt, err := simon.New(append([]simon.Option{simon.WithModel(model.EchoModel{})}, opts...)...)
	if err != nil {
		t.Fatalf("simon.New: %v", err)
	}
	return rt
}

func TestRuntimeCloseIsIdempotent(t *testing.T) {
	rt := newTestRuntime(t)
	if err := rt.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := rt.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestRuntimeMethodsAfterCloseReturnErrRuntimeClosed(t *testing.T) {
	rt := newTestRuntime(t)
	if err := rt.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := rt.NewSession("s"); !errors.Is(err, simonerr.ErrRuntimeClosed) {
		t.Errorf("NewSession after Close = %v, want ErrRuntimeClosed", err)
	}
	if err := rt.RegisterTools(); !errors.Is(err, simonerr.ErrRuntimeClosed) {
		t.Errorf("RegisterTools after Close = %v, want ErrRuntimeClosed", err)
	}
}

func TestRuntimeConcurrentRegisterAndSession(t *testing.T) {
	rt := newTestRuntime(t)
	defer rt.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := rt.NewSession(sessionName(i)); err != nil {
				t.Errorf("NewSession: %v", err)
			}
		}()
	}
	wg.Wait()
}

func sessionName(i int) string {
	return "session-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
}

func TestRuntimeCloseCancelsActiveSessions(t *testing.T) {
	block := make(chan struct{})
	rt := newTestRuntime(t, simon.WithModel(blockingModel{block: block}))
	sess, err := rt.NewSession("s")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, runErr := sess.Run(context.Background(), "hi")
		done <- runErr
	}()

	close(block) // let the model start waiting on ctx.Done()
	if err := rt.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case runErr := <-done:
		if runErr == nil {
			t.Error("expected Run to fail after Runtime.Close, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run to be cancelled by Runtime.Close")
	}
}

// blockingModel signals started (if non-nil, once, non-blocking) as soon as
// Complete is entered, then waits until block is closed, then blocks on ctx
// until cancelled — used to simulate an in-flight run for busy-state and
// cancellation tests.
type blockingModel struct {
	block   chan struct{}
	started chan struct{}
}

func (m blockingModel) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	if m.started != nil {
		select {
		case m.started <- struct{}{}:
		default:
		}
	}
	<-m.block
	<-ctx.Done()
	return model.Response{}, ctx.Err()
}
