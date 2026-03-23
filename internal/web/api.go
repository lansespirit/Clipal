package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/logger"
	"github.com/lansespirit/Clipal/internal/proxy"
)

var startTime = time.Now()

const (
	providerNameMaxLen = 64
)

// API handles all API requests for the management interface
type API struct {
	configDir string
	version   string
	runtime   *proxy.Router
	configMu  sync.Mutex
}

// NewAPI creates a new API handler
func NewAPI(configDir, version string, runtime *proxy.Router) *API {
	return &API{
		configDir: configDir,
		version:   version,
		runtime:   runtime,
	}
}

func (a *API) reloadRuntimeProviderConfigs() error {
	if a.runtime == nil {
		return nil
	}
	return a.runtime.ReloadProviderConfigs()
}

// HandleGetGlobalConfig returns the current global configuration
func (a *API) HandleGetGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	writeJSON(w, toGlobalConfigResponse(cfg.Global))
}

// HandleUpdateGlobalConfig updates the global configuration
func (a *API) HandleUpdateGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GlobalConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Load current config
	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	// Update global config
	cfg.Global.ListenAddr = req.ListenAddr
	cfg.Global.Port = req.Port
	cfg.Global.LogLevel = config.LogLevel(req.LogLevel)
	cfg.Global.ReactivateAfter = req.ReactivateAfter
	cfg.Global.UpstreamIdleTimeout = req.UpstreamIdleTimeout
	// Backwards compatible: if the UI/client doesn't send this (older versions),
	// keep the current/default value instead of overwriting with empty.
	if strings.TrimSpace(req.ResponseHeaderTimeout) != "" {
		cfg.Global.ResponseHeaderTimeout = req.ResponseHeaderTimeout
	}
	cfg.Global.MaxRequestBody = req.MaxRequestBodyBytes
	cfg.Global.LogDir = req.LogDir
	cfg.Global.LogRetentionDays = req.LogRetentionDays
	cfg.Global.LogStdout = req.LogStdout
	cfg.Global.Notifications.Enabled = req.Notifications.Enabled
	cfg.Global.Notifications.MinLevel = config.LogLevel(req.Notifications.MinLevel)
	cfg.Global.Notifications.ProviderSwitch = req.Notifications.ProviderSwitch
	cfg.Global.CircuitBreaker.FailureThreshold = req.CircuitBreaker.FailureThreshold
	cfg.Global.CircuitBreaker.SuccessThreshold = req.CircuitBreaker.SuccessThreshold
	cfg.Global.CircuitBreaker.OpenTimeout = req.CircuitBreaker.OpenTimeout
	cfg.Global.CircuitBreaker.HalfOpenMaxInFlight = req.CircuitBreaker.HalfOpenMaxInFlight

	if !a.saveGlobalConfigOrWriteError(w, cfg) {
		return
	}

	logger.Info("global configuration updated via web interface")
	writeJSON(w, SuccessResponse{Message: "configuration updated successfully"})
}

// HandleGetProviders returns providers for a specific client
func (a *API) HandleGetProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType := extractClientType(r.URL.EscapedPath())
	if clientType == "" {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, toProviderResponses(cc.Providers))
}

func (a *API) HandleGetClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType := extractClientTypeFromClientConfigPath(r.URL.EscapedPath())
	if clientType == "" {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, ClientConfigResponse{
		Mode:           string(cc.Mode),
		PinnedProvider: cc.PinnedProvider,
	})
}

func (a *API) HandleUpdateClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType := extractClientTypeFromClientConfigPath(r.URL.EscapedPath())
	if clientType == "" {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}

	var req ClientConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	mode := config.ClientMode(strings.ToLower(strings.TrimSpace(req.Mode)))
	pin := strings.TrimSpace(req.PinnedProvider)

	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	cc.Mode = mode
	cc.PinnedProvider = pin
	if !a.saveClientConfigOrWriteError(w, clientType, cfg) {
		return
	}

	logger.Info("client configuration updated for %s via web interface", clientType)
	writeJSON(w, SuccessResponse{Message: "configuration updated successfully"})
}

