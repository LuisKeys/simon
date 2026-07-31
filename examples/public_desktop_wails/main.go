// Command public_desktop_wails shows the event-forwarding pattern a
// desktop application built with Wails (https://wails.io) would use to
// pipe simon.Event values into its frontend. No Wails dependency is added
// here — the SDK itself must stay free of UI/desktop dependencies — this
// is a plain Go program simulating what the handler would do.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/model"
	"simon-go/simon"
)

// uiEvents stands in for a desktop app's event bridge. In a real Wails
// app, forward(ev) would instead call runtime.EventsEmit(wailsCtx,
// "simon:event", ev) from github.com/wailsapp/wails/v2/pkg/runtime, so the
// frontend's JS layer can subscribe with EventsOn("simon:event", ...).
type uiEvents struct {
	received chan simon.Event
}

// forward is the callback passed to WithEventHandler. The runtime calls it
// once for every simon.Event produced during a run, on whatever goroutine
// is driving that run.
func (u *uiEvents) forward(_ context.Context, ev simon.Event) {
	u.received <- ev
}

func main() {
	ui := &uiEvents{received: make(chan simon.Event, 16)}

	// WithEventHandler wires ui.forward into every run on this Runtime, so
	// a desktop frontend can react to progress without polling.
	rt, err := simon.New(
		simon.WithModel(model.EchoModel{}),
		simon.WithEventHandler(ui.forward),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	session, err := rt.NewSession("desktop")
	if err != nil {
		log.Fatal(err)
	}

	// Run the agent on its own goroutine so the main goroutine is free to
	// consume events as they arrive, the way a Wails app would drain them
	// on a UI-event loop instead of blocking on the run itself.
	go func() {
		if _, err := session.Run(context.Background(), "Hello from the desktop app"); err != nil {
			log.Println("run error:", err)
		}
		close(ui.received)
	}()

	// Print each forwarded event as it arrives; in a real Wails app this
	// loop is replaced by runtime.EventsEmit calls inside forward itself.
	for ev := range ui.received {
		fmt.Printf("[ui] %s\n", ev.Type)
	}
}
