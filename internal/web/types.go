package web

// Keep request/response DTOs in the web package so we don't leak internal config
// structs directly over HTTP. This prevents accidental JSON field-name mismatches
// and lets us redact sensitive fields like API keys.

import (
	"github.com/lansespirit/Clipal/internal/config"
	integrationpkg "github.com/lansespirit/Clipal/internal/integration"
)

// GlobalConfigRequest represents a request to update global configuration
type GlobalConfigRequest struct {
	ListenAddr            string                      `json:"listen_addr"`
	Port                  int                         `json:"port"`
	LogLevel              string                      `json:"log_level"`
	ReactivateAfter       string                      `json:"reactivate_after"`
	UpstreamIdleTimeout   string                      `json:"upstream_idle_timeout"`
	ResponseHeaderTimeout string                      `json:"response_header_timeout"`
	MaxRequestBodyBytes   int64                       `json:"max_request_body_bytes"`
	LogDir                string                      `json:"log_dir"`
	LogRetentionDays      int                         `json:"log_retention_days"`
	LogStdout             *bool                       `json:"log_stdout"`
	Notifications         NotificationsConfigRequest  `json:"notifications"`
	CircuitBreaker        CircuitBreakerConfigRequest `json:"circuit_breaker"`
}

type NotificationsConfigRequest struct {
	Enabled        bool   `json:"enabled"`
	MinLevel       string `json:"min_level"`
	ProviderSwitch *bool  `json:"provider_switch"`
}

type CircuitBreakerConfigRequest struct {
	FailureThreshold    int    `json:"failure_threshold"`
	SuccessThreshold    int    `json:"success_threshold"`
	OpenTimeout         string `json:"open_timeout"`
	HalfOpenMaxInFlight int    `json:"half_open_max_inflight"`
}

// GlobalConfigResponse represents the global configuration returned to the UI.
type GlobalConfigResponse struct {
	ListenAddr            string                       `json:"listen_addr"`
	Port                  int                          `json:"port"`
	LogLevel              string                       `json:"log_level"`
	ReactivateAfter       string                       `json:"reactivate_after"`
	UpstreamIdleTimeout   string                       `json:"upstream_idle_timeout"`
	ResponseHeaderTimeout string                       `json:"response_header_timeout"`
	MaxRequestBodyBytes   int64                        `json:"max_request_body_bytes"`
	LogDir                string                       `json:"log_dir"`
	LogRetentionDays      int                          `json:"log_retention_days"`
	LogStdout             bool                         `json:"log_stdout"`
	Notifications         NotificationsConfigResponse  `json:"notifications"`
	CircuitBreaker        CircuitBreakerConfigResponse `json:"circuit_breaker"`
}

type NotificationsConfigResponse struct {
	Enabled        bool   `json:"enabled"`
	MinLevel       string `json:"min_level"`
	ProviderSwitch bool   `json:"provider_switch"`
}

type CircuitBreakerConfigResponse struct {
	FailureThreshold    int    `json:"failure_threshold"`
	SuccessThreshold    int    `json:"success_threshold"`
	OpenTimeout         string `json:"open_timeout"`
	HalfOpenMaxInFlight int    `json:"half_open_max_inflight"`
}

type ClientConfigRequest struct {
	Mode           string `json:"mode"`
	PinnedProvider string `json:"pinned_provider"`
}

type ClientConfigResponse struct {
	Mode           string `json:"mode"`
	PinnedProvider string `json:"pinned_provider"`
}

// ProviderRequest represents a request to create or update a provider
type ProviderRequest struct {
	Name    string   `json:"name"`
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key,omitempty"`
	APIKeys []string `json:"api_keys,omitempty"`
	// Priority is 1-based. Omit to keep existing value (on updates) or to
	// auto-assign the next priority (on create).
	Priority *int  `json:"priority,omitempty"`
	Enabled  *bool `json:"enabled,omitempty"`
}

// ProviderResponse is returned for provider listings (never includes api_key).
type ProviderResponse struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
	KeyCount int    `json:"key_count"`
}