// HandleAddProvider adds a new provider for a specific client
func (a *API) HandleAddProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType := extractClientType(r.URL.EscapedPath())
	if clientType == "" {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}

	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(req.Name)
	baseURL := strings.TrimSpace(req.BaseURL)
	keys, err := normalizeProviderKeys(req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate provider fields (name is used as a URL identifier).
	if err := validateProviderName(name); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := validateBaseURL(baseURL); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(keys) == 0 {
		writeError(w, "api_key or api_keys is required", http.StatusBadRequest)
		return
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Enforce unique provider names within a client config (the name is used as the identifier in URLs).
	if providerNameExists(cc.Providers, name) {
		writeError(w, "provider name already exists", http.StatusConflict)
		return
	}

	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
		if priority < 1 {
			writeError(w, "priority must be >= 1", http.StatusBadRequest)
			return
		}
	}
	if priority == 0 {
		priority = nextProviderPriority(cc.Providers)
	}

	provider := config.Provider{
		Name:     name,
		BaseURL:  baseURL,
		Priority: priority,
		Enabled:  req.Enabled,
	}
	assignProviderKeys(&provider, keys)

	cc.Providers = append(cc.Providers, provider)
	if !a.saveClientConfigOrWriteError(w, clientType, cfg) {
		return
	}

	logger.Info("provider %s added to %s via web interface", name, clientType)
	writeJSON(w, SuccessResponse{Message: "provider added successfully"})
}

// HandleUpdateProvider updates an existing provider
func (a *API) HandleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType, providerName := extractClientAndProvider(r.URL.EscapedPath())
	if clientType == "" || providerName == "" {
		writeError(w, "invalid client type or provider name", http.StatusBadRequest)
		return
	}

	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Normalize/validate optional fields.
	if strings.TrimSpace(req.Name) != "" {
		req.Name = strings.TrimSpace(req.Name)
		if err := validateProviderName(req.Name); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if strings.TrimSpace(req.BaseURL) != "" {
		req.BaseURL = strings.TrimSpace(req.BaseURL)
		if _, err := validateBaseURL(req.BaseURL); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	keys, err := normalizeProviderKeys(req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Priority != nil {
		if *req.Priority < 1 {
			writeError(w, "priority must be >= 1", http.StatusBadRequest)
			return
		}
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Reject renames that collide with another provider in the same client config.
	if req.Name != "" && req.Name != providerName {
		if providerNameExists(cc.Providers, req.Name) {
			writeError(w, "provider name already exists", http.StatusConflict)
			return
		}
	}

	updated := updateProviderInList(cc.Providers, providerName, req, keys)
	if updated {
		if !a.saveClientConfigOrWriteError(w, clientType, cfg) {
			return
		}
	}

	if !updated {
		writeError(w, "provider not found", http.StatusNotFound)
		return
	}

	logger.Info("provider %s updated in %s via web interface", providerName, clientType)
	writeJSON(w, SuccessResponse{Message: "provider updated successfully"})
}

// HandleDeleteProvider deletes a provider
func (a *API) HandleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType, providerName := extractClientAndProvider(r.URL.EscapedPath())
	if clientType == "" || providerName == "" {
		writeError(w, "invalid client type or provider name", http.StatusBadRequest)
		return
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	var deleted bool
	cc.Providers, deleted = deleteProviderFromList(cc.Providers, providerName)
	if deleted {
		if !a.saveClientConfigOrWriteError(w, clientType, cfg) {
			return
		}
	}

	if !deleted {
		writeError(w, "provider not found", http.StatusNotFound)
		return
	}

	logger.Info("provider %s deleted from %s via web interface", providerName, clientType)
	writeJSON(w, SuccessResponse{Message: "provider deleted successfully"})
}

// HandleReorderProviders reorders providers based on the provided list
func (a *API) HandleReorderProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType := extractClientType(r.URL.EscapedPath())
	if clientType == "" {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}

	var req ReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	cc, err := getClientConfigRef(cfg, clientType)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	reordered, err := reorderProviders(cc.Providers, req.Providers)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	cc.Providers = reordered
	if !a.saveClientConfigOrWriteError(w, clientType, cfg) {
		return
	}

	logger.Info("providers reordered for %s via web interface", clientType)
	writeJSON(w, SuccessResponse{Message: "providers reordered successfully"})
}

// HandleGetStatus returns the current system status
func (a *API) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg *config.Config
	if a.runtime != nil {
		cfg = a.runtime.ConfigSnapshot()
	}
	if cfg == nil {
		cfg = a.loadConfigOrWriteError(w)
		if cfg == nil {
			return
		}
	}

	var snap proxy.RouterRuntimeSnapshot
	if a.runtime != nil {
		snap = a.runtime.RuntimeSnapshot()
	}

	status := StatusResponse{
		Version:   a.version,
		Uptime:    time.Since(startTime).String(),
		ConfigDir: a.configDir,
		Clients:   make(map[string]ClientStatus),
	}

	status.Clients["claude-code"] = buildClientStatus(cfg.ClaudeCode, cfg.ClaudeCode.Providers, snap.Clients[proxy.ClientClaudeCode])
	status.Clients["codex"] = buildClientStatus(cfg.Codex, cfg.Codex.Providers, snap.Clients[proxy.ClientCodex])
	status.Clients["gemini"] = buildClientStatus(cfg.Gemini, cfg.Gemini.Providers, snap.Clients[proxy.ClientGemini])

	writeJSON(w, status)
}

// HandleExportConfig exports all configuration as JSON
func (a *API) HandleExportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}

	exportData := ExportConfigResponse{
		Global:     toGlobalConfigResponse(cfg.Global),
		ClaudeCode: toClientConfigExport(cfg.ClaudeCode),
		Codex:      toClientConfigExport(cfg.Codex),
		Gemini:     toClientConfigExport(cfg.Gemini),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=clipal-config.json")
	_ = json.NewEncoder(w).Encode(exportData)
}

