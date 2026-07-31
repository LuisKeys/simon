// Command public_streaming demonstrates Session.Stream: consuming the
// <-chan simon.Event as a run progresses instead of waiting for the final
// Response.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/model"
	"simon-go/simon"
)

func main() {
	rt, err := simon.New(simon.WithModel(model.EchoModel{}))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	session, err := rt.NewSession("main")
	if err != nil {
		log.Fatal(err)
	}

	// Stream returns immediately with a channel of events emitted as the
	// run progresses, instead of blocking until the final Response like
	// Run does.
	events, err := session.Stream(context.Background(), "Stream this reply")
	if err != nil {
		log.Fatal(err)
	}

	// The channel closes once the run reaches a terminal state
	// (completed, failed, or cancelled), so this loop exits on its own.
	for event := range events {
		switch event.Type {
		case simon.EventRunStarted:
			fmt.Println("run started")
		case simon.EventModelSelected:
			fmt.Println("model selected:", event.Data)
		case simon.EventToolRequested, simon.EventToolStarted, simon.EventToolCompleted:
			fmt.Println(event.Type, event.Data)
		case simon.EventRunCompleted:
			// The final Response is delivered as event.Data on this event,
			// as an alternative to reading it from session.Run's return value.
			resp := event.Data.(simon.Response)
			fmt.Println("run completed:", resp.Text)
		case simon.EventRunFailed, simon.EventRunCancelled:
			fmt.Println(event.Type, event.Data)
		}
	}
}
