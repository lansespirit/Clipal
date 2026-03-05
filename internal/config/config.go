package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func ptr[T any](v T) *T { return &v }

// LogLevel represents the log level
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type NotificationsConfig struct {
	Enabled        bool     `yaml:"enabled"`
	MinLevel       LogLevel `yaml:"min_level"`
	ProviderSwitch *bool    `yaml:"provider_switch"`
}

type CircuitBreakerConfig struct {
	// FailureThreshold opens the circuit after this many consecutive failures.
	FailureThreshold int `yaml:"failure_threshold"`
	// SuccessThreshold closes the circuit after this many consecutive successes in half-open.
	SuccessThreshold int `yaml:"success_threshold"`
	// OpenTimeout controls how long the circuit remains open before transitioning to half-open.
	OpenTimeout string `yaml:"open_timeout"`
	// HalfOpenMaxInFlight limits concurrent probe requests in half-open state.
	HalfOpenMaxInFlight int `yaml:"half_open_max_inflight"`
}

// GlobalConfig represents the global configuration
type GlobalConfig struct {
	ListenAddr      string   `yaml:"listen_addr"`
	Port            int      `yaml:"port"`
	LogLevel        LogLevel `yaml:"log_level"`
	ReactivateAfter string   `yaml:"reactivate_after"`
	// UpstreamIdleTimeout cancels an upstream attempt if no response body bytes are received
	// for the duration (useful for SSE streams that may hang after headers).
	// Set to "0" to disable.
	UpstreamIdleTimeout string               `yaml:"upstream_idle_timeout"`
	MaxRequestBody      int64                `yaml:"max_request_body_bytes"`
	LogDir              string               `yaml:"log_dir"`
	LogRetentionDays    int                  `yaml:"log_retention_days"`
	LogStdout           *bool                `yaml:"log_stdout"`
	Notifications       NotificationsConfig  `yaml:"notifications"`
	CircuitBreaker      CircuitBreakerConfig `yaml:"circuit_breaker"`
	// IgnoreCountTokensFailover disables provider switching for Claude Code
	// /v1/messages/count_tokens requests, which helps keep context cache warm.
	IgnoreCountTokensFailover bool `yaml:"ignore_count_tokens_failover"`
}

type ClientMode string

const (
	ClientModeAuto   ClientMode = "auto"
	ClientModeManual ClientMode = "manual"
)

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
	Mode           ClientMode `yaml:"mode"`
	PinnedProvider string     `yaml:"pinned_provider"`
	Providers      []Provider `yaml:"providers"`
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
		ListenAddr:          "127.0.0.1",
		Port:                3333,
		LogLevel:            LogLevelInfo,
		ReactivateAfter:     "1h",
		UpstreamIdleTimeout: "3m",
		// Default body limit: 32 MiB. clipal buffers request bodies to support retries,
		// so a hard cap prevents unbounded memory usage.
		MaxRequestBody:   32 * 1024 * 1024,
		LogDir:           "",
		LogRetentionDays: 7,
		LogStdout:        ptr(true),
		Notifications: NotificationsConfig{
			Enabled:        false,
			MinLevel:       LogLevelError,
			ProviderSwitch: ptr(true),
		},
		CircuitBreaker: CircuitBreakerConfig{
			// Conservative defaults: only trips on sustained failures.
			FailureThreshold:    4,
			SuccessThreshold:    2,
			OpenTimeout:         "60s",
			HalfOpenMaxInFlight: 1,
		},
		// Keep existing behavior by default.
		IgnoreCountTokensFailover: false,
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

	applyClientDefaults(&cfg.ClaudeCode)
	applyClientDefaults(&cfg.Codex)
	applyClientDefaults(&cfg.Gemini)

	// Sort providers by priority
	sortProviders(cfg.ClaudeCode.Providers)
	sortProviders(cfg.Codex.Providers)
	sortProviders(cfg.Gemini.Providers)

	return cfg, nil
}

