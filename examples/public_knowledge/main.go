// Command public_knowledge demonstrates attaching a knowledge base to a
// Runtime and surfacing a retrieved hit through a Session.Run call, using
// a scripted Model so the run is deterministic and needs no embedding API.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/LuisKeys/simon"
	"github.com/LuisKeys/simon/knowledge"
	"github.com/LuisKeys/simon/model"
)

// staticSearcher is a minimal knowledge.Searcher that always returns the
// same hit — enough to demonstrate the WithKnowledgeBase wiring without a
// real embedder/index.
type staticSearcher struct{}

// Search ignores the query and top-K arguments and always returns the same
// single hit — a real knowledge.Searcher would embed the query and run a
// vector search over an index instead.
func (staticSearcher) Search(_ context.Context, _ string, _ int) ([]knowledge.Hit, error) {
	return []knowledge.Hit{
		{Text: "Simon is a Go port of a Python agent SDK.", Source: "README.md", Score: 0.92},
	}, nil
}

// echoingModel replies with whatever system-message context it was given,
// so the example can show the knowledge hit reaching the model.
type echoingModel struct{}

// Complete looks for the system message the runtime injects with the
// retrieved knowledge context and echoes it back verbatim, so the printed
// output proves the knowledge hit actually reached the model.
func (echoingModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	for _, msg := range req.Messages {
		if msg.Role == model.RoleSystem && msg.Content != "" {
			return model.Response{Text: msg.Content}, nil
		}
	}
	return model.Response{Text: "no knowledge context received"}, nil
}

func main() {
	// WithKnowledgeBase attaches a knowledge.Searcher; the runtime queries
	// it on every Run and injects the results as system-message context
	// before calling the model.
	rt, err := simon.New(
		simon.WithModel(echoingModel{}),
		simon.WithKnowledgeBase(staticSearcher{}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	session, err := rt.NewSession("main")
	if err != nil {
		log.Fatal(err)
	}

	resp, err := session.Run(context.Background(), "What is Simon?")
	if err != nil {
		log.Fatal(err)
	}
	// Prints the staticSearcher's hit text, since echoingModel just relays
	// the injected system message.
	fmt.Println(resp.Text)
}
