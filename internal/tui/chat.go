package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"simon-go/internal/agent"
)

const banner = "\n" + yellow + bold + "────────────────────────────────────────────────" + reset + "\n" +
	yellow + bold + "  %s" + reset + "\n" +
	yellow + bold + "────────────────────────────────────────────────" + reset + "\n" +
	dim + "  /quit  to quit  |  /clear  to clear screen" + reset + "\n"

// Chat drives an interactive chat loop against a, reading lines from in and
// writing output to out, mirroring Python's tui.chat(). clearScreen is
// called for the "/clear" command (os.Stdout callers typically pass a
// func that prints an ANSI clear sequence or shells out to `clear`).
func Chat(ctx context.Context, a *agent.Agent, in io.Reader, out io.Writer, clearScreen func()) error {
	fmt.Fprintf(out, banner, a.Name)

	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, cyan+bold+"[You] "+reset)
		if !scanner.Scan() {
			break
		}
		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}

		switch strings.ToLower(userInput) {
		case "/quit":
			fmt.Fprintf(out, "\n%sBye!%s\n\n", dim, reset)
			return nil
		case "/clear":
			if clearScreen != nil {
				clearScreen()
			}
			continue
		}

		resp, err := a.Run(ctx, userInput)
		if err != nil {
			fmt.Fprintf(out, "\n%sError: %v%s\n\n", red, err, reset)
			continue
		}

		rendered := RenderMarkdown(resp.Text)
		indented := strings.ReplaceAll(rendered, "\n", "\n  ")
		fmt.Fprintf(out, "\n%s%s[%s]%s\n  %s\n\n", green, bold, a.Name, reset, indented)
		if resp.Usage != nil {
			fmt.Fprintf(out, "%s  tokens — input: %d  output: %d  total: %d%s\n\n",
				dim, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens, reset)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\n%sBye!%s\n\n", dim, reset)
	return nil
}
