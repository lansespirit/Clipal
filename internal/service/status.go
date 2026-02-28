package service

import (
	"context"
	"encoding/json"
	"os"
)

// Status is a best-effort summary of the OS service state.
// It is intentionally small and stable for human- and machine-readable output.
type Status struct {
	Manager   string `json:"manager"`
	Name      string `json:"name"`  // unit name / launchd label / task name
	Scope     string `json:"scope"` // e.g. "user", "system", "gui/501"
	Installed bool   `json:"installed"`
	Loaded    bool   `json:"loaded"` // manager-level loaded/enabled
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`

	// Optional details when available (may be empty).
	BinaryPath string `json:"binary_path,omitempty"`
	ConfigDir  string `json:"config_dir,omitempty"`
	StdoutPath string `json:"stdout_path,omitempty"`
	StderrPath string `json:"stderr_path,omitempty"`
	LastExit   string `json:"last_exit,omitempty"`
	Detail     string `json:"detail,omitempty"` // small note (e.g. ActiveState/SubState)
}

// GetStatus returns a best-effort status summary and the raw manager output (if any).
// The raw output is suitable for "--raw" / troubleshooting.
func GetStatus(ctx context.Context, opts Options) (Status, string, error) {
	return getStatus(ctx, opts)
}

func (s Status) MarshalJSON() ([]byte, error) {
	// Ensure a stable shape even if new fields are added later.
	type alias Status
	return json.Marshal(alias(s))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
