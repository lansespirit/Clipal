package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidate_CircuitBreakerDisabled_AllowsInvalidCBFields(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Global:     DefaultGlobalConfig(),
		ClaudeCode: ClientConfig{Mode: ClientModeAuto},
		Codex:      ClientConfig{Mode: ClientModeAuto},
		Gemini:     ClientConfig{Mode: ClientModeAuto},
	}

	// Disable circuit breaker explicitly.
	cfg.Global.CircuitBreaker.FailureThreshold = 0
	// Intentionally make other CB fields invalid; validation should skip them when disabled.
	cfg.Global.CircuitBreaker.OpenTimeout = "not-a-duration"
	cfg.Global.CircuitBreaker.SuccessThreshold = 0
	cfg.Global.CircuitBreaker.HalfOpenMaxInFlight = 0

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected config to validate with circuit breaker disabled, got: %v", err)
	}
}

func TestExampleConfig_DoesNotExposeDeprecatedIgnoreCountTokensFailover(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "examples", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	if strings.Contains(string(data), "ignore_count_tokens_failover") {
		t.Fatalf("example config should not expose deprecated ignore_count_tokens_failover")
	}
}

func TestValidate_CircuitBreakerEnabled_StillValidatesCBFields(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Global:     DefaultGlobalConfig(),
		ClaudeCode: ClientConfig{Mode: ClientModeAuto},
		Codex:      ClientConfig{Mode: ClientModeAuto},
		Gemini:     ClientConfig{Mode: ClientModeAuto},
	}

	cfg.Global.CircuitBreaker.FailureThreshold = 1
	cfg.Global.CircuitBreaker.OpenTimeout = "nope"

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for invalid circuit_breaker.open_timeout when enabled")
	}
}

func TestValidate_CircuitBreakerFailureThresholdNegative_IsRejected(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Global:     DefaultGlobalConfig(),
		ClaudeCode: ClientConfig{Mode: ClientModeAuto},
		Codex:      ClientConfig{Mode: ClientModeAuto},
		Gemini:     ClientConfig{Mode: ClientModeAuto},
	}

	cfg.Global.CircuitBreaker.FailureThreshold = -1
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for negative circuit_breaker.failure_threshold")
	}
}

func TestValidate_ManualMode_RequiresEnabledPinnedProvider(t *testing.T) {
	t.Parallel()

	provider := Provider{
		Name:     "p1",
		BaseURL:  "http://example.com",
		APIKey:   "key",
		Priority: 1,
		Enabled:  ptr(false),
	}

	cfg := &Config{
		Global: DefaultGlobalConfig(),
		ClaudeCode: ClientConfig{
			Mode:           ClientModeManual,
			PinnedProvider: "p1",
			Providers:      []Provider{provider},
		},
		Codex:  ClientConfig{Mode: ClientModeAuto},
		Gemini: ClientConfig{Mode: ClientModeAuto},
	}

	// 1. Fails if provider is disabled
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for disabled pinned_provider")
	}

	// 2. Fails if pinned_provider is missing from list
	cfg.ClaudeCode.PinnedProvider = "ghost"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for non-existent pinned_provider")
	}

	// 3. Fails if pinned_provider is empty
	cfg.ClaudeCode.PinnedProvider = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for empty pinned_provider")
	}

	// 4. Passes if enabled and exists
	cfg.ClaudeCode.PinnedProvider = "p1"
	cfg.ClaudeCode.Providers[0].Enabled = ptr(true)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected validation success, got: %v", err)
	}
}

func TestValidate_ProviderAPIKeys_MultiKeySupported(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Global: DefaultGlobalConfig(),
		Codex: ClientConfig{
			Mode: ClientModeAuto,
			Providers: []Provider{
				{
					Name:     "p1",
					BaseURL:  "https://example.com",
					APIKeys:  []string{" key1 ", "key2", "key1"},
					Priority: 1,
				},
			},
		},
		ClaudeCode: ClientConfig{Mode: ClientModeAuto},
		Gemini:     ClientConfig{Mode: ClientModeAuto},
	}

	applyClientDefaults(&cfg.Codex)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected validation success, got: %v", err)
	}
	if got := cfg.Codex.Providers[0].KeyCount(); got != 2 {
		t.Fatalf("key count: got %d want %d", got, 2)
	}
	if cfg.Codex.Providers[0].APIKey != "" {
		t.Fatalf("expected canonical multi-key provider to use api_keys, got api_key=%q", cfg.Codex.Providers[0].APIKey)
	}
}

