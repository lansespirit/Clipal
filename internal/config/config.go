package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func ptr[T any](v T) *T { return &v }

const (
	defaultReactivateAfter       = time.Hour
	defaultUpstreamIdleTimeout   = 3 * time.Minute
	defaultResponseHeaderTimeout = 2 * time.Minute
	defaultCircuitOpenTimeout    = 60 * time.Second
)

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

type StickySessionsConfig struct {
	Enabled                bool   `yaml:"enabled"`
	ExplicitTTL            string `yaml:"explicit_ttl"`
	CacheHintTTL           string `yaml:"cache_hint_ttl"`
	DynamicFeatureTTL      string `yaml:"dynamic_feature_ttl"`
	DynamicFeatureCapacity int    `yaml:"dynamic_feature_capacity"`
	ResponseLookupTTL      string `yaml:"response_lookup_ttl"`
}

type BusyBackpressureConfig struct {
	Enabled            bool     `yaml:"enabled"`
	RetryDelays        []string `yaml:"retry_delays"`
	ProbeMaxInFlight   int      `yaml:"probe_max_inflight"`
	ShortRetryAfterMax string   `yaml:"short_retry_after_max"`
	MaxInlineWait      string   `yaml:"max_inline_wait"`
}

type RoutingConfig struct {
	StickySessions   StickySessionsConfig   `yaml:"sticky_sessions"`
	BusyBackpressure BusyBackpressureConfig `yaml:"busy_backpressure"`
}

// OpenTimeoutDuration parses the configured circuit breaker timeout.
func (c CircuitBreakerConfig) OpenTimeoutDuration() (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(c.OpenTimeout))
	if err != nil {
		return 0, fmt.Errorf("invalid circuit_breaker.open_timeout: %w", err)
	}
	return d, nil
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
	UpstreamIdleTimeout string `yaml:"upstream_idle_timeout"`
	// ResponseHeaderTimeout controls how long we wait for the upstream to return
	// response headers after the request is fully written. Set to "0" to disable.
	ResponseHeaderTimeout string               `yaml:"response_header_timeout"`
	MaxRequestBody        int64                `yaml:"max_request_body_bytes"`
	LogDir                string               `yaml:"log_dir"`
	LogRetentionDays      int                  `yaml:"log_retention_days"`
	LogStdout             *bool                `yaml:"log_stdout"`
	Notifications         NotificationsConfig  `yaml:"notifications"`
	CircuitBreaker        CircuitBreakerConfig `yaml:"circuit_breaker"`
	Routing               RoutingConfig        `yaml:"routing"`
	// Deprecated: retained only so older config.yaml files still load under
	// strict KnownFields decoding. Runtime no longer reads this field.
	IgnoreCountTokensFailover bool `yaml:"ignore_count_tokens_failover"`
}

// RuntimeDurations contains the parsed global timing values used by the proxy runtime.
type RuntimeDurations struct {
	ReactivateAfter       time.Duration
	UpstreamIdleTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
}

type ClientMode string

const (
	ClientModeAuto   ClientMode = "auto"
	ClientModeManual ClientMode = "manual"
)

// Provider represents an API provider configuration
type Provider struct {
	Name     string   `yaml:"name"`
	BaseURL  string   `yaml:"base_url"`
	APIKey   string   `yaml:"api_key,omitempty"`
	APIKeys  []string `yaml:"api_keys,omitempty"`
	Priority int      `yaml:"priority"`
	Enabled  *bool    `yaml:"enabled,omitempty"`
}

// IsEnabled returns whether the provider is enabled (default true)
func (p *Provider) IsEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// NormalizedAPIKeys returns the configured API keys with whitespace removed,
// empty entries dropped, and duplicates removed while preserving order.
func (p *Provider) NormalizedAPIKeys() []string {
	if p == nil {
		return nil
	}
	keys := make([]string, 0, len(p.APIKeys)+1)
	seen := make(map[string]struct{}, len(p.APIKeys)+1)
	appendKey := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		keys = append(keys, v)
	}
	if strings.TrimSpace(p.APIKey) != "" {
		appendKey(p.APIKey)
	}
	for _, key := range p.APIKeys {
		appendKey(key)
	}
	return keys
}

