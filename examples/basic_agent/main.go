// Command basic_agent mirrors Python's examples/basic_agent.py — the
// smallest possible Simon agent: build one with defaults and run a prompt.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
)

func main() {
	settings := config.Load()
	a := agent.New(settings)

	resp, err := a.Run(context.Background(), "What is reinforcement learning?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Text)
}
