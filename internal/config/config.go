package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LogLevel represents the log level
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// GlobalConfig represents the global configuration
type GlobalConfig struct {
	ListenAddr      string   `yaml:"listen_addr"`
	Port            int      `yaml:"port"`
	LogLevel        LogLevel `yaml:"log_level"`
	ReactivateAfter string   `yaml:"reactivate_after"`
}

// Provider represents an API provider configuration
type Provider struct {
	Name     string `yaml:"name"`
	BaseURL  string `yaml:"base_url"`
	APIKey   string `yaml:"api_key"`
	Priority int    `yaml:"priority"`
	Enabled  *bool  `yaml:"enabled,omitempty"`
}

// IsEnabled returns whether the provider is enabled (default true)
func (p *Provider) IsEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// ClientConfig represents a client-specific configuration
type ClientConfig struct {
	Providers []Provider `yaml:"providers"`
}

// Config represents the complete application configuration
type Config struct {
	Global     GlobalConfig
	ClaudeCode ClientConfig
	Codex      ClientConfig
	Gemini     ClientConfig
	configDir  string
}

// DefaultGlobalConfig returns the default global configuration
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		ListenAddr:      "127.0.0.1",
		Port:            3333,
		LogLevel:        LogLevelInfo,
		ReactivateAfter: "1h",
	}
}

// Load loads the configuration from the specified directory
func Load(configDir string) (*Config, error) {
	cfg := &Config{
		Global:    DefaultGlobalConfig(),
		configDir: configDir,
	}

	// Load global config
	globalPath := filepath.Join(configDir, "config.yaml")
	if err := loadYAML(globalPath, &cfg.Global); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	// Apply defaults if not set
	if cfg.Global.ListenAddr == "" {
		cfg.Global.ListenAddr = "127.0.0.1"
	}
	if cfg.Global.Port == 0 {
		cfg.Global.Port = 3333
	}
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = LogLevelInfo
	}
	if cfg.Global.ReactivateAfter == "" {
		cfg.Global.ReactivateAfter = "1h"
	}

	// Load client configs
	claudeCodePath := filepath.Join(configDir, "claude-code.yaml")
	if err := loadYAML(claudeCodePath, &cfg.ClaudeCode); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load claude-code config: %w", err)
	}

	codexPath := filepath.Join(configDir, "codex.yaml")
	if err := loadYAML(codexPath, &cfg.Codex); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load codex config: %w", err)
	}

	geminiPath := filepath.Join(configDir, "gemini.yaml")
	if err := loadYAML(geminiPath, &cfg.Gemini); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load gemini config: %w", err)
	}

	// Sort providers by priority
	sortProviders(cfg.ClaudeCode.Providers)
	sortProviders(cfg.Codex.Providers)
	sortProviders(cfg.Gemini.Providers)

	return cfg, nil
}

// loadYAML loads a YAML file into the target struct
func loadYAML(path string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, target)
}

// sortProviders sorts providers by priority (ascending)
func sortProviders(providers []Provider) {
	// Stable sort so providers with the same priority keep YAML order.
	sort.SliceStable(providers, func(i, j int) bool {
		return providers[i].Priority < providers[j].Priority
	})
}

// GetEnabledProviders returns only enabled providers for a client config
func GetEnabledProviders(cc ClientConfig) []Provider {
	var enabled []Provider
	for _, p := range cc.Providers {
		if p.IsEnabled() {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetConfigDir returns the default config directory
func GetConfigDir() string {
	if dir := os.Getenv("CLIPAL_CONFIG_DIR"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".clipal")
	}
	return ".clipal"
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Global.ListenAddr) == "" {
		return fmt.Errorf("listen_addr cannot be empty")
	}
	if c.Global.Port < 1 || c.Global.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Global.Port)
	}

	switch c.Global.LogLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		// valid
	default:
		return fmt.Errorf("invalid log level: %s", c.Global.LogLevel)
	}

	d, err := time.ParseDuration(c.Global.ReactivateAfter)
	if err != nil || d < 0 {
		return fmt.Errorf("invalid reactivate_after: %s", c.Global.ReactivateAfter)
	}

	// Validate providers
	if err := validateProviders("claude-code", c.ClaudeCode.Providers); err != nil {
		return err
	}
	if err := validateProviders("codex", c.Codex.Providers); err != nil {
		return err
	}
	if err := validateProviders("gemini", c.Gemini.Providers); err != nil {
		return err
	}

	return nil
}

func (c *Config) ConfigDir() string {
	return c.configDir
}

func validateProviders(clientName string, providers []Provider) error {
	for i, p := range providers {
		if p.Name == "" {
			return fmt.Errorf("%s provider %d: name is required", clientName, i+1)
		}
		if p.BaseURL == "" {
			return fmt.Errorf("%s provider %s: base_url is required", clientName, p.Name)
		}
		if p.APIKey == "" {
			return fmt.Errorf("%s provider %s: api_key is required", clientName, p.Name)
		}
	}
	return nil
}
