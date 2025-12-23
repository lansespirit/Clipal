package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/logger"
	"github.com/lansespirit/Clipal/internal/proxy"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Parse command line flags
	configDir := flag.String("config-dir", "", "Configuration directory (default: ~/.clipal)")
	listenAddr := flag.String("listen-addr", "", "Override listen address from config (default: 127.0.0.1)")
	port := flag.Int("port", 0, "Override port from config")
	logLevel := flag.String("log-level", "", "Override log level (debug/info/warn/error)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("clipal %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

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

	// Set log level
	logger.SetLevel(cfg.Global.LogLevel)
	if err := configureFileLogging(cfgDir, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "log file setup failed: %v\n", err)
	}

	// Create and start the router
	router := proxy.NewRouter(cfg)

	// Handle shutdown signals
	errCh := make(chan error, 1)
	go func() {
		errCh <- router.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("received signal %s, shutting down...", sig.String())
		if err := router.Stop(); err != nil {
			logger.Warn("graceful shutdown failed: %v", err)
		}
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped with error: %v", err)
			os.Exit(1)
		}
	}

	logger.Info("clipal stopped")
}

func configureFileLogging(cfgDir string, cfg *config.Config) error {
	logDir := strings.TrimSpace(cfg.Global.LogDir)
	if logDir == "" {
		logDir = filepath.Join(cfgDir, "logs")
	}

	retention := cfg.Global.LogRetentionDays
	if retention <= 0 {
		retention = 7
	}

	w, err := logger.NewRotatingFileWriter(logDir, "clipal", retention)
	if err != nil {
		return err
	}

	// Keep the file writer alive for the duration of the process.
	// Closing on exit is optional; the OS will release the fd.
	if cfg.Global.LogStdout == nil || *cfg.Global.LogStdout {
		logger.SetOutput(io.MultiWriter(os.Stdout, w))
	} else {
		logger.SetOutput(w)
	}
	return nil
}
