// Command run_context_example is an idiomatic Go adaptation of Python's
// examples/run_context_example.py, NOT a literal port.
//
// Python's RunContext wraps contextvars.ContextVar so @tool functions can
// read ambient per-request state (bound via `with ctx.bind(...)`) without
// threading it through parameters, staying coroutine-safe under
// asyncio.gather.
//
// simon-go has no RunContext/contextvars analogue, and — more importantly
// — internal/tool.RunToolCall (invoked from inside Agent.Run's ReAct loop)
// always calls each tool with context.Background(), not the caller's ctx.
// So even a stdlib context.Context bound around a.Run(ctx, ...) would
// never reach a tool function; that plumbing doesn't exist for tools
// invoked through the agent loop.
//
// The idiomatic Go replacement demonstrated here is closures: build the
// get_user_name/get_user_tier tools fresh for each request, closing over
// that request's own user/tier values, and hand them to a dedicated Agent
// for that request. Each goroutine gets its own Agent + tools + values, so
// isolation falls out of ordinary Go variable scoping — no shared mutable
// state, no context-injection trick required.
package main

import (
	"context"
	"fmt"
	"sync"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/tool"
)

// buildTools returns get_user_name/get_user_tier tools bound to one
// request's user/tier values via closure.
func buildTools(user, tier string) []tool.Tool {
	type noParams struct{}

	getUserName := tool.New("get_user_name", "Return the name of the current user.",
		func(_ context.Context, _ noParams) (string, error) {
			return user, nil
		})

	getUserTier := tool.New("get_user_tier", "Return the subscription tier of the current user (free or pro).",
		func(_ context.Context, _ noParams) (string, error) {
			return tier, nil
		})

	return []tool.Tool{getUserName, getUserTier}
}

// newRequestAgent builds an Agent whose tools are bound to this request's
// own user/tier values.
func newRequestAgent(settings config.Settings, user, tier string) *agent.Agent {
	return agent.New(settings,
		agent.WithSystemPrompt("You are a helpful assistant. Use the available tools to personalise your response."),
		agent.WithTools(buildTools(user, tier)...),
	)
}

// handleRequest handles a single user request with its own isolated tools.
func handleRequest(settings config.Settings, user, tier, question string) string {
	a := newRequestAgent(settings, user, tier)
	resp, err := a.Run(context.Background(), question)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return resp.Text
}

// handleConcurrentRequests runs multiple requests concurrently, each with
// its own isolated agent+tools — the goroutine analogue of Python's
// asyncio.gather over per-request bound contexts.
func handleConcurrentRequests(settings config.Settings) {
	type req struct{ user, tier, question string }
	requests := []req{
		{"Alice", "pro", "Who am I and what plan am I on?"},
		{"Bob", "free", "Who am I and what plan am I on?"},
		{"Carol", "pro", "Who am I and what plan am I on?"},
	}

	results := make([]string, len(requests))
	var wg sync.WaitGroup
	for i, r := range requests {
		wg.Add(1)
		go func(i int, r req) {
			defer wg.Done()
			text := handleRequest(settings, r.user, r.tier, r.question)
			results[i] = fmt.Sprintf("[%s] %s", r.user, text)
		}(i, r)
	}
	wg.Wait()

	for _, line := range results {
		fmt.Println(line)
	}
}

func main() {
	settings := config.Load()

	fmt.Println("=== Sync (sequential) ===")
	fmt.Println(handleRequest(settings, "Dave", "free", "Who am I and what plan am I on?"))
	fmt.Println()

	fmt.Println("=== Async (concurrent) ===")
	handleConcurrentRequests(settings)
}
