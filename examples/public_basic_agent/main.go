// Command public_basic_agent is the smallest possible consumer of the
// public simon SDK: build a Runtime, open a Session, run a prompt. It uses
// model.EchoModel so it needs no API key and always prints the same thing.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/model"
	"simon-go/simon"
)

func main() {
	// Create the Runtime. model.EchoModel just echoes the prompt back, so
	// this example runs with no network calls and no API key.
	rt, err := simon.New(simon.WithModel(model.EchoModel{}))
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	// A Session holds conversation state for one logical conversation.
	// "main" is just an identifier — pick any string that's unique per
	// conversation in your app.
	session, err := rt.NewSession("main")
	if err != nil {
		log.Fatal(err)
	}

	// Run sends the prompt through the full agent loop and blocks until a
	// final Response is produced.
	response, err := session.Run(context.Background(), "Explain this repository")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text)
}
