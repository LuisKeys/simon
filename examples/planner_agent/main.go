// Command planner_agent mirrors Python's examples/planner_agent.py —
// decompose a goal into tasks and run each one. The checklist prints after
// every status change (via planner.WithOnUpdate), so you watch tasks move
// from pending (o) to in-progress to done, then the final results are
// printed as one description+result block per task.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/planner"
)

func main() {
	settings := config.Load()
	execAgent := agent.New(settings)

	p := planner.New(settings, execAgent, planner.WithOnUpdate(func(tasks []planner.Task) {
		fmt.Println("\n" + planner.RenderTasks(tasks) + "\n")
	}))

	tasks, err := p.Run(context.Background(), "Plan a short blog post about why Python is great for beginners")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nResults:")
	for _, task := range tasks {
		fmt.Printf("- %s\n  %s\n\n", task.Description, task.Result)
	}
}
