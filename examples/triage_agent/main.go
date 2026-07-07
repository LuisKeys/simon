// Command triage_agent mirrors Python's examples/triage_agent.py — a
// triage agent routes tasks to the right specialist via multi.NewTriage.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/multi"
)

func main() {
	settings := config.Load()

	codeAgent := agent.New(settings)
	mathAgent := agent.New(settings)
	writingAgent := agent.New(settings)

	triage, err := multi.NewTriage(settings, map[string]*agent.Agent{
		"code":    codeAgent,
		"math":    mathAgent,
		"writing": writingAgent,
	}, map[string]string{
		"code":    "Handles programming questions, debugging, and code reviews.",
		"math":    "Solves mathematical problems, equations, and proofs.",
		"writing": "Helps with essays, creative writing, and editing.",
	})
	if err != nil {
		log.Fatal(err)
	}

	tasks := []string{
		"Write a Python function that reverses a linked list.",
		"Solve the equation 3x^2 - 12x + 9 = 0.",
		"Write a short poem about the ocean at sunset.",
	}

	ctx := context.Background()
	for _, task := range tasks {
		fmt.Printf("Task: %s\n", task)
		response, err := triage.Run(ctx, task)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Response: %s\n", response.Text)
		fmt.Println()
	}
}
