//go:build darwin

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLaunchdPlist_EscapesAndOptionalPaths(t *testing.T) {
	got := buildLaunchdPlist(`/Applications/Clipal & Tools/clipal`, `/tmp/clipal "config"`, `/tmp/stdout's.log`, `/tmp/stderr<&>.log`)

	for _, want := range []string{
		`<string>/Applications/Clipal &amp; Tools/clipal</string>`,
		`<string>/tmp/clipal &quot;config&quot;</string>`,
		`<key>StandardOutPath</key>`,
		`<string>/tmp/stdout&apos;s.log</string>`,
		`<key>StandardErrorPath</key>`,
		`<string>/tmp/stderr&lt;&amp;&gt;.log</string>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected plist to contain %q\n%s", want, got)
		}
	}
}

func TestDarwinManagerPlan_InstallAndStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	manager := darwinManager{}
	opts := Options{
		BinaryPath: "/usr/local/bin/clipal",
		ConfigDir:  "/tmp/clipal",
		StdoutPath: "/tmp/clipal.stdout.log",
		StderrPath: "/tmp/clipal.stderr.log",
	}

	installPlan, err := manager.Plan(ActionInstall, opts)
	if err != nil {
		t.Fatalf("Plan install: %v", err)
	}
	if len(installPlan.Mkdirs) != 1 || !strings.Contains(installPlan.Mkdirs[0], filepath.Join("Library", "LaunchAgents")) {
		t.Fatalf("mkdirs = %#v", installPlan.Mkdirs)
	}
	if len(installPlan.Writes) != 1 {
		t.Fatalf("writes = %#v", installPlan.Writes)
	}
	if installPlan.Writes[0].Perm != 0o644 {
		t.Fatalf("perm = %v, want 0644", installPlan.Writes[0].Perm)
	}
	if !strings.Contains(string(installPlan.Writes[0].Content), "StandardOutPath") {
		t.Fatalf("expected plist content in write")
	}
	if len(installPlan.Commands) != 2 {
		t.Fatalf("commands = %#v", installPlan.Commands)
	}
	if installPlan.Commands[0].Path != "launchctl" || installPlan.Commands[0].Args[0] != "bootstrap" {
		t.Fatalf("first command = %#v", installPlan.Commands[0])
	}
	if installPlan.Commands[1].Args[0] != "kickstart" {
		t.Fatalf("second command = %#v", installPlan.Commands[1])
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", darwinLaunchdLabel+".plist")
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(plistPath, []byte("plist"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	statusPlan, err := manager.Plan(ActionStatus, opts)
	if err != nil {
		t.Fatalf("Plan status: %v", err)
	}
	if len(statusPlan.Commands) != 1 || statusPlan.Commands[0].Args[0] != "print" {
		t.Fatalf("status commands = %#v", statusPlan.Commands)
	}
}
