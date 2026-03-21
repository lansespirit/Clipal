package service

import (
	"context"
	"strings"
	"testing"
)

func TestExecutePlanDryRun(t *testing.T) {
	t.Parallel()

	plan := &Plan{
		Notes:   []string{"note"},
		Mkdirs:  []string{"/tmp/clipal"},
		Removes: []string{"/tmp/clipal.old"},
		Commands: []Command{
			{Path: "echo", Args: []string{"hello world"}},
		},
	}

	out, err := ExecutePlan(context.Background(), plan, true)
	if err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	for _, want := range []string{
		"note",
		"mkdir -p /tmp/clipal",
		"rm /tmp/clipal.old",
		"echo 'hello world'",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestShellEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "Simple", input: "clipal", want: "clipal"},
		{name: "Empty", input: "", want: "''"},
		{name: "NeedsQuotes", input: "hello world", want: "'hello world'"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shellEscape(tt.input); got != tt.want {
				t.Fatalf("shellEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
