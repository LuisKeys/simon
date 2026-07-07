package planner

import (
	"context"
	"testing"

	"simon-go/internal/agent"
	"simon-go/internal/agent/response"
	"simon-go/internal/config"
	"simon-go/internal/model"
)

// scriptedModel replies with a fixed sequence, one per Complete call.
type scriptedModel struct {
	texts []string
	i     int
}

func (s *scriptedModel) Complete(context.Context, []model.Message, []model.ToolSpec) (response.AgentResponse, error) {
	text := s.texts[s.i]
	s.i++
	return response.AgentResponse{Text: text}, nil
}

func TestParseTasksFromJSONArray(t *testing.T) {
	got := parseTasks(`Sure! ["first task", "second task", ""]`)
	want := []string{"first task", "second task"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTasksFallsBackToLines(t *testing.T) {
	got := parseTasks("1. first task\n- second task\n* third task\n\n")
	want := []string{"first task", "second task", "third task"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRenderTasksShowsStatusIcons(t *testing.T) {
	tasks := []Task{{Description: "a", Status: Pending}, {Description: "b", Status: Done}}
	got := RenderTasks(tasks)
	want := "○ a\n✓ b"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPlanDecomposesGoalIntoTasks(t *testing.T) {
	sm := &scriptedModel{texts: []string{`["research the topic", "write a draft"]`}}
	execAgent := agent.New(config.Settings{}, agent.WithModelOverride(model.EchoModel{}))
	p := New(config.Settings{}, execAgent)
	p.PlannerAgent = agent.New(config.Settings{}, agent.WithModelOverride(sm))

	tasks, err := p.Plan(context.Background(), "ship a blog post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 || tasks[0].Description != "research the topic" || tasks[0].Status != Pending {
		t.Errorf("tasks = %+v", tasks)
	}
}

func TestRunExecutesEachTaskAndUpdatesStatus(t *testing.T) {
	planSM := &scriptedModel{texts: []string{`["task one", "task two"]`}}
	execSM := &scriptedModel{texts: []string{"result one", "result two"}}

	execAgent := agent.New(config.Settings{}, agent.WithModelOverride(execSM))
	p := New(config.Settings{}, execAgent)
	p.PlannerAgent = agent.New(config.Settings{}, agent.WithModelOverride(planSM))

	var updates [][]Task
	p.OnUpdate = func(tasks []Task) {
		snapshot := make([]Task, len(tasks))
		copy(snapshot, tasks)
		updates = append(updates, snapshot)
	}

	tasks, err := p.Run(context.Background(), "ship a blog post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 || tasks[0].Status != Done || tasks[0].Result != "result one" {
		t.Fatalf("tasks = %+v", tasks)
	}
	if tasks[1].Result != "result two" {
		t.Fatalf("tasks[1] = %+v", tasks[1])
	}
	// One update after Plan, then in_progress+done for each of 2 tasks = 5.
	if len(updates) != 5 {
		t.Errorf("expected 5 progress updates, got %d", len(updates))
	}
}