// ReorderRequest represents a request to reorder providers
type ReorderRequest struct {
	Providers []string `json:"providers"` // Array of provider names in desired order
}

// ExportConfigResponse represents the full configuration export.
// NOTE: This includes API keys and should only be used for local backup/migration.
type ExportConfigResponse struct {
	Global GlobalConfigResponse `json:"global"`
	Claude ClientConfigExport   `json:"claude"`
	OpenAI ClientConfigExport   `json:"openai"`
	Gemini ClientConfigExport   `json:"gemini"`
}

type ClientConfigExport struct {
	Mode           string           `json:"mode"`
	PinnedProvider string           `json:"pinned_provider"`
	Providers      []ProviderExport `json:"providers"`
}

type ProviderExport struct {
	Name     string   `json:"name"`
	BaseURL  string   `json:"base_url"`
	APIKey   string   `json:"api_key,omitempty"`
	APIKeys  []string `json:"api_keys,omitempty"`
	Priority int      `json:"priority"`
	Enabled  *bool    `json:"enabled,omitempty"`
}

// StatusResponse represents the system status
type StatusResponse struct {
	Version   string                  `json:"version"`
	Uptime    string                  `json:"uptime"`
	ConfigDir string                  `json:"config_dir"`
	Clients   map[string]ClientStatus `json:"clients"`
}

// ClientStatus represents the status of a client proxy
type ClientStatus struct {
	Mode           string `json:"mode"`
	PinnedProvider string `json:"pinned_provider,omitempty"`

	ProviderCount    int               `json:"provider_count"`
	EnabledProviders []string          `json:"enabled_providers"`
	CurrentProvider  string            `json:"current_provider"`
	CurrentProviders map[string]string `json:"current_providers,omitempty"`

	LastSwitch  *ProviderSwitchStatus `json:"last_switch,omitempty"`
	LastRequest *RequestOutcomeStatus `json:"last_request,omitempty"`
	Providers   []ProviderStatus      `json:"providers,omitempty"`
}

type ProviderSwitchStatus struct {
	At     string `json:"at"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
	Status int    `json:"status"`
	Label  string `json:"label,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type ProviderStatus struct {
	Name              string `json:"name"`
	Priority          int    `json:"priority"`
	Enabled           bool   `json:"enabled"`
	KeyCount          int    `json:"key_count"`
	AvailableKeyCount int    `json:"available_key_count,omitempty"`
	State             string `json:"state,omitempty"`
	Label             string `json:"label,omitempty"`
	Detail            string `json:"detail,omitempty"`

	SkipReason string `json:"skip_reason,omitempty"` // disabled | deactivated | circuit_open

	DeactivatedReason  string `json:"deactivated_reason,omitempty"`
	DeactivatedMessage string `json:"deactivated_message,omitempty"`
	DeactivatedIn      string `json:"deactivated_in,omitempty"`

	CircuitState  string `json:"circuit_state,omitempty"` // closed | open | half_open
	CircuitOpenIn string `json:"circuit_open_in,omitempty"`
}