// loadYAML loads a YAML file into the target struct
func loadYAML(path string, target interface{}) error {
	warnIfPermissiveConfig(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(target)
}

// sortProviders sorts providers by priority (ascending)
func sortProviders(providers []Provider) {
	// Stable sort so providers with the same priority keep YAML order.
	sort.SliceStable(providers, func(i, j int) bool {
		return providers[i].Priority < providers[j].Priority
	})
}

func applyClientDefaults(cc *ClientConfig) {
	if cc == nil {
		return
	}
	// Normalize mode to lowercase to tolerate "Auto", "MANUAL", etc.
	cc.Mode = ClientMode(strings.ToLower(strings.TrimSpace(string(cc.Mode))))
	if cc.Mode == "" {
		cc.Mode = ClientModeAuto
	}

	// Priority is 1-based. Older configs may omit it (decoded as 0).
	for i := range cc.Providers {
		if cc.Providers[i].Priority == 0 {
			cc.Providers[i].Priority = 1
		}
	}
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
	if c.Global.MaxRequestBody < 1 {
		return fmt.Errorf("invalid max_request_body_bytes: %d", c.Global.MaxRequestBody)
	}

	switch c.Global.LogLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		// valid
	default:
		return fmt.Errorf("invalid log level: %s", c.Global.LogLevel)
	}

	if c.Global.Notifications.MinLevel != "" {
		switch c.Global.Notifications.MinLevel {
		case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
			// valid
		default:
			return fmt.Errorf("invalid notifications.min_level: %s", c.Global.Notifications.MinLevel)
		}
	}

	d, err := time.ParseDuration(c.Global.ReactivateAfter)
	if err != nil || d < 0 {
		return fmt.Errorf("invalid reactivate_after: %s", c.Global.ReactivateAfter)
	}
	idle, err := time.ParseDuration(c.Global.UpstreamIdleTimeout)
	if err != nil || idle < 0 {
		return fmt.Errorf("invalid upstream_idle_timeout: %s", c.Global.UpstreamIdleTimeout)
	}
	if c.Global.LogRetentionDays < 0 {
		return fmt.Errorf("invalid log_retention_days: %d", c.Global.LogRetentionDays)
	}

	// Circuit breaker:
	// - failure_threshold == 0 disables the circuit breaker entirely.
	// - failure_threshold  > 0 enables it and requires the other fields to be valid.
	if c.Global.CircuitBreaker.FailureThreshold < 0 {
		return fmt.Errorf("invalid circuit_breaker.failure_threshold: %d", c.Global.CircuitBreaker.FailureThreshold)
	}
	if c.Global.CircuitBreaker.SuccessThreshold < 0 {
		return fmt.Errorf("invalid circuit_breaker.success_threshold: %d", c.Global.CircuitBreaker.SuccessThreshold)
	}
	if c.Global.CircuitBreaker.HalfOpenMaxInFlight < 0 {
		return fmt.Errorf("invalid circuit_breaker.half_open_max_inflight: %d", c.Global.CircuitBreaker.HalfOpenMaxInFlight)
	}
	if c.Global.CircuitBreaker.FailureThreshold > 0 {
		if c.Global.CircuitBreaker.SuccessThreshold <= 0 {
			return fmt.Errorf("invalid circuit_breaker.success_threshold: %d", c.Global.CircuitBreaker.SuccessThreshold)
		}
		if c.Global.CircuitBreaker.HalfOpenMaxInFlight <= 0 {
			return fmt.Errorf("invalid circuit_breaker.half_open_max_inflight: %d", c.Global.CircuitBreaker.HalfOpenMaxInFlight)
		}
		cbTimeout, err := time.ParseDuration(c.Global.CircuitBreaker.OpenTimeout)
		if err != nil || cbTimeout <= 0 {
			return fmt.Errorf("invalid circuit_breaker.open_timeout: %s", c.Global.CircuitBreaker.OpenTimeout)
		}
	}

	if err := validateClientConfig("claude-code", c.ClaudeCode); err != nil {
		return err
	}
	if err := validateClientConfig("codex", c.Codex); err != nil {
		return err
	}
	if err := validateClientConfig("gemini", c.Gemini); err != nil {
		return err
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

func validateClientConfig(name string, cc ClientConfig) error {
	switch cc.Mode {
	case ClientModeAuto, ClientModeManual:
		// ok
	default:
		return fmt.Errorf("%s: invalid mode: %q (expected %q or %q)", name, cc.Mode, ClientModeAuto, ClientModeManual)
	}
	if cc.Mode == ClientModeManual {
		pin := strings.TrimSpace(cc.PinnedProvider)
		if pin == "" {
			return fmt.Errorf("%s: pinned_provider is required when mode=manual", name)
		}
		found := false
		for _, p := range cc.Providers {
			if p.Name == pin {
				if !p.IsEnabled() {
					return fmt.Errorf("%s: pinned_provider %q is disabled", name, pin)
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: pinned_provider %q not found", name, pin)
		}
	}
	return nil
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
		if p.Priority < 1 {
			return fmt.Errorf("%s provider %s: priority must be >= 1", clientName, p.Name)
		}
	}
	return nil
}
