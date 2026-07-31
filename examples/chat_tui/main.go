// Command chat_tui mirrors Python's examples/chat_tui.py — an interactive
// terminal chat with a named, personality-driven agent.
package main

import (
	"context"
	"log"
	"os"

	"github.com/LuisKeys/simon/internal/agent"
	"github.com/LuisKeys/simon/internal/config"
	"github.com/LuisKeys/simon/internal/memory"
	"github.com/LuisKeys/simon/internal/tui"
)

func main() {
	settings := config.Load()
	a := agent.New(settings,
		agent.WithName("Luke"),
		agent.WithMemory(memory.NewInMemory()),
		agent.WithSystemPrompt(
			"You are Luke, an expert chef with decades of experience in international cuisine. "+
				"You respond with enthusiasm, share professional cooking techniques, suggest "+
				"alternative ingredients when helpful, and always end your response with a practical tip.",
		),
	)

	if err := tui.Chat(context.Background(), a, os.Stdin, os.Stdout, nil); err != nil {
		log.Fatal(err)
	}
}
