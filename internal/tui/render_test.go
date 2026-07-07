package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdownHeaders(t *testing.T) {
	cases := map[string]string{
		"# Title":    bold + yellow + "Title" + reset,
		"## Sub":     bold + "Sub" + reset,
		"### Detail": bold + dim + "Detail" + reset,
	}
	for input, want := range cases {
		if got := RenderMarkdown(input); got != want {
			t.Errorf("RenderMarkdown(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRenderMarkdownBullets(t *testing.T) {
	got := RenderMarkdown("- first\n* second")
	want := "• first\n• second"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderMarkdownBoldAndCode(t *testing.T) {
	got := RenderMarkdown("this is **bold** and `code`")
	want := "this is " + bold + "bold" + reset + " and " + cyan + "code" + reset
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	got := RenderMarkdown("```\nx := 1\n```")
	if !strings.Contains(got, dim+"x := 1"+reset) {
		t.Errorf("expected dimmed code line, got %q", got)
	}
	if strings.Count(got, "─") == 0 {
		t.Errorf("expected a fence separator, got %q", got)
	}
}

func TestRenderMarkdownPlainTextUnchanged(t *testing.T) {
	got := RenderMarkdown("just plain text")
	if got != "just plain text" {
		t.Errorf("got %q", got)
	}
}
