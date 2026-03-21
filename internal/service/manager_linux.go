//go:build linux

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const linuxUnitName = "clipal.service"

type linuxManager struct{}

func DefaultManager() Manager {
	return linuxManager{}
}

func (linuxManager) Plan(action Action, opts Options) (*Plan, error) {
	if opts.BinaryPath == "" {
		return nil, fmt.Errorf("binary path is required")
	}
	if opts.ConfigDir == "" {
		return nil, fmt.Errorf("config dir is required")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	unitPath := filepath.Join(unitDir, linuxUnitName)

	if action == ActionInstall {
		if _, err := os.Stat(unitPath); err == nil && !opts.Force {
			return nil, fmt.Errorf("service already installed (%s); re-run with --force to overwrite", unitPath)
		}
	} else {
		if _, err := os.Stat(unitPath); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("service not installed (missing %s); run: clipal service install", unitPath)
			}
			return nil, err
		}
	}

	plan := &Plan{}
	switch action {
	case ActionInstall:
		content := buildSystemdUnit(opts.BinaryPath, opts.ConfigDir)
		plan.Mkdirs = append(plan.Mkdirs, unitDir)
		plan.Writes = append(plan.Writes, FileWrite{Path: unitPath, Content: []byte(content), Perm: 0o644})
		plan.Commands = append(plan.Commands,
			Command{Path: "systemctl", Args: []string{"--user", "daemon-reload"}},
			Command{Path: "systemctl", Args: []string{"--user", "enable", "--now", linuxUnitName}},
		)
		if opts.Force {
			plan.Commands = append(plan.Commands, Command{Path: "systemctl", Args: []string{"--user", "restart", linuxUnitName}, IgnoreError: true})
		}
	case ActionUninstall:
		plan.Commands = append(plan.Commands,
			Command{Path: "systemctl", Args: []string{"--user", "disable", "--now", linuxUnitName}, IgnoreError: true},
		)
		plan.Removes = append(plan.Removes, unitPath)
		plan.Commands = append(plan.Commands, Command{Path: "systemctl", Args: []string{"--user", "daemon-reload"}, IgnoreError: true})
	case ActionStart:
		plan.Commands = append(plan.Commands, Command{Path: "systemctl", Args: []string{"--user", "start", linuxUnitName}})
	case ActionStop:
		plan.Commands = append(plan.Commands, Command{Path: "systemctl", Args: []string{"--user", "stop", linuxUnitName}})
	case ActionRestart:
		plan.Commands = append(plan.Commands, Command{Path: "systemctl", Args: []string{"--user", "restart", linuxUnitName}})
	case ActionStatus:
		plan.Commands = append(plan.Commands, Command{Path: "systemctl", Args: []string{"--user", "status", linuxUnitName, "--no-pager"}})
	default:
		return nil, fmt.Errorf("unsupported action %q", action)
	}

	return plan, nil
}

func buildSystemdUnit(binaryPath, configDir string) string {
	var b strings.Builder
	_, _ = b.WriteString("[Unit]\n")
	_, _ = b.WriteString("Description=clipal local proxy\n")
	_, _ = b.WriteString("After=network.target\n\n")
	_, _ = b.WriteString("[Service]\n")
	_, _ = b.WriteString("Type=simple\n")
	_, _ = b.WriteString("ExecStart=" + systemdEscape(binaryPath) + " --config-dir " + systemdEscape(configDir) + "\n")
	_, _ = b.WriteString("Restart=always\n")
	_, _ = b.WriteString("RestartSec=2\n\n")
	_, _ = b.WriteString("[Install]\n")
	_, _ = b.WriteString("WantedBy=default.target\n")
	return b.String()
}

func systemdEscape(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\"\\") {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return `"` + s + `"`
	}
	return s
}
