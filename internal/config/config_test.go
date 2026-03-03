package config

import "testing"

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
