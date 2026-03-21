package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/lansespirit/Clipal/internal/app"
	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/selfupdate"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "update":
			runUpdate(os.Args[2:])
			return
		case "status":
			runStatus(os.Args[2:])
			return
		case "service":
			runService(os.Args[2:])
			return
		case "__apply-update":
			runApplyUpdate(os.Args[2:])
			return
		}
	}

	// Parse command line flags
	configDir := flag.String("config-dir", "", "Configuration directory (default: ~/.clipal)")
	listenAddr := flag.String("listen-addr", "", "Override listen address from config (default: 127.0.0.1)")
	port := flag.Int("port", 0, "Override port from config")
	logLevel := flag.String("log-level", "", "Override log level (debug/info/warn/error)")
	detachConsole := flag.Bool("detach-console", false, "Windows: detach console window (used by Task Scheduler)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("clipal %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	maybeDetachConsole(*detachConsole)

	// Determine config directory
	cfgDir := *configDir
	if cfgDir == "" {
		cfgDir = config.GetConfigDir()
	}

	// Load configuration
	cfg, err := config.Load(cfgDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Apply command line overrides
	if *listenAddr != "" {
		cfg.Global.ListenAddr = *listenAddr
	}
	if *port > 0 {
		cfg.Global.Port = *port
	}
	if *logLevel != "" {
		cfg.Global.LogLevel = config.LogLevel(*logLevel)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	application, err := app.New(cfgDir, cfg, app.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipal failed to initialize: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = application.Shutdown(context.Background()) }()

	// Handle shutdown signals
	errCh := make(chan error, 1)
	go func() {
		errCh <- application.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		application.LogSignalShutdown(sig.String())
		fmt.Fprintf(os.Stderr, "clipal: received signal %s, shutting down...\n", sig.String())
		if err := application.Shutdown(context.Background()); err != nil {
			application.LogShutdownFailure(err)
			fmt.Fprintf(os.Stderr, "clipal: graceful shutdown failed: %v\n", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			application.LogServerError(err)
			fmt.Fprintf(os.Stderr, "clipal: server stopped with error: %v\n", err)
			os.Exit(1)
		}
	}
	application.LogStopped()
}

func runUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	check := fs.Bool("check", false, "Check for updates only")
	force := fs.Bool("force", false, "Force update (allow reinstall/downgrade)")
	dryRun := fs.Bool("dry-run", false, "Show what would be downloaded and replaced")
	timeout := fs.Duration("timeout", 2*time.Minute, "Overall update timeout")
	relaunch := fs.Bool("relaunch", false, "Windows: relaunch clipal after updating")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	opts := selfupdate.Options{
		Check:    *check,
		Force:    *force,
		DryRun:   *dryRun,
		Timeout:  *timeout,
		Relaunch: *relaunch,
	}

	plan, needsOrUpdated, err := selfupdate.Update(context.Background(), version, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipal update failed: %v\n", err)
		os.Exit(1)
	}

	if *check {
		if needsOrUpdated {
			fmt.Printf("update available: %s -> %s\n", plan.CurrentVersion, plan.LatestVersion)
		} else {
			fmt.Printf("up to date: %s\n", plan.CurrentVersion)
		}
		return
	}

	if *dryRun {
		fmt.Printf("current: %s\nlatest: %s\n", plan.CurrentVersion, plan.LatestVersion)
		fmt.Printf("exe: %s\n", plan.ExecutablePath)
		fmt.Printf("asset: %s\nchecksums: %s\n", plan.BinaryAsset.Name, plan.ChecksumsAsset.Name)
		fmt.Printf("download: %s\n", plan.BinaryAsset.BrowserDownloadURL)
		return
	}

	if needsOrUpdated {
		if runtime.GOOS == "windows" {
			fmt.Printf("update scheduled: %s -> %s\n", plan.CurrentVersion, plan.LatestVersion)
			fmt.Printf("note: this process will exit so the updater can replace %s\n", plan.ExecutablePath)
			return
		}
		fmt.Printf("updated: %s -> %s\n", plan.CurrentVersion, plan.LatestVersion)
		return
	}
	fmt.Printf("up to date: %s\n", plan.CurrentVersion)
}

func runApplyUpdate(args []string) {
	fs := flag.NewFlagSet("__apply-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pid := fs.Int("pid", 0, "PID to wait for before replacing")
	src := fs.String("src", "", "Downloaded update binary path")
	dst := fs.String("dst", "", "Target executable path to replace")
	helper := fs.String("helper", "", "Helper executable path to delete after update")
	relaunch := fs.Bool("relaunch", false, "Relaunch updated binary after replacing")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	err := selfupdate.ApplyUpdateWindows(selfupdate.ApplyUpdateOptions{
		PID:      *pid,
		Src:      *src,
		Dst:      *dst,
		Helper:   *helper,
		Relaunch: *relaunch,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "clipal: apply update failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "clipal: update applied")
}