// PrimaryAPIKey returns the first normalized API key, or an empty string.
func (p *Provider) PrimaryAPIKey() string {
	keys := p.NormalizedAPIKeys()
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

// KeyCount returns the number of normalized API keys configured for the provider.
func (p *Provider) KeyCount() int {
	return len(p.NormalizedAPIKeys())
}

// ClientConfig represents a client-specific configuration
type ClientConfig struct {
	Mode           ClientMode `yaml:"mode"`
	PinnedProvider string     `yaml:"pinned_provider"`
	Providers      []Provider `yaml:"providers"`
}

// Config represents the complete application configuration
type Config struct {
	Global    GlobalConfig
	Claude    ClientConfig
	OpenAI    ClientConfig
	Gemini    ClientConfig
	configDir string
}

type clientConfigFileSpec struct {
	clientType  string
	currentName string
	legacyName  string
}

var clientConfigFileSpecs = []clientConfigFileSpec{
	{clientType: "claude", currentName: "claude.yaml", legacyName: "claude-code.yaml"},
	{clientType: "openai", currentName: "openai.yaml", legacyName: "codex.yaml"},
	{clientType: "gemini", currentName: "gemini.yaml", legacyName: ""},
}

func CanonicalClientType(clientType string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(clientType)) {
	case "claude", "claude-code", "claudecode":
		return "claude", true
	case "openai", "codex":
		return "openai", true
	case "gemini":
		return "gemini", true
	default:
		return "", false
	}
}

// DefaultGlobalConfig returns the default global configuration
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		ListenAddr:            "127.0.0.1",
		Port:                  3333,
		LogLevel:              LogLevelInfo,
		ReactivateAfter:       "1h",
		UpstreamIdleTimeout:   "3m",
		ResponseHeaderTimeout: "2m",
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
		Routing: RoutingConfig{
			StickySessions: StickySessionsConfig{
				Enabled:                true,
				ExplicitTTL:            "30m",
				CacheHintTTL:           "10m",
				DynamicFeatureTTL:      "10m",
				DynamicFeatureCapacity: 1024,
				ResponseLookupTTL:      "15m",
			},
			BusyBackpressure: BusyBackpressureConfig{
				Enabled:            true,
				RetryDelays:        []string{"5s", "10s"},
				ProbeMaxInFlight:   1,
				ShortRetryAfterMax: "3s",
				MaxInlineWait:      "8s",
			},
		},
	}
}

// DefaultRuntimeDurations returns the runtime timing defaults used by Clipal.
func DefaultRuntimeDurations() RuntimeDurations {
	return RuntimeDurations{
		ReactivateAfter:       defaultReactivateAfter,
		UpstreamIdleTimeout:   defaultUpstreamIdleTimeout,
		ResponseHeaderTimeout: defaultResponseHeaderTimeout,
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

	if err := migrateLegacyClientConfigFiles(configDir); err != nil {
		return nil, err
	}

	// Load client configs
	claudePath := filepath.Join(configDir, "claude.yaml")
	if err := loadYAML(claudePath, &cfg.Claude); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load claude config: %w", err)
	}

	openAIPath := filepath.Join(configDir, "openai.yaml")
	if err := loadYAML(openAIPath, &cfg.OpenAI); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load openai config: %w", err)
	}

	geminiPath := filepath.Join(configDir, "gemini.yaml")
	if err := loadYAML(geminiPath, &cfg.Gemini); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load gemini config: %w", err)
	}

	applyClientDefaults(&cfg.Claude)
	applyClientDefaults(&cfg.OpenAI)
	applyClientDefaults(&cfg.Gemini)

	// Sort providers by priority
	sortProviders(cfg.Claude.Providers)
	sortProviders(cfg.OpenAI.Providers)
	sortProviders(cfg.Gemini.Providers)

	return cfg, nil
}

func clientConfigFileSpecForType(clientType string) (clientConfigFileSpec, bool) {
	clientType, ok := CanonicalClientType(clientType)
	if !ok {
		return clientConfigFileSpec{}, false
	}
	for _, spec := range clientConfigFileSpecs {
		if spec.clientType == clientType {
			return spec, true
		}
	}
	return clientConfigFileSpec{}, false
}

func ClientConfigFilename(clientType string) (string, error) {
	spec, ok := clientConfigFileSpecForType(clientType)
	if !ok {
		return "", fmt.Errorf("unknown client type: %q", clientType)
	}
	return spec.currentName, nil
}

