// Package multi implements Simon's multi-agent patterns (AgentGroup,
// AgentPool, TriageAgent), mirroring Python's simon/multi package.
// Python's asyncio.gather becomes plain goroutines + sync.WaitGroup: Go has
// no "already running event loop" concept to guard against, so there is no
// run/run_async duality here either — just Run.
package multi

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"simon-go/internal/agent"
	"simon-go/internal/agent/response"
	"simon-go/internal/config"
)

// Group runs multiple named agents in parallel over the same prompt,
// mirroring AgentGroup.
type Group struct {
	Agents map[string]*agent.Agent
}

// NewGroup builds a Group from named agents.
func NewGroup(agents map[string]*agent.Agent) *Group {
	return &Group{Agents: agents}
}

// RunAll runs every agent concurrently on prompt, returning one response
// per name. Unlike Python's asyncio.gather (which cancels sibling tasks on
// the first exception), Go's goroutines here always run to completion; if
// any agent errors, RunAll returns the first error by map key order once
// all agents have finished.
func (g *Group) RunAll(ctx context.Context, prompt string) (map[string]response.AgentResponse, error) {
	names := make([]string, 0, len(g.Agents))
	for name := range g.Agents {
		names = append(names, name)
	}

	results := make([]response.AgentResponse, len(names))
	errs := make([]error, len(names))

	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(i int, a *agent.Agent) {
			defer wg.Done()
			results[i], errs[i] = a.Run(ctx, prompt)
		}(i, g.Agents[name])
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", names[i], err)
		}
	}

	out := make(map[string]response.AgentResponse, len(names))
	for i, name := range names {
		out[name] = results[i]
	}
	return out, nil
}

// Task is one (agent, prompt) pair for Pool.
type Task struct {
	Agent  *agent.Agent
	Prompt string
}

// Result pairs one Task's response with its error (if any), letting Pool
// surface per-task failures when ReturnExceptions is set — Go has no
// exception-as-value like Python's `list[AgentResponse | BaseException]`.
type Result struct {
	Response response.AgentResponse
	Err      error
}

// Pool runs heterogeneous (agent, prompt) pairs in parallel, each on its
// own goroutine, mirroring AgentPool.
type Pool struct {
	// ReturnExceptions, when true, reports per-task errors in each Result
	// instead of Run failing outright — matching
	// asyncio.gather(return_exceptions=True).
	ReturnExceptions bool
}

// NewPool builds a Pool with the given return_exceptions behavior.
func NewPool(returnExceptions bool) *Pool {
	return &Pool{ReturnExceptions: returnExceptions}
}

// Run executes every task concurrently, in input order in the returned
// slice. When ReturnExceptions is false (the default) and any task errors,
// Run returns (nil, err) for the first error by input order instead of
// partial results, matching asyncio.gather(return_exceptions=False) raising.
func (p *Pool) Run(ctx context.Context, tasks []Task) ([]Result, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	results := make([]Result, len(tasks))
	var wg sync.WaitGroup
	for i, task := range tasks {
		wg.Add(1)
		go func(i int, task Task) {
			defer wg.Done()
			resp, err := task.Agent.Run(ctx, task.Prompt)
			results[i] = Result{Response: resp, Err: err}
		}(i, task)
	}
	wg.Wait()

	if !p.ReturnExceptions {
		for _, r := range results {
			if r.Err != nil {
				return nil, r.Err
			}
		}
	}
	return results, nil
}

// Triage delegates a task to the best-fit specialist agent, chosen by an
// internal router-agent LLM call, mirroring TriageAgent.
type Triage struct {
	agents       map[string]*agent.Agent
	descriptions map[string]string
	routerAgent  *agent.Agent
}

// NewTriage builds a Triage. routerOpts configure the internal routing
// agent (e.g. agent.WithModel to pin its provider); memory and tools are
// deliberately not exposed here, matching Python's
// Agent(memory=False, tools=None) for the router agent.
func NewTriage(settings config.Settings, agents map[string]*agent.Agent, descriptions map[string]string, routerOpts ...agent.Option) (*Triage, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("multi: agents map must not be empty")
	}
	var missing []string
	for name := range agents {
		if _, ok := descriptions[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("multi: missing descriptions for agents: %s", strings.Join(missing, ", "))
	}

	return &Triage{
		agents:       agents,
		descriptions: descriptions,
		routerAgent:  agent.New(settings, routerOpts...),
	}, nil
}

func (t *Triage) routingPrompt(task string) string {
	var b strings.Builder
	b.WriteString("You are a task router. Given the task below, reply with ONLY the name of\n")
	b.WriteString("the most suitable agent from the list. No punctuation, no explanation.\n\n")
	b.WriteString("Available agents:\n")
	for name, desc := range t.descriptions {
		fmt.Fprintf(&b, "  %s: %s\n", name, desc)
	}
	fmt.Fprintf(&b, "\nTask: %s\n\nAgent name:", task)
	return b.String()
}

// Run asks the router agent to pick a specialist by name, then forwards
// prompt to it. Router replies are matched case-insensitively and with
// trailing punctuation stripped, tolerating minor LLM formatting.
func (t *Triage) Run(ctx context.Context, prompt string) (response.AgentResponse, error) {
	routed, err := t.routerAgent.Run(ctx, t.routingPrompt(prompt))
	if err != nil {
		return response.AgentResponse{}, err
	}

	chosen := strings.Trim(strings.TrimSpace(routed.Text), ".,;:!? \t\n")
	var match string
	for name := range t.agents {
		if strings.EqualFold(name, chosen) {
			match = name
			break
		}
	}
	if match == "" {
		names := make([]string, 0, len(t.agents))
		for name := range t.agents {
			names = append(names, name)
		}
		return response.AgentResponse{}, fmt.Errorf(
			"multi: router returned unknown agent %q. Available: %s", routed.Text, strings.Join(names, ", "))
	}

	return t.agents[match].Run(ctx, prompt)
}
