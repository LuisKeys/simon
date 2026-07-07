// Command memory_agent mirrors Python's examples/memory_agent.py —
// demonstrates conversation memory across two sequential Run calls.
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
	a := agent.New(settings, agent.WithMemory(memory.NewInMemory()))

	ctx := context.Background()

	resp1, err := a.Run(ctx, "My favorite topic is robotics.")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp1.Text)

	resp2, err := a.Run(ctx, "What did I say my favorite topic is?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp2.Text)
}
