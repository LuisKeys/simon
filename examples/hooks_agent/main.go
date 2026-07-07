// Command hooks_agent mirrors Python's examples/hooks_agent.py —
// observability hooks and usage tracking via agent.WithOnEvent.
//
// Adaptation note: Go's agent.Event data payloads differ slightly from
// Python's AgentEvent (e.g. there is no "latency" key on
// "response_received" — Go emits "usage" and "steps" instead), so the
// response_received branch below prints what the Go agent actually emits
// rather than a literal latency figure.
package main

import (
	"context"
	"fmt"
	"log"

	"simon-go/internal/agent"
	"simon-go/internal/config"
)

func onEvent(event agent.Event) {
	switch event.Type {
	case "model_selected":
		fmt.Printf("[%s] provider=%v\n", event.Type, event.Data["model"])
	case "tool_called":
		result, _ := event.Data["result"].(string)
		if len(result) > 60 {
			result = result[:60]
		}
		fmt.Printf("[%s] %v -> %s\n", event.Type, event.Data["tool"], result)
	case "retry_attempted":
		fmt.Printf("[%s] attempt=%v error=%v\n", event.Type, event.Data["attempt"], event.Data["error"])
	case "response_received":
		fmt.Printf("[%s] steps=%v usage=%v\n", event.Type, event.Data["steps"], event.Data["usage"])
	}
}

func main() {
	settings := config.Load()
	a := agent.New(settings, agent.WithOnEvent(onEvent))

	response, err := a.Run(context.Background(), "What is 2 + 2?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nResponse: %s\n", response.Text)
	fmt.Printf("Total usage: %+v\n", a.TotalUsage)
}
