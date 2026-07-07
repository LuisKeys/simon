// Package tui implements Simon's terminal chat interface, mirroring
// Python's simon/tui.py.
//
// Python hand-rolls raw terminal mode (termios/tty) for inline tab-complete
// hints on "/" commands, matching the project's "no external dependencies"
// educational style. This Go port keeps the same no-heavy-dependency spirit
// but reads lines with bufio.Scanner instead of raw mode: implementing
// character-by-character raw input handling (autocomplete hints redrawn
// per keystroke) is substantial, hard-to-test terminal-specific code for a
// cosmetic feature. RenderMarkdown and the command handling are ported
// faithfully; the tab-completion hint UI is the one deliberately dropped
// feature, noted here rather than silently.
package tui

import (
	"regexp"
	"strings"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	cyan   = "\033[96m"
	green  = "\033[92m"
	yellow = "\033[93m"
	dim    = "\033[2m"
	red    = "\033[91m"
)

var (
	bulletRe = regexp.MustCompile(`^(\s*)[-*] `)
	boldRe   = regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__`)
	italicRe = regexp.MustCompile(`(^|[^\w])\*(\S.*?\S|\S)\*([^\w]|$)|(^|[^\w])_(\S.*?\S|\S)_([^\w]|$)`)
	codeRe   = regexp.MustCompile("`([^`]+)`")
)

// RenderMarkdown converts a subset of Markdown to ANSI-styled terminal
// output, mirroring Python's _render_md: fenced code blocks, #/##/###
// headers, "-"/"*" bullets normalized to "•", **bold**/__bold__,
// *italic*/_italic_, and `code`.
func RenderMarkdown(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	var out []string
	inCodeBlock := false

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			out = append(out, dim+strings.Repeat("─", 40)+reset)
			continue
		}
		if inCodeBlock {
			out = append(out, dim+line+reset)
			continue
		}

		switch {
		case strings.HasPrefix(line, "### "):
			out = append(out, bold+dim+line[4:]+reset)
			continue
		case strings.HasPrefix(line, "## "):
			out = append(out, bold+line[3:]+reset)
			continue
		case strings.HasPrefix(line, "# "):
			out = append(out, bold+yellow+line[2:]+reset)
			continue
		}

		if bulletRe.MatchString(line) {
			line = bulletRe.ReplaceAllString(line, "$1• ")
		}
		line = boldRe.ReplaceAllStringFunc(line, func(m string) string {
			sub := boldRe.FindStringSubmatch(m)
			inner := sub[1]
			if inner == "" {
				inner = sub[2]
			}
			return bold + inner + reset
		})
		line = italicRe.ReplaceAllStringFunc(line, func(m string) string {
			sub := italicRe.FindStringSubmatch(m)
			prefix, inner, suffix := sub[1], sub[2], sub[3]
			if inner == "" {
				prefix, inner, suffix = sub[4], sub[5], sub[6]
			}
			return prefix + dim + inner + reset + suffix
		})
		line = codeRe.ReplaceAllString(line, cyan+"$1"+reset)

		out = append(out, strings.TrimRight(line, " \t"))
	}

	return strings.Join(out, "\n")
}
