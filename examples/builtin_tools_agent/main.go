// Command builtin_tools_agent mirrors Python's
// examples/builtin_tools_agent.py — running an agent's built-in tools
// directly via the "tool:name {json_args}" shorthand.
//
// Adaptation note: Go's tool package (unlike Python's simon.tools.builtin)
// ships no built-in tool library — Go can't recover a function's parameter
// names/docstring at runtime the way Python's @tool decorator does, so
// tools are declared explicitly (see internal/tool/tool.go's package doc).
// This example therefore defines small local equivalents of
// datetime_now/fs_list/fs_read/fs_write/shell_run with tool.New, and a
// stubbed web_search (there is no built-in web-search provider on the Go
// side either) that just reports it isn't wired up.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/tool"
)

type noParams struct{}

type pathParams struct {
	Path string `json:"path"`
}

type writeParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type shellParams struct {
	Command string `json:"command"`
}

type searchParams struct {
	Query string `json:"query"`
}

var (
	datetimeNow = tool.New("datetime_now", "Return the current date and time.",
		func(_ context.Context, _ noParams) (string, error) {
			return time.Now().Format(time.RFC3339), nil
		})

	fsList = tool.New("fs_list", "List the contents of a directory.",
		func(_ context.Context, p pathParams) (string, error) {
			entries, err := os.ReadDir(p.Path)
			if err != nil {
				return "", err
			}
			out := ""
			for _, e := range entries {
				out += e.Name() + "\n"
			}
			return out, nil
		})

	fsRead = tool.New("fs_read", "Read the contents of a file.",
		func(_ context.Context, p pathParams) (string, error) {
			data, err := os.ReadFile(p.Path)
			if err != nil {
				return "", err
			}
			return string(data), nil
		})

	fsWrite = tool.New("fs_write", "Write content to a file.",
		func(_ context.Context, p writeParams) (string, error) {
			if err := os.WriteFile(p.Path, []byte(p.Content), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(p.Content), p.Path), nil
		})

	shellRun = tool.New("shell_run", "Run a shell command and return its output.",
		func(ctx context.Context, p shellParams) (string, error) {
			out, err := exec.CommandContext(ctx, "sh", "-c", p.Command).CombinedOutput()
			if err != nil {
				return string(out), err
			}
			return string(out), nil
		})

	webSearch = tool.New("web_search", "Search the web (stub: no provider wired up on the Go side).",
		func(_ context.Context, p searchParams) (string, error) {
			return fmt.Sprintf("web_search is not implemented in simon-go; query was %q", p.Query), nil
		})
)

func run(ctx context.Context, a *agent.Agent, label, call string) {
	fmt.Printf("=== %s ===\n", label)
	resp, err := a.Run(ctx, call)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Text)
}

func main() {
	settings := config.Load()
	a := agent.New(settings, agent.WithTools(datetimeNow, fsList, fsRead, fsWrite, shellRun, webSearch))
	ctx := context.Background()

	run(ctx, a, "datetime_now", `tool:datetime_now {}`)
	fmt.Println()

	run(ctx, a, "fs_list (current dir)", `tool:fs_list {"path": "."}`)
	fmt.Println()

	fmt.Println("=== fs_write + fs_read ===")
	if _, err := a.Run(ctx, `tool:fs_write {"path": "/tmp/simon_test.txt", "content": "hello from simon"}`); err != nil {
		log.Fatal(err)
	}
	resp, err := a.Run(ctx, `tool:fs_read {"path": "/tmp/simon_test.txt"}`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.Text)
	fmt.Println()

	run(ctx, a, "shell_run", `tool:shell_run {"command": "echo hello from shell"}`)
	fmt.Println()

	run(ctx, a, "web_search", `tool:web_search {"query": "golang concurrency tutorial"}`)
}
