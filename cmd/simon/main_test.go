package main

import "testing"

func TestSplitCommandExtractsFirstNonFlagArg(t *testing.T) {
	cases := []struct {
		argv        []string
		wantCommand string
		wantRest    []string
	}{
		{[]string{"ask", "hello"}, "ask", []string{"hello"}},
		{[]string{"-m", "openai_model", "ask", "hello"}, "ask", []string{"-m", "openai_model", "hello"}},
		{[]string{"--model", "openai_model", "ask", "hello"}, "ask", []string{"--model", "openai_model", "hello"}},
		{[]string{}, "", nil},
	}
	for _, c := range cases {
		command, rest := splitCommand(c.argv)
		if command != c.wantCommand {
			t.Errorf("splitCommand(%v) command = %q, want %q", c.argv, command, c.wantCommand)
		}
		if !equalSlices(rest, c.wantRest) {
			t.Errorf("splitCommand(%v) rest = %v, want %v", c.argv, rest, c.wantRest)
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRunUnknownCommandReturnsUsageError(t *testing.T) {
	if got := run([]string{"bogus"}); got != 2 {
		t.Errorf("run([bogus]) = %d, want 2", got)
	}
}

func TestRunAskWithoutPromptReturnsUsageError(t *testing.T) {
	if got := run([]string{"ask"}); got != 2 {
		t.Errorf("run([ask]) = %d, want 2", got)
	}
}

func TestRunNoArgsReturnsUsageError(t *testing.T) {
	if got := run([]string{}); got != 2 {
		t.Errorf("run([]) = %d, want 2", got)
	}
}