// Helper functions

func buildClientStatus(cc config.ClientConfig, providers []config.Provider, rt proxy.ClientRuntimeSnapshot) ClientStatus {
	enabled := config.GetEnabledProviders(cc)
	enabledNames := make([]string, 0, len(enabled))
	for _, p := range enabled {
		enabledNames = append(enabledNames, p.Name)
	}

	mode := strings.TrimSpace(string(cc.Mode))
	if mode == "" {
		mode = string(config.ClientModeAuto)
	}
	pin := strings.TrimSpace(cc.PinnedProvider)

	current := strings.TrimSpace(rt.CurrentProvider)
	if mode == string(config.ClientModeManual) && pin != "" {
		// Manual mode is intended to be "sticky" to the pinned provider. Use the
		// config value so UI updates immediately after pinning (before the runtime
		// reload watcher swaps proxies).
		current = pin
	} else if current == "" {
		current = getFirstEnabledProvider(enabled)
	}

	currentProviders := make(map[string]string, len(rt.CurrentProviders)+1)
	for scope, provider := range rt.CurrentProviders {
		scope = strings.TrimSpace(scope)
		provider = strings.TrimSpace(provider)
		if scope == "" || provider == "" {
			continue
		}
		currentProviders[scope] = provider
	}
	if current != "" {
		currentProviders["default"] = current
	}

	// Map runtime per-provider info by name (runtime only contains enabled providers).
	rtByName := make(map[string]proxy.ProviderRuntimeSnapshot, len(rt.Providers))
	for _, p := range rt.Providers {
		rtByName[p.Name] = p
	}

	outProviders := make([]ProviderStatus, 0, len(providers))
	now := time.Now()
	for _, p := range providers {
		enabled := p.IsEnabled()
		ps := ProviderStatus{
			Name:              p.Name,
			Priority:          p.Priority,
			Enabled:           enabled,
			KeyCount:          p.KeyCount(),
			AvailableKeyCount: p.KeyCount(),
		}
		if !enabled {
			ps.SkipReason = "disabled"
			view := proxy.DescribeProviderAvailability(p.Name, enabled, proxy.ProviderRuntimeSnapshot{Name: p.Name})
			ps.State = view.State
			ps.Label = view.Label
			ps.Detail = view.Detail
			outProviders = append(outProviders, ps)
			continue
		}
		if rtSnap, ok := rtByName[p.Name]; ok {
			if rtSnap.KeyCount > 0 {
				ps.KeyCount = rtSnap.KeyCount
			}
			ps.AvailableKeyCount = rtSnap.AvailableKeyCount
			if ps.SkipReason == "" && rtSnap.KeyCount > 0 && rtSnap.AvailableKeyCount == 0 {
				ps.SkipReason = "keys_exhausted"
			}
			if !rtSnap.DeactivatedUntil.IsZero() && now.Before(rtSnap.DeactivatedUntil) {
				ps.SkipReason = "deactivated"
				ps.DeactivatedReason = rtSnap.DeactivatedReason
				ps.DeactivatedMessage = rtSnap.DeactivatedMessage
				ps.DeactivatedIn = time.Until(rtSnap.DeactivatedUntil).Truncate(time.Second).String()
			}
			ps.CircuitState = rtSnap.CircuitState
			if ps.SkipReason == "" && rtSnap.CircuitState == "open" && rtSnap.CircuitOpenIn > 0 {
				ps.SkipReason = "circuit_open"
				ps.CircuitOpenIn = rtSnap.CircuitOpenIn.Truncate(time.Second).String()
			}
			view := proxy.DescribeProviderAvailability(p.Name, enabled, rtSnap)
			ps.State = view.State
			ps.Label = view.Label
			ps.Detail = view.Detail
		} else {
			view := proxy.DescribeProviderAvailability(p.Name, enabled, proxy.ProviderRuntimeSnapshot{Name: p.Name})
			ps.State = view.State
			ps.Label = view.Label
			ps.Detail = view.Detail
		}
		outProviders = append(outProviders, ps)
	}

	var lastSwitch *ProviderSwitchStatus
	if rt.LastSwitch != nil && !rt.LastSwitch.At.IsZero() {
		view := proxy.DescribeProviderSwitch(rt.LastSwitch.From, rt.LastSwitch.To, rt.LastSwitch.Reason, rt.LastSwitch.Status)
		lastSwitch = &ProviderSwitchStatus{
			At:     rt.LastSwitch.At.Format(time.RFC3339),
			From:   rt.LastSwitch.From,
			To:     rt.LastSwitch.To,
			Reason: rt.LastSwitch.Reason,
			Status: rt.LastSwitch.Status,
			Label:  view.Label,
			Detail: view.Detail,
		}
	}
	var lastRequest *RequestOutcomeStatus
	if rt.LastRequest != nil && !rt.LastRequest.At.IsZero() {
		view := proxy.DescribeRequestOutcome(*rt.LastRequest)
		lastRequest = &RequestOutcomeStatus{
			At:         rt.LastRequest.At.Format(time.RFC3339),
			Provider:   rt.LastRequest.Provider,
			Status:     rt.LastRequest.Status,
			Delivery:   rt.LastRequest.Delivery,
			Protocol:   rt.LastRequest.Protocol,
			Capability: userVisibleCapability(rt.LastRequest.Capability),
			Cause:      rt.LastRequest.Cause,
			Bytes:      rt.LastRequest.Bytes,
			Result:     view.Result,
			Label:      view.Label,
			Detail:     view.Detail,
		}
	}

	return ClientStatus{
		Mode:           mode,
		PinnedProvider: pin,

		ProviderCount:    len(providers),
		EnabledProviders: enabledNames,
		CurrentProvider:  current,
		CurrentProviders: currentProviders,

		LastSwitch:  lastSwitch,
		LastRequest: lastRequest,
		Providers:   outProviders,
	}
}