type RequestOutcomeStatus struct {
	At         string `json:"at"`
	Provider   string `json:"provider"`
	Status     int    `json:"status"`
	Delivery   string `json:"delivery"`
	Protocol   string `json:"protocol"`
	Capability string `json:"capability,omitempty"`
	Cause      string `json:"cause,omitempty"`
	Bytes      int    `json:"bytes"`
	Result     string `json:"result,omitempty"`
	Label      string `json:"label,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
	Reason string `json:"reason"`
}

// SuccessResponse represents a success response
type SuccessResponse struct {
	Message string `json:"message"`
}

// ServiceActionRequest triggers an OS service action.
// Most fields are optional and only apply on some platforms.
type ServiceActionRequest struct {
	Force      bool   `json:"force"`
	StdoutPath string `json:"stdout_path,omitempty"` // macOS: launchd StandardOutPath
	StderrPath string `json:"stderr_path,omitempty"` // macOS: launchd StandardErrorPath
}

// ServiceStatusResponse reports best-effort status for "clipal service status".
// Output and Error (if any) are intended for display/debugging.
type ServiceStatusResponse struct {
	OS        string `json:"os"`
	Supported bool   `json:"supported"`
	Installed bool   `json:"installed"`
	OK        bool   `json:"ok"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`

	// UI helpers (e.g. Windows may require elevated install in some environments).
	InstallCommand string `json:"install_command,omitempty"`
	InstallHint    string `json:"install_hint,omitempty"`
}

type IntegrationResponse struct {
	Product         string `json:"product"`
	Name            string `json:"name"`
	State           string `json:"state"`
	TargetPath      string `json:"target_path"`
	BackupAvailable bool   `json:"backup_available"`
	Warning         string `json:"warning,omitempty"`
	CurrentContent  string `json:"current_content,omitempty"`
	PlannedContent  string `json:"planned_content,omitempty"`
}

type IntegrationActionResponse struct {
	Message string              `json:"message"`
	Product string              `json:"product"`
	Status  IntegrationResponse `json:"status"`
}

func boolPtrOrTrue(v *bool) bool {
	if v == nil {
		return true
	}
	return *v
}

func toGlobalConfigResponse(gc config.GlobalConfig) GlobalConfigResponse {
	return GlobalConfigResponse{
		ListenAddr:            gc.ListenAddr,
		Port:                  gc.Port,
		LogLevel:              string(gc.LogLevel),
		ReactivateAfter:       gc.ReactivateAfter,
		UpstreamIdleTimeout:   gc.UpstreamIdleTimeout,
		ResponseHeaderTimeout: gc.ResponseHeaderTimeout,
		MaxRequestBodyBytes:   gc.MaxRequestBody,
		LogDir:                gc.LogDir,
		LogRetentionDays:      gc.LogRetentionDays,
		LogStdout:             boolPtrOrTrue(gc.LogStdout),
		Notifications: NotificationsConfigResponse{
			Enabled:        gc.Notifications.Enabled,
			MinLevel:       string(gc.Notifications.MinLevel),
			ProviderSwitch: boolPtrOrTrue(gc.Notifications.ProviderSwitch),
		},
		CircuitBreaker: CircuitBreakerConfigResponse{
			FailureThreshold:    gc.CircuitBreaker.FailureThreshold,
			SuccessThreshold:    gc.CircuitBreaker.SuccessThreshold,
			OpenTimeout:         gc.CircuitBreaker.OpenTimeout,
			HalfOpenMaxInFlight: gc.CircuitBreaker.HalfOpenMaxInFlight,
		},
	}
}

func toProviderResponses(providers []config.Provider) []ProviderResponse {
	out := make([]ProviderResponse, 0, len(providers))
	for _, p := range providers {
		out = append(out, ProviderResponse{
			Name:     p.Name,
			BaseURL:  p.BaseURL,
			Priority: p.Priority,
			Enabled:  p.IsEnabled(),
			KeyCount: p.KeyCount(),
		})
	}
	return out
}

func toClientConfigExport(cc config.ClientConfig) ClientConfigExport {
	out := make([]ProviderExport, 0, len(cc.Providers))
	for _, p := range cc.Providers {
		out = append(out, ProviderExport{
			Name:     p.Name,
			BaseURL:  p.BaseURL,
			APIKey:   p.APIKey,
			APIKeys:  append([]string(nil), p.APIKeys...),
			Priority: p.Priority,
			Enabled:  p.Enabled,
		})
	}
	return ClientConfigExport{
		Mode:           string(cc.Mode),
		PinnedProvider: cc.PinnedProvider,
		Providers:      out,
	}
}

func toIntegrationResponse(product integrationpkg.ProductID, status integrationpkg.Status, preview integrationpkg.Preview) IntegrationResponse {
	return IntegrationResponse{
		Product:         string(product),
		Name:            integrationpkg.ProductName(product),
		State:           string(status.State),
		TargetPath:      status.TargetPath,
		BackupAvailable: status.BackupAvailable,
		Warning:         status.Warning,
		CurrentContent:  preview.CurrentContent,
		PlannedContent:  preview.PlannedContent,
	}
}