func TestGlobalConfigRuntimeDurations(t *testing.T) {
	t.Parallel()

	cfg := DefaultGlobalConfig()
	got, err := cfg.RuntimeDurations()
	if err != nil {
		t.Fatalf("RuntimeDurations: %v", err)
	}
	if got.ReactivateAfter != time.Hour {
		t.Fatalf("ReactivateAfter = %s, want %s", got.ReactivateAfter, time.Hour)
	}
	if got.UpstreamIdleTimeout != 3*time.Minute {
		t.Fatalf("UpstreamIdleTimeout = %s, want %s", got.UpstreamIdleTimeout, 3*time.Minute)
	}
	if got.ResponseHeaderTimeout != 2*time.Minute {
		t.Fatalf("ResponseHeaderTimeout = %s, want %s", got.ResponseHeaderTimeout, 2*time.Minute)
	}
}

func TestCircuitBreakerOpenTimeoutDuration(t *testing.T) {
	t.Parallel()

	d, err := CircuitBreakerConfig{OpenTimeout: "90s"}.OpenTimeoutDuration()
	if err != nil {
		t.Fatalf("OpenTimeoutDuration: %v", err)
	}
	if d != 90*time.Second {
		t.Fatalf("OpenTimeoutDuration = %s, want %s", d, 90*time.Second)
	}
}

func TestValidate_ProviderAPIKeys_RejectsMixedForms(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Global: DefaultGlobalConfig(),
		Codex: ClientConfig{
			Mode: ClientModeAuto,
			Providers: []Provider{
				{
					Name:     "p1",
					BaseURL:  "https://example.com",
					APIKey:   "key1",
					APIKeys:  []string{"key2"},
					Priority: 1,
				},
			},
		},
		ClaudeCode: ClientConfig{Mode: ClientModeAuto},
		Gemini:     ClientConfig{Mode: ClientModeAuto},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error when api_key and api_keys are both set")
	}
}

func TestValidate_RejectsDuplicateProviderNamesPerClient(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Global: DefaultGlobalConfig(),
		Codex: ClientConfig{
			Mode: ClientModeAuto,
			Providers: []Provider{
				{Name: "p1", BaseURL: "https://one.example", APIKey: "key1", Priority: 1},
				{Name: "p1", BaseURL: "https://two.example", APIKey: "key2", Priority: 2},
			},
		},
		ClaudeCode: ClientConfig{Mode: ClientModeAuto},
		Gemini:     ClientConfig{Mode: ClientModeAuto},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `duplicate provider name "p1"`) {
		t.Fatalf("err = %v, want duplicate provider name error", err)
	}
}

func TestGlobalConfigRuntimeDurations_ErrorsAndBoundaryValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*GlobalConfig)
		wantErr string
		wantDur RuntimeDurations
	}{
		{
			name: "MalformedReactivateAfter",
			mutate: func(gc *GlobalConfig) {
				gc.ReactivateAfter = "nope"
			},
			wantErr: "invalid reactivate_after",
		},
		{
			name: "MalformedUpstreamIdleTimeout",
			mutate: func(gc *GlobalConfig) {
				gc.UpstreamIdleTimeout = "bad"
			},
			wantErr: "invalid upstream_idle_timeout",
		},
		{
			name: "MalformedResponseHeaderTimeout",
			mutate: func(gc *GlobalConfig) {
				gc.ResponseHeaderTimeout = "bad"
			},
			wantErr: "invalid response_header_timeout",
		},
		{
			name: "NegativeDurations",
			mutate: func(gc *GlobalConfig) {
				gc.ReactivateAfter = "-1h"
				gc.UpstreamIdleTimeout = "-3m"
				gc.ResponseHeaderTimeout = "-2m"
			},
			wantDur: RuntimeDurations{
				ReactivateAfter:       -1 * time.Hour,
				UpstreamIdleTimeout:   -3 * time.Minute,
				ResponseHeaderTimeout: -2 * time.Minute,
			},
		},
		{
			name: "ZeroDurations",
			mutate: func(gc *GlobalConfig) {
				gc.ReactivateAfter = "0"
				gc.UpstreamIdleTimeout = "0"
				gc.ResponseHeaderTimeout = "0"
			},
			wantDur: RuntimeDurations{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gc := DefaultGlobalConfig()
			tt.mutate(&gc)
			got, err := gc.RuntimeDurations()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RuntimeDurations: %v", err)
			}
			if got != tt.wantDur {
				t.Fatalf("durations = %#v, want %#v", got, tt.wantDur)
			}
		})
	}
}

