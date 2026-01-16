package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/service"
)

func runService(args []string) {
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configDir := fs.String("config-dir", "", "Configuration directory (default: ~/.clipal)")
	binaryPath := fs.String("bin", "", "Path to clipal binary (default: current executable)")
	force := fs.Bool("force", false, "Reinstall/update the system service if it already exists")
	dryRun := fs.Bool("dry-run", false, "Print actions without executing them")
	timeout := fs.Duration("timeout", 30*time.Second, "Overall timeout for service manager commands")

	// macOS launchd (optional)
	stdoutPath := fs.String("stdout", "", "macOS: launchd StandardOutPath (optional)")
	stderrPath := fs.String("stderr", "", "macOS: launchd StandardErrorPath (optional)")

	actionArg, flagArgs, err := splitServiceArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipal service: %v\n", err)
		printServiceUsage()
		os.Exit(2)
	}

	if err := fs.Parse(flagArgs); err != nil {
		os.Exit(2)
	}

	action, err := service.ParseAction(actionArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipal service: %v\n", err)
		printServiceUsage()
		os.Exit(2)
	}

	cfgDir := *configDir
	if cfgDir == "" {
		cfgDir = config.GetConfigDir()
	}

	bin := *binaryPath
	if bin == "" {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			fmt.Fprintf(os.Stderr, "clipal service: failed to determine executable path: %v\n", exeErr)
			os.Exit(1)
		}
		if resolved, resolvedErr := filepath.EvalSymlinks(exe); resolvedErr == nil {
			exe = resolved
		}
		bin = exe
	}

	mgr := service.DefaultManager()
	opts := service.Options{
		ConfigDir:  cfgDir,
		BinaryPath: bin,
		Force:      *force,
		DryRun:     *dryRun,
		StdoutPath: *stdoutPath,
		StderrPath: *stderrPath,
	}

	plan, err := mgr.Plan(action, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipal service %s failed: %v\n", action, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	out, err := service.ExecutePlan(ctx, plan, opts.DryRun)
	if err != nil {
		if out != "" {
			fmt.Fprint(os.Stderr, out)
		}
		fmt.Fprintf(os.Stderr, "clipal service %s failed: %v\n", action, err)
		os.Exit(1)
	}

	if out != "" {
		fmt.Fprint(os.Stdout, out)
	}
}

func printServiceUsage() {
	fmt.Fprintln(os.Stderr, "usage: clipal service [flags] <install|uninstall|start|stop|restart|status>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "examples:")
	fmt.Fprintln(os.Stderr, "  clipal service install --config-dir ~/.clipal")
	fmt.Fprintln(os.Stderr, "  clipal service restart")
	fmt.Fprintln(os.Stderr, "  clipal service status")
}

// splitServiceArgs accepts both:
//
//	clipal service [flags] <action>
//	clipal service <action> [flags]
//
// It extracts the action token and returns a reordered slice containing only
// flag arguments (so flag.FlagSet can parse them reliably).
func splitServiceArgs(args []string) (action string, flagArgs []string, err error) {
	// Flags that require a following value (unless provided via -flag=value).
	needsValue := map[string]bool{
		"config-dir": true,
		"bin":        true,
		"timeout":    true,
		"stdout":     true,
		"stderr":     true,
	}

	var haveAction bool
	for i := 0; i < len(args); i++ {
		a := args[i]

		// Standard "--" terminator: everything after is treated as positional.
		if a == "--" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("missing action")
			}
			if haveAction {
				return "", nil, fmt.Errorf("unexpected argument %q", args[i+1])
			}
			action = args[i+1]
			haveAction = true
			// Any further positional args are not supported.
			if i+2 < len(args) {
				return "", nil, fmt.Errorf("unexpected argument %q", args[i+2])
			}
			break
		}

		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)

			name := strings.TrimLeft(a, "-")
			if name == "" {
				continue
			}
			// Handle -flag=value / --flag=value.
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
			} else if needsValue[name] {
				// For value flags, consume the next arg as the value even if it begins with "-".
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("flag %s requires a value", a)
				}
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
			continue
		}

		// Non-flag token -> action.
		if !haveAction {
			action = a
			haveAction = true
			continue
		}
		return "", nil, fmt.Errorf("unexpected argument %q", a)
	}

	if !haveAction {
		return "", nil, fmt.Errorf("missing action")
	}
	return action, flagArgs, nil
}
