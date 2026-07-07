// Command agent_pool_example mirrors Python's examples/agent_pool_example.py
// — run three specialized agents in parallel, each on a different task, via
// multi.Pool. AgentPool is for heterogeneous workloads: every agent gets
// its own prompt and returns its own response. Total wall-clock time equals
// the slowest agent, not the sum of all agents.
//
// Compare with AgentGroup (examples/parallel_agents), which runs all
// agents over the same shared prompt.
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

	researcher := agent.New(settings, agent.WithSystemPrompt(
		"You are a research assistant. Give concise, factual answers."))
	critic := agent.New(settings, agent.WithSystemPrompt(
		"You are a critical thinker. Identify weaknesses and counterarguments."))
	summarizer := agent.New(settings, agent.WithSystemPrompt(
		"You are a summarizer. Condense ideas into 2-3 bullet points."))

	pool := multi.NewPool(false)

	tasks := []multi.Task{
		{Agent: researcher, Prompt: "Explain the CAP theorem in distributed systems."},
		{Agent: critic, Prompt: "What are the main criticisms of microservices architecture?"},
		{Agent: summarizer, Prompt: "Summarize the key trade-offs of event-driven architecture."},
	}

	results, err := pool.Run(context.Background(), tasks)
	if err != nil {
		log.Fatal(err)
	}

	labels := []string{"RESEARCHER", "CRITIC", "SUMMARIZER"}
	for i, label := range labels {
		fmt.Printf("=== %s ===\n", label)
		fmt.Println(results[i].Response.Text)
		fmt.Println()
	}
}