func userVisibleCapability(capability string) string {
	switch capability {
	case string(proxy.CapabilityClaudeCountTokens), string(proxy.CapabilityGeminiCountTokens):
		return ""
	default:
		return capability
	}
}

func extractClientType(path string) string {
	// Extract client type from path like /api/providers/claude-code
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "providers" {
		return parts[2]
	}
	return ""
}

func extractClientTypeFromClientConfigPath(path string) string {
	// Extract client type from path like /api/client-config/claude-code
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "client-config" {
		return parts[2]
	}
	return ""
}

func getClientConfigRef(cfg *config.Config, clientType string) (*config.ClientConfig, error) {
	switch clientType {
	case "claude-code":
		return &cfg.ClaudeCode, nil
	case "codex":
		return &cfg.Codex, nil
	case "gemini":
		return &cfg.Gemini, nil
	default:
		return nil, fmt.Errorf("unknown client type")
	}
}

func providerNameExists(providers []config.Provider, name string) bool {
	for _, p := range providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

func nextProviderPriority(providers []config.Provider) int {
	maxPriority := 0
	for _, p := range providers {
		if p.Priority > maxPriority {
			maxPriority = p.Priority
		}
	}
	return maxPriority + 1
}

func normalizeProviderKeys(req ProviderRequest) ([]string, error) {
	apiKey := strings.TrimSpace(req.APIKey)
	keys := make([]string, 0, len(req.APIKeys)+1)
	seen := make(map[string]struct{}, len(req.APIKeys)+1)
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
	if apiKey != "" && len(req.APIKeys) > 0 {
		return nil, fmt.Errorf("api_key and api_keys cannot both be set")
	}
	if apiKey != "" {
		appendKey(apiKey)
	}
	for _, key := range req.APIKeys {
		appendKey(key)
	}
	return keys, nil
}

func assignProviderKeys(provider *config.Provider, keys []string) {
	if provider == nil {
		return
	}
	provider.APIKey = ""
	provider.APIKeys = nil
	switch len(keys) {
	case 0:
		return
	case 1:
		provider.APIKey = keys[0]
	default:
		provider.APIKeys = append([]string(nil), keys...)
	}
}

func extractClientAndProvider(path string) (string, string) {
	// Extract from path like /api/providers/claude-code/provider-name
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "providers" {
		// net/http populates r.URL.Path in decoded form, but be defensive if callers
		// use EscapedPath/RawPath or if we receive encoded segments.
		name, err := url.PathUnescape(parts[3])
		if err != nil {
			name = parts[3]
		}
		return parts[2], name
	}
	return "", ""
}

func updateProviderInList(providers []config.Provider, name string, req ProviderRequest, keys []string) bool {
	for i := range providers {
		if providers[i].Name == name {
			if req.Name != "" {
				providers[i].Name = req.Name
			}
			if req.BaseURL != "" {
				providers[i].BaseURL = req.BaseURL
			}
			if req.APIKey != "" || len(req.APIKeys) > 0 {
				assignProviderKeys(&providers[i], keys)
			}
			if req.Priority != nil {
				providers[i].Priority = *req.Priority
			}
			if req.Enabled != nil {
				providers[i].Enabled = req.Enabled
			}
			return true
		}
	}
	return false
}

func deleteProviderFromList(providers []config.Provider, name string) ([]config.Provider, bool) {
	for i, p := range providers {
		if p.Name == name {
			return append(providers[:i], providers[i+1:]...), true
		}
	}
	return providers, false
}

func reorderProviders(providers []config.Provider, order []string) ([]config.Provider, error) {
	// Create a map for quick lookup (and to detect duplicates).
	providerMap := make(map[string]config.Provider, len(providers))
	for _, p := range providers {
		if _, exists := providerMap[p.Name]; exists {
			return nil, fmt.Errorf("duplicate provider name: %s", p.Name)
		}
		providerMap[p.Name] = p
	}

	seen := make(map[string]struct{}, len(order))
	reordered := make([]config.Provider, 0, len(providers))

	// Add providers in the requested order first.
	for _, name := range order {
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("duplicate provider in reorder list: %s", name)
		}
		seen[name] = struct{}{}
		p, exists := providerMap[name]
		if !exists {
			return nil, fmt.Errorf("unknown provider in reorder list: %s", name)
		}
		reordered = append(reordered, p)
	}

	// Append any providers that were not mentioned, preserving their current order.
	for _, p := range providers {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		reordered = append(reordered, p)
	}

	// Normalize priorities to a continuous range so ordering is deterministic.
	for i := range reordered {
		reordered[i].Priority = i + 1
	}

	return reordered, nil
}