func WatchedConfigFilenames() []string {
	names := []string{"config.yaml"}
	seen := map[string]struct{}{"config.yaml": {}}
	for _, spec := range clientConfigFileSpecs {
		if spec.currentName != "" {
			if _, ok := seen[spec.currentName]; !ok {
				names = append(names, spec.currentName)
				seen[spec.currentName] = struct{}{}
			}
		}
	}
	return names
}

func migrateLegacyClientConfigFiles(configDir string) error {
	for _, spec := range clientConfigFileSpecs {
		if err := migrateLegacyClientConfigFile(configDir, spec); err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacyClientConfigFile(configDir string, spec clientConfigFileSpec) error {
	if spec.legacyName == "" || spec.currentName == spec.legacyName {
		return nil
	}

	currentPath := filepath.Join(configDir, spec.currentName)
	legacyPath := filepath.Join(configDir, spec.legacyName)

	currentInfo, currentErr := os.Stat(currentPath)
	legacyInfo, legacyErr := os.Stat(legacyPath)
	currentExists := currentErr == nil
	legacyExists := legacyErr == nil

	if currentErr != nil && !os.IsNotExist(currentErr) {
		return fmt.Errorf("failed to stat %s: %w", spec.currentName, currentErr)
	}
	if legacyErr != nil && !os.IsNotExist(legacyErr) {
		return fmt.Errorf("failed to stat %s: %w", spec.legacyName, legacyErr)
	}
	if currentExists && currentInfo.IsDir() {
		return fmt.Errorf("%s is a directory", spec.currentName)
	}
	if legacyExists && legacyInfo.IsDir() {
		return fmt.Errorf("%s is a directory", spec.legacyName)
	}

	switch {
	case !currentExists && !legacyExists:
		return nil
	case currentExists && !legacyExists:
		return nil
	case !currentExists && legacyExists:
		if err := os.Rename(legacyPath, currentPath); err != nil {
			return fmt.Errorf("failed to migrate %s to %s: %w", spec.legacyName, spec.currentName, err)
		}
		return nil
	default:
		same, err := clientConfigFilesEquivalent(currentPath, legacyPath)
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("both %s and %s exist with different content", spec.currentName, spec.legacyName)
		}
		if err := os.Remove(legacyPath); err != nil {
			return fmt.Errorf("failed to remove legacy config %s: %w", spec.legacyName, err)
		}
		return nil
	}
}

func clientConfigFilesEquivalent(currentPath string, legacyPath string) (bool, error) {
	var current ClientConfig
	if err := loadYAML(currentPath, &current); err != nil {
		return false, fmt.Errorf("failed to load %s: %w", filepath.Base(currentPath), err)
	}
	var legacy ClientConfig
	if err := loadYAML(legacyPath, &legacy); err != nil {
		return false, fmt.Errorf("failed to load %s: %w", filepath.Base(legacyPath), err)
	}
	applyClientDefaults(&current)
	applyClientDefaults(&legacy)
	return reflect.DeepEqual(current, legacy), nil
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
		cc.Providers[i].APIKey = strings.TrimSpace(cc.Providers[i].APIKey)
		cc.Providers[i].APIKeys = cc.Providers[i].NormalizedAPIKeys()
		if len(cc.Providers[i].APIKeys) == 1 {
			cc.Providers[i].APIKey = cc.Providers[i].APIKeys[0]
			cc.Providers[i].APIKeys = nil
		} else {
			cc.Providers[i].APIKey = ""
		}
	}
}

