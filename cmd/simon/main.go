// Command simon is Simon SDK's command-line interface, mirroring Python's
// simon/cli.py (chat | ask | index | plan). It uses the standard library's
// flag package with manual subcommands rather than a third-party CLI
// framework — consistent with the small size of the original argparse
// setup (62 lines) and the project's minimal-dependencies philosophy.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"simon-go/internal/agent"
	"simon-go/internal/config"
	"simon-go/internal/knowledge"
	"simon-go/internal/knowledge/embed"
	"simon-go/internal/memory"
	"simon-go/internal/planner"
	"simon-go/internal/tui"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	if len(argv) == 0 {
		usage()
		return 2
	}

	model := flag.NewFlagSet("simon", flag.ContinueOnError)
	modelFlag := model.String("m", "", "Force a provider (e.g. OPENAI_MODEL).")
	model.StringVar(modelFlag, "model", "", "Force a provider (e.g. OPENAI_MODEL).")

	// The global -m/--model flag can appear before or after the subcommand
	// name, matching argparse's behavior for a parent parser flag.
	command, rest := splitCommand(argv)
	if err := model.Parse(rest); err != nil {
		return 2
	}

	settings := config.Load()

	switch command {
	case "chat":
		return cmdChat(settings, *modelFlag)
	case "ask":
		return cmdAsk(settings, *modelFlag, model.Args())
	case "index":
		return cmdIndex(settings, model.Args())
	case "plan":
		return cmdPlan(settings, *modelFlag, model.Args())
	default:
		fmt.Fprintf(os.Stderr, "simon: unknown command %q\n", command)
		usage()
		return 2
	}
}

// splitCommand extracts the first non-flag argument as the subcommand,
// leaving the rest (including any -m/--model appearing before it) for the
// flag set to parse. -m/--model takes a separate value argument (unless
// written as "-m=value"/"--model=value"), so that value is skipped too,
// not mistaken for the command.
func splitCommand(argv []string) (command string, rest []string) {
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if len(arg) == 0 || arg[0] != '-' {
			rest = append(append([]string{}, argv[:i]...), argv[i+1:]...)
			return arg, rest
		}
		if (arg == "-m" || arg == "--model") && i+1 < len(argv) {
			i++ // skip the flag's value
		}
	}
	return "", argv
}

func usage() {
	fmt.Fprintln(os.Stderr, `simon: Simon SDK command-line interface.

Usage:
  simon [-m|--model NAME] <command> [args]

Commands:
  chat            Start an interactive terminal chat.
  ask <prompt>    Run a single prompt and print the answer.
  index <path>    Index a file or folder into the knowledge base.
  plan <goal>     Decompose a goal into tasks and run them.`)
}

func cmdChat(settings config.Settings, model string) int {
	a := agent.New(settings, agent.WithModel(model), agent.WithMemory(memory.NewInMemory()))
	if err := tui.Chat(context.Background(), a, os.Stdin, os.Stdout, nil); err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}
	return 0
}

func cmdAsk(settings config.Settings, model string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "simon: ask requires a prompt")
		return 2
	}
	a := agent.New(settings, agent.WithModel(model))
	resp, err := a.Run(context.Background(), args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}
	fmt.Println(resp.Text)
	return 0
}

func cmdIndex(settings config.Settings, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "simon: index requires a path")
		return 2
	}
	path := args[0]

	embedder, err := embed.Default(settings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}
	storePath := settings.KnowledgeStorePath
	if storePath == "" {
		storePath = ".simon_knowledge"
	}
	kb, err := knowledge.New(embedder, storePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}

	count, err := kb.Add(context.Background(), path, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}
	fmt.Printf("Indexed %d chunk(s) from %s\n", count, path)
	return 0
}

func cmdPlan(settings config.Settings, model string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "simon: plan requires a goal")
		return 2
	}
	execAgent := agent.New(settings, agent.WithModel(model))
	p := planner.New(settings, execAgent, planner.WithOnUpdate(func(tasks []planner.Task) {
		fmt.Println("\n" + planner.RenderTasks(tasks) + "\n")
	}))

	if _, err := p.Run(context.Background(), args[0]); err != nil {
		fmt.Fprintln(os.Stderr, "simon:", err)
		return 1
	}
	return 0
}
