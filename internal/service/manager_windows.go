//go:build windows

package service

import (
	"fmt"
	"os"
	"os/user"
	"strings"
)

const windowsTaskName = "Clipal"

type windowsManager struct{}

func DefaultManager() Manager {
	return windowsManager{}
}

func (windowsManager) Plan(action Action, opts Options) (*Plan, error) {
	if opts.BinaryPath == "" {
		return nil, fmt.Errorf("binary path is required")
	}
	if opts.ConfigDir == "" {
		return nil, fmt.Errorf("config dir is required")
	}

	runLine := buildWindowsTaskRunLine(opts.BinaryPath, opts.ConfigDir)

	plan := &Plan{}
	switch action {
	case ActionInstall:
		runAs := currentWindowsUser()
		createArgs := []string{"/Create", "/TN", windowsTaskName, "/TR", runLine, "/SC", "ONLOGON"}
		if runAs != "" {
			createArgs = append(createArgs, "/RU", runAs)
			// Run in the interactive session so clipal can access network normally.
			// The clipal binary detaches its console window when started by a task.
			createArgs = append(createArgs, "/IT")
		}
		// Be explicit about privilege level; default is LIMITED but we want to keep it stable.
		createArgs = append(createArgs, "/RL", "LIMITED")
		if opts.Force {
			createArgs = append(createArgs, "/F")
		}
		plan.Commands = append(plan.Commands,
			Command{Path: "schtasks.exe", Args: createArgs},
			Command{Path: "schtasks.exe", Args: []string{"/Run", "/TN", windowsTaskName}},
		)
	case ActionUninstall:
		plan.Commands = append(plan.Commands,
			Command{Path: "schtasks.exe", Args: []string{"/End", "/TN", windowsTaskName}, IgnoreError: true},
			Command{Path: "schtasks.exe", Args: []string{"/Delete", "/TN", windowsTaskName, "/F"}},
		)
	case ActionStart:
		plan.Commands = append(plan.Commands, Command{Path: "schtasks.exe", Args: []string{"/Run", "/TN", windowsTaskName}})
	case ActionStop:
		plan.Commands = append(plan.Commands, Command{Path: "schtasks.exe", Args: []string{"/End", "/TN", windowsTaskName}, IgnoreError: true})
	case ActionRestart:
		plan.Commands = append(plan.Commands,
			Command{Path: "schtasks.exe", Args: []string{"/End", "/TN", windowsTaskName}, IgnoreError: true},
			Command{Path: "schtasks.exe", Args: []string{"/Run", "/TN", windowsTaskName}},
		)
	case ActionStatus:
		plan.Commands = append(plan.Commands, Command{Path: "schtasks.exe", Args: []string{"/Query", "/TN", windowsTaskName, "/FO", "LIST", "/V"}})
	default:
		return nil, fmt.Errorf("unsupported action %q", action)
	}

	return plan, nil
}

func currentWindowsUser() string {
	// Prefer a fully qualified account name (DOMAIN\User) when available.
	if u, err := user.Current(); err == nil {
		if name := strings.TrimSpace(u.Username); name != "" {
			return name
		}
	}
	// Fall back to environment variables.
	userName := strings.TrimSpace(os.Getenv("USERNAME"))
	if userName == "" {
		return ""
	}
	domain := strings.TrimSpace(os.Getenv("USERDOMAIN"))
	if domain != "" {
		return domain + `\` + userName
	}
	return userName
}

func buildWindowsTaskRunLine(binaryPath, configDir string) string {
	bin := quoteWindowsCmd(binaryPath)
	cfg := quoteWindowsCmd(configDir)
	// Detach the console so users don't accidentally close the task window and
	// kill clipal. This also makes Task Scheduler runs effectively "background".
	return fmt.Sprintf("%s --detach-console --config-dir %s", bin, cfg)
}

// quoteWindowsCmd quotes a value for a Windows command line fragment.
// This is used for schtasks /TR, which expects a single command line string.
func quoteWindowsCmd(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	escaped := strings.ReplaceAll(s, `"`, `\"`)
	return `"` + escaped + `"`
}