// RuntimeDurations parses the runtime timing values from global config.
func (g GlobalConfig) RuntimeDurations() (RuntimeDurations, error) {
	out := DefaultRuntimeDurations()

	reactivateAfter, err := time.ParseDuration(strings.TrimSpace(g.ReactivateAfter))
	if err != nil {
		return RuntimeDurations{}, fmt.Errorf("invalid reactivate_after: %w", err)
	}
	upstreamIdle, err := time.ParseDuration(strings.TrimSpace(g.UpstreamIdleTimeout))
	if err != nil {
		return RuntimeDurations{}, fmt.Errorf("invalid upstream_idle_timeout: %w", err)
	}
	responseHeaderTimeout, err := time.ParseDuration(strings.TrimSpace(g.ResponseHeaderTimeout))
	if err != nil {
		return RuntimeDurations{}, fmt.Errorf("invalid response_header_timeout: %w", err)
	}

	out.ReactivateAfter = reactivateAfter
	out.UpstreamIdleTimeout = upstreamIdle
	out.ResponseHeaderTimeout = responseHeaderTimeout
	return out, nil
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
	respHdr, err := time.ParseDuration(c.Global.ResponseHeaderTimeout)
	if err != nil || respHdr < 0 {
		return fmt.Errorf("invalid response_header_timeout: %s", c.Global.ResponseHeaderTimeout)
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

	if err := validateRoutingConfig(c.Global.Routing); err != nil {
		return err
	}

	if err := validateClientConfig("claude", c.Claude); err != nil {
		return err
	}
	if err := validateClientConfig("openai", c.OpenAI); err != nil {
		return err
	}
	if err := validateClientConfig("gemini", c.Gemini); err != nil {
		return err
	}

	// Validate providers
	if err := validateProviders("claude", c.Claude.Providers); err != nil {
		return err
	}
	if err := validateProviders("openai", c.OpenAI.Providers); err != nil {
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
	seenNames := make(map[string]int, len(providers))
	for i, p := range providers {
		if p.Name == "" {
			return fmt.Errorf("%s provider %d: name is required", clientName, i+1)
		}
		if firstIdx, ok := seenNames[p.Name]; ok {
			return fmt.Errorf("%s: duplicate provider name %q at positions %d and %d", clientName, p.Name, firstIdx+1, i+1)
		}
		seenNames[p.Name] = i
		if p.BaseURL == "" {
			return fmt.Errorf("%s provider %s: base_url is required", clientName, p.Name)
		}
		if strings.TrimSpace(p.APIKey) != "" && len(p.APIKeys) > 0 {
			return fmt.Errorf("%s provider %s: api_key and api_keys cannot both be set", clientName, p.Name)
		}
		if len(p.NormalizedAPIKeys()) == 0 {
			return fmt.Errorf("%s provider %s: api_key or api_keys is required", clientName, p.Name)
		}
		if p.Priority < 1 {
			return fmt.Errorf("%s provider %s: priority must be >= 1", clientName, p.Name)
		}
	}
	return nil
}

func validateRoutingConfig(rc RoutingConfig) error {
	if rc.StickySessions.Enabled {
		if err := validatePositiveDuration("routing.sticky_sessions.explicit_ttl", rc.StickySessions.ExplicitTTL); err != nil {
			return err
		}
		if err := validatePositiveDuration("routing.sticky_sessions.cache_hint_ttl", rc.StickySessions.CacheHintTTL); err != nil {
			return err
		}
		if err := validatePositiveDuration("routing.sticky_sessions.dynamic_feature_ttl", rc.StickySessions.DynamicFeatureTTL); err != nil {
			return err
		}
		if err := validatePositiveDuration("routing.sticky_sessions.response_lookup_ttl", rc.StickySessions.ResponseLookupTTL); err != nil {
			return err
		}
		if rc.StickySessions.DynamicFeatureCapacity <= 0 {
			return fmt.Errorf("invalid routing.sticky_sessions.dynamic_feature_capacity: %d", rc.StickySessions.DynamicFeatureCapacity)
		}
	}

	if rc.BusyBackpressure.Enabled {
		if len(rc.BusyBackpressure.RetryDelays) == 0 {
			return fmt.Errorf("invalid routing.busy_backpressure.retry_delays: empty")
		}
		for i, delay := range rc.BusyBackpressure.RetryDelays {
			if err := validatePositiveDuration(fmt.Sprintf("routing.busy_backpressure.retry_delays[%d]", i), delay); err != nil {
				return err
			}
		}
		if rc.BusyBackpressure.ProbeMaxInFlight < 0 {
			return fmt.Errorf("invalid routing.busy_backpressure.probe_max_inflight: %d", rc.BusyBackpressure.ProbeMaxInFlight)
		}
		if err := validatePositiveDuration("routing.busy_backpressure.short_retry_after_max", rc.BusyBackpressure.ShortRetryAfterMax); err != nil {
			return err
		}
		if err := validatePositiveDuration("routing.busy_backpressure.max_inline_wait", rc.BusyBackpressure.MaxInlineWait); err != nil {
			return err
		}
	}

	return nil
}

func validatePositiveDuration(field string, value string) error {
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || d <= 0 {
		return fmt.Errorf("invalid %s: %s", field, value)
	}
	return nil
}
