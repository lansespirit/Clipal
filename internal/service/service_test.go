package service

import "testing"

func TestParseAction(t *testing.T) {
	t.Parallel()

	valid := []Action{
		ActionInstall,
		ActionUninstall,
		ActionStart,
		ActionStop,
		ActionRestart,
		ActionStatus,
	}
	for _, want := range valid {
		want := want
		t.Run(string(want), func(t *testing.T) {
			t.Parallel()

			got, err := ParseAction(string(want))
			if err != nil {
				t.Fatalf("ParseAction(%q): %v", want, err)
			}
			if got != want {
				t.Fatalf("ParseAction(%q) = %q, want %q", want, got, want)
			}
		})
	}

	if _, err := ParseAction("reload"); err == nil {
		t.Fatalf("expected unknown action to fail")
	}
}
