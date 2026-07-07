// Command tool_runner_example mirrors Python's
// examples/tool_runner_example.py — tool.Runner, Simon's standalone,
// turn-by-turn tool-use loop.
//
// Mirrors the Anthropic SDK's tool_runner: pass tools + an initial message
// and the runner drives the model<->tool loop for you. Without an API key
// configured this runs against model.EchoModel (no real tool calls), so it
// is safe to run as a smoke test — same as the Python version.
package main

import (
	"context"
	"fmt"

	"simon-go/internal/model"
	"simon-go/internal/tool"
)

var add = tool.New("add", "Add two integers.",
	func(_ context.Context, p struct {
		A int `json:"a"`
		B int `json:"b"`
	}) (string, error) {
		return fmt.Sprintf("%d", p.A+p.B), nil
	})

var multiply = tool.New("multiply", "Multiply two integers.",
	func(_ context.Context, p struct {
		A int `json:"a"`
		B int `json:"b"`
	}) (string, error) {
		return fmt.Sprintf("%d", p.A*p.B), nil
	})

func main() {
	ctx := context.Background()
	m := model.EchoModel{}

	fmt.Println("=== 1) Turn-by-turn iteration (Turns) ===")
	runner := tool.NewRunner(m,
		tool.WithRunnerTools(add, multiply),
		tool.WithRunnerMessages(model.Message{Role: model.RoleUser, Content: "What is (2 + 3) * 4?"}),
	)
	i := 0
	for resp, err := range runner.Turns(ctx) {
		if err != nil {
			fmt.Println("error:", err)
			break
		}
		names := make([]string, len(resp.ToolCalls))
		for j, c := range resp.ToolCalls {
			names[j] = c.Name
		}
		fmt.Printf("turn %d: %q tool_calls=%v\n", i, resp.Text, names)
		i++
	}

	fmt.Println("\n=== 2) UntilDone() — straight to the final answer ===")
	runner = tool.NewRunner(m,
		tool.WithRunnerTools(add),
		tool.WithRunnerMessages(model.Message{Role: model.RoleUser, Content: "What is 15 + 27?"}),
	)
	final, err := runner.UntilDone(ctx)
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Println("final:", final.Text)

	fmt.Println("\n=== 3) Intervention between turns ===")
	runner = tool.NewRunner(m,
		tool.WithRunnerTools(add),
		tool.WithRunnerMessages(model.Message{Role: model.RoleUser, Content: "What is 1 + 1?"}),
		tool.WithMaxIterations(5),
	)
	for resp, err := range runner.Turns(ctx) {
		if err != nil {
			fmt.Println("error:", err)
			break
		}
		results, ok := runner.GenerateToolCallResponse()
		if ok {
			for _, r := range results {
				fmt.Printf("  tool result -> %s (is_error=%v)\n", r.Message.Content, r.IsError)
			}
			// Take over the history: feed results back + inject a steering message.
			msgs := []model.Message{{Role: model.RoleAssistant, Content: resp.Text, ToolCalls: resp.ToolCalls}}
			for _, r := range results {
				msgs = append(msgs, r.Message)
			}
			msgs = append(msgs, model.Message{Role: model.RoleUser, Content: "Please answer concisely."})
			runner.AppendMessages(msgs...)
		}
	}
	if last := runner.LastResponse(); last != nil {
		fmt.Println("final:", last.Text)
	}
}
