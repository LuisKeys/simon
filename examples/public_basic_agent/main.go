// Command public_basic_agent is the smallest possible consumer of the
// public simon SDK: build a Runtime, open a Session, run a prompt. With no
// options, the Runtime loads settings from the environment and resolves a
// real provider (OpenAI/Anthropic/Ollama) per prompt via Simon's default
// router, falling back to model.EchoModel only if no provider is configured.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/simon"
)

func main() {
	// Create the Runtime. With no options, settings come from the
	// environment (OPENAI_API_KEY, ANTHROPIC_API_KEY, OLLAMA_HOST, etc.),
	// and each Session resolves a provider via the default router.
	rt, err := simon.New()
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
	response, err := session.Run(context.Background(), "What is quantum computer?")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text)
}
