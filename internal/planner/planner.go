// Package planner decomposes a goal into an ordered task list via an LLM
// call, then runs each task through an Agent, mirroring Python's
// simon/planner/planner.py Planner.
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"simon-go/internal/agent"
	"simon-go/internal/config"
)

// Status is a Task's lifecycle state.
type Status string

const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Done       Status = "done"
)

var icons = map[Status]string{Pending: "○", InProgress: "◐", Done: "✓"}

// Task is one step of a decomposed plan.
type Task struct {
	Description string
	Status      Status
	Result      string
}

const planSystemPrompt = "You are a planning assistant. Break the user's goal into a short, ordered " +
	"list of concrete tasks. Reply with ONLY a JSON array of strings, e.g. " +
	`["first task", "second task"]. No prose, no numbering.`

// RenderTasks renders tasks as a checklist string, mirroring render_tasks.
func RenderTasks(tasks []Task) string {
	lines := make([]string, len(tasks))
	for i, t := range tasks {
		lines[i] = fmt.Sprintf("%s %s", icons[t.Status], t.Description)
	}
	return strings.Join(lines, "\n")
}

var jsonArrayRe = regexp.MustCompile(`(?s)\[.*\]`)
var leadingBulletRe = regexp.MustCompile(`^[\s\-*\d.)]+`)

// parseTasks extracts task strings from the model's reply, mirroring
// _parse_tasks: try to find and JSON-decode an array first, falling back to
// treating non-empty lines (with leading bullets/numbering stripped) as
// tasks.
func parseTasks(text string) []string {
	if match := jsonArrayRe.FindString(text); match != "" {
		var data []any
		if err := json.Unmarshal([]byte(match), &data); err == nil {
			out := make([]string, 0, len(data))
			for _, item := range data {
				if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
					out = append(out, s)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}

	var out []string
	for _, line := range strings.Split(text, "\n") {
		cleaned := strings.TrimSpace(leadingBulletRe.ReplaceAllString(line, ""))
		if cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}

// Planner decomposes a goal into tasks, reports progress via OnUpdate, and
// runs each task through Agent.
type Planner struct {
	Agent        *agent.Agent
	PlannerAgent *agent.Agent
	OnUpdate     func([]Task)
	Tasks        []Task
}

// Option configures a Planner at construction time.
type Option func(*Planner)

// WithOnUpdate sets the progress callback, called after every task-status
// change (mirrors Python's default of printing render_tasks).
func WithOnUpdate(fn func([]Task)) Option { return func(p *Planner) { p.OnUpdate = fn } }

// New builds a Planner. execAgent runs each decomposed task; a second,
// internal agent (same settings, no tools/memory, pinned to
// planSystemPrompt) produces the task list itself, matching Python's
// dedicated `planner_agent`.
func New(settings config.Settings, execAgent *agent.Agent, opts ...Option) *Planner {
	p := &Planner{
		Agent: execAgent,
		PlannerAgent: agent.New(settings,
			agent.WithModel(execAgent.ModelName()),
			agent.WithSystemPrompt(planSystemPrompt),
		),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Planner) emit() {
	if p.OnUpdate != nil {
		p.OnUpdate(p.Tasks)
	}
}

// Plan decomposes goal into Tasks without running them.
func (p *Planner) Plan(ctx context.Context, goal string) ([]Task, error) {
	resp, err := p.PlannerAgent.Run(ctx, goal)
	if err != nil {
		return nil, err
	}
	descriptions := parseTasks(resp.Text)
	p.Tasks = make([]Task, len(descriptions))
	for i, d := range descriptions {
		p.Tasks[i] = Task{Description: d, Status: Pending}
	}
	p.emit()
	return p.Tasks, nil
}

// Run decomposes goal into tasks, then runs each sequentially through
// Agent, updating status/result as it goes.
func (p *Planner) Run(ctx context.Context, goal string) ([]Task, error) {
	if _, err := p.Plan(ctx, goal); err != nil {
		return nil, err
	}

	for i := range p.Tasks {
		p.Tasks[i].Status = InProgress
		p.emit()

		resp, err := p.Agent.Run(ctx, p.Tasks[i].Description)
		if err != nil {
			return p.Tasks, err
		}
		p.Tasks[i].Result = resp.Text
		p.Tasks[i].Status = Done
		p.emit()
	}
	return p.Tasks, nil
}
