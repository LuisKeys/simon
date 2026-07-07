// Command parallel_agents mirrors Python's examples/parallel_agents.py —
// run three specialized agents in parallel over the same prompt via
// multi.Group.RunAll.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/multi"
)

func main() {
	settings := config.Load()

	analyst := agent.New(settings)
	critic := agent.New(settings)
	summarizer := agent.New(settings)

	group := multi.NewGroup(map[string]*agent.Agent{
		"analyst":    analyst,
		"critic":     critic,
		"summarizer": summarizer,
	})

	prompt := "What are the main trade-offs of microservices vs a monolith?"

	results, err := group.RunAll(context.Background(), prompt)
	if err != nil {
		log.Fatal(err)
	}

	for name, response := range results {
		fmt.Printf("=== %s ===\n", strings.ToUpper(name))
		fmt.Println(response.Text)
		fmt.Println()
	}
}
