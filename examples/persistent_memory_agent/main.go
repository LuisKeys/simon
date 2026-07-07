// Command persistent_memory_agent mirrors Python's
// examples/persistent_memory_agent.py — one JSON file == one conversation.
// Run this program twice: the second run remembers what the first run
// said, because the history lives in the file. Open
// ".simon_chats/robotics_chat.json" to read the stored conversation.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/memory"
)

func main() {
	settings := config.Load()
	mem := memory.NewJSONFile("robotics_chat.json")
	a := agent.New(settings, agent.WithMemory(mem))

	ctx := context.Background()

	history, err := a.Run(ctx, "What did I say my favorite topic is?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(history.Text)

	if _, err := a.Run(ctx, "My favorite topic is robotics."); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Saved. Run this program again to see it remembered across runs.")
}
