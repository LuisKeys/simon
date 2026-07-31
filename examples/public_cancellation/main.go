// Command public_cancellation demonstrates Session.Cancel: a slow scripted
// Model is interrupted mid-flight, and the resulting event stream ends
// with run.cancelled instead of run.completed.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/LuisKeys/simon"
	"github.com/LuisKeys/simon/model"
)

// slowModel takes long enough that a Cancel() issued shortly after Stream
// starts reliably interrupts it before it replies.
type slowModel struct{}

func (slowModel) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	// Wait 5 seconds before replying, but respect context cancellation —
	// exactly what a real HTTP-backed model.Model implementation must do
	// so Session.Cancel can actually interrupt an in-flight call.
	select {
	case <-time.After(5 * time.Second):
		return model.Response{Text: "finished"}, nil
	case <-ctx.Done():
		return model.Response{}, ctx.Err()
	}
}

func main() {
	rt, err := simon.New(simon.WithModel(slowModel{}))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	session, err := rt.NewSession("main")
	if err != nil {
		log.Fatal(err)
	}

	// Stream, unlike Run, returns immediately with an event channel instead
	// of blocking until the run finishes.
	events, err := session.Stream(context.Background(), "This will be cancelled")
	if err != nil {
		log.Fatal(err)
	}

	// Give the run a brief head start, then cancel it while slowModel is
	// still waiting on its 5-second timer.
	go func() {
		time.Sleep(100 * time.Millisecond)
		session.Cancel()
	}()

	// The event stream ends with "run.cancelled" instead of
	// "run.completed" because Cancel() fired before the model replied.
	for event := range events {
		fmt.Println(event.Type)
	}
}