func getFirstEnabledProvider(providers []config.Provider) string {
	if len(providers) > 0 {
		return providers[0].Name
	}
	return ""
}

func validateProviderName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.Contains(name, "\\") {
		return fmt.Errorf("invalid provider name %q: backslash (\\\\) is not allowed", name)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("invalid provider name %q: slash (/) is not allowed", name)
	}
	if len(name) > providerNameMaxLen {
		return fmt.Errorf("invalid provider name %q: too long (max %d)", name, providerNameMaxLen)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if i == 0 && !isAlphaNum {
			return fmt.Errorf("invalid provider name %q: must start with a letter or number (allowed: letters, numbers, '.', '_', '-')", name)
		}
		if isAlphaNum || c == '.' || c == '_' || c == '-' {
			continue
		}
		return fmt.Errorf("invalid provider name %q: allowed characters are letters, numbers, '.', '_', '-' (no spaces)", name)
	}
	return nil
}

func validateBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("base_url is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base_url %q: %v", raw, err)
	}
	// If the scheme is missing, treat the value as a host and default to https.
	if parsed.Scheme == "" {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return "", fmt.Errorf("invalid base_url %q: %v", raw, err)
		}
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid base_url %q: host is empty", raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		// ok
	default:
		return "", fmt.Errorf("invalid base_url %q: scheme must be http or https", raw)
	}
	if parsed.RawQuery != "" {
		return "", fmt.Errorf("invalid base_url %q: query is not allowed", raw)
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("invalid base_url %q: fragment is not allowed", raw)
	}
	return raw, nil
}

func atomicWriteFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, ".clipal-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	success := false
	defer func() {
		_ = f.Close()
		if !success {
			_ = os.Remove(tmp)
		}
	}()

	if err := f.Chmod(perm); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	success = true
	return nil
}