func TestDefaultGlobalConfig_IncludesRoutingDefaults(t *testing.T) {
	t.Parallel()

	cfg := DefaultGlobalConfig()

	if !cfg.Routing.StickySessions.Enabled {
		t.Fatalf("sticky_sessions.enabled: got false want true")
	}
	if got := cfg.Routing.StickySessions.ExplicitTTL; got != "30m" {
		t.Fatalf("sticky_sessions.explicit_ttl: got %q want %q", got, "30m")
	}
	if got := cfg.Routing.StickySessions.CacheHintTTL; got != "10m" {
		t.Fatalf("sticky_sessions.cache_hint_ttl: got %q want %q", got, "10m")
	}
	if got := cfg.Routing.StickySessions.DynamicFeatureTTL; got != "10m" {
		t.Fatalf("sticky_sessions.dynamic_feature_ttl: got %q want %q", got, "10m")
	}
	if got := cfg.Routing.StickySessions.DynamicFeatureCapacity; got != 1024 {
		t.Fatalf("sticky_sessions.dynamic_feature_capacity: got %d want %d", got, 1024)
	}
	if got := cfg.Routing.StickySessions.ResponseLookupTTL; got != "15m" {
		t.Fatalf("sticky_sessions.response_lookup_ttl: got %q want %q", got, "15m")
	}
	if !cfg.Routing.BusyBackpressure.Enabled {
		t.Fatalf("busy_backpressure.enabled: got false want true")
	}
	if got := cfg.Routing.BusyBackpressure.RetryDelays; len(got) != 2 || got[0] != "5s" || got[1] != "10s" {
		t.Fatalf("busy_backpressure.retry_delays: got %#v want [5s 10s]", got)
	}
	if got := cfg.Routing.BusyBackpressure.ProbeMaxInFlight; got != 1 {
		t.Fatalf("busy_backpressure.probe_max_inflight: got %d want %d", got, 1)
	}
	if got := cfg.Routing.BusyBackpressure.ShortRetryAfterMax; got != "3s" {
		t.Fatalf("busy_backpressure.short_retry_after_max: got %q want %q", got, "3s")
	}
	if got := cfg.Routing.BusyBackpressure.MaxInlineWait; got != "8s" {
		t.Fatalf("busy_backpressure.max_inline_wait: got %q want %q", got, "8s")
	}
}

func TestValidate_RoutingConfigRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "empty retry delays",
			mutate: func(cfg *Config) {
				cfg.Global.Routing.BusyBackpressure.RetryDelays = nil
			},
			wantErr: "routing.busy_backpressure.retry_delays",
		},
		{
			name: "bad explicit ttl",
			mutate: func(cfg *Config) {
				cfg.Global.Routing.StickySessions.ExplicitTTL = "bad"
			},
			wantErr: "routing.sticky_sessions.explicit_ttl",
		},
		{
			name: "non positive dynamic feature capacity",
			mutate: func(cfg *Config) {
				cfg.Global.Routing.StickySessions.DynamicFeatureCapacity = 0
			},
			wantErr: "routing.sticky_sessions.dynamic_feature_capacity",
		},
		{
			name: "negative probe inflight",
			mutate: func(cfg *Config) {
				cfg.Global.Routing.BusyBackpressure.ProbeMaxInFlight = -1
			},
			wantErr: "routing.busy_backpressure.probe_max_inflight",
		},
		{
			name: "bad max inline wait",
			mutate: func(cfg *Config) {
				cfg.Global.Routing.BusyBackpressure.MaxInlineWait = "later"
			},
			wantErr: "routing.busy_backpressure.max_inline_wait",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				Global:     DefaultGlobalConfig(),
				ClaudeCode: ClientConfig{Mode: ClientModeAuto},
				Codex:      ClientConfig{Mode: ClientModeAuto},
				Gemini:     ClientConfig{Mode: ClientModeAuto},
			}
			tt.mutate(cfg)

			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestCircuitBreakerOpenTimeoutDuration_InvalidString(t *testing.T) {
	t.Parallel()

	if _, err := (CircuitBreakerConfig{OpenTimeout: "bad"}).OpenTimeoutDuration(); err == nil {
		t.Fatalf("expected invalid duration error")
	}
}

func TestLoad_RejectsUnknownYAMLFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("listen_addr: 127.0.0.1\nport: 3333\nunknown_field: true\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("Load error = %v, want unknown_field failure", err)
	}
}

func TestGetConfigDir_RespectsEnvironmentOverride(t *testing.T) {
	const want = "/tmp/clipal-config-override"
	t.Setenv("CLIPAL_CONFIG_DIR", want)
	if got := GetConfigDir(); got != want {
		t.Fatalf("GetConfigDir = %q, want %q", got, want)
	}
}
