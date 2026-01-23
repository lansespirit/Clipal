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
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/logger"
)

var startTime = time.Now()

const (
	providerNameMaxLen = 64
)

// API handles all API requests for the management interface
type API struct {
	configDir string
	version   string
}

// NewAPI creates a new API handler
func NewAPI(configDir, version string) *API {
	return &API{
		configDir: configDir,
		version:   version,
	}
}

// HandleGetGlobalConfig returns the current global configuration
func (a *API) HandleGetGlobalConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
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
	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	// Update global config
	cfg.Global.ListenAddr = req.ListenAddr
	cfg.Global.Port = req.Port
	cfg.Global.LogLevel = config.LogLevel(req.LogLevel)
	cfg.Global.ReactivateAfter = req.ReactivateAfter
	cfg.Global.UpstreamIdleTimeout = req.UpstreamIdleTimeout
	cfg.Global.MaxRequestBody = req.MaxRequestBodyBytes
	cfg.Global.LogDir = req.LogDir
	cfg.Global.LogRetentionDays = req.LogRetentionDays
	cfg.Global.LogStdout = req.LogStdout
	cfg.Global.Notifications.Enabled = req.Notifications.Enabled
	cfg.Global.Notifications.MinLevel = config.LogLevel(req.Notifications.MinLevel)
	cfg.Global.Notifications.ProviderSwitch = req.Notifications.ProviderSwitch
	cfg.Global.IgnoreCountTokensFailover = req.IgnoreCountTokensFailover

	// Validate
	if err := cfg.Validate(); err != nil {
		writeError(w, fmt.Sprintf("invalid configuration: %v", err), http.StatusBadRequest)
		return
	}

	// Save to file
	if err := a.saveGlobalConfig(cfg.Global); err != nil {
		writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
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

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	var providers []config.Provider
	switch clientType {
	case "claude-code":
		providers = cfg.ClaudeCode.Providers
	case "codex":
		providers = cfg.Codex.Providers
	case "gemini":
		providers = cfg.Gemini.Providers
	default:
		writeError(w, "unknown client type", http.StatusBadRequest)
		return
	}

	writeJSON(w, toProviderResponses(providers))
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
	apiKey := strings.TrimSpace(req.APIKey)

	// Validate provider fields (name is used as a URL identifier).
	if err := validateProviderName(name); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := validateBaseURL(baseURL); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if apiKey == "" {
		writeError(w, "api_key is required", http.StatusBadRequest)
		return
	}

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	// Enforce unique provider names within a client config (the name is used as the identifier in URLs).
	existing := getProvidersForClient(cfg, clientType)
	if providerNameExists(existing, name) {
		writeError(w, "provider name already exists", http.StatusConflict)
		return
	}

	priority := req.Priority
	if priority <= 0 {
		priority = nextPriority(existing)
	}

	provider := config.Provider{
		Name:     name,
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Priority: priority,
		Enabled:  req.Enabled,
	}

	// Add provider to appropriate client config
	switch clientType {
	case "claude-code":
		cfg.ClaudeCode.Providers = append(cfg.ClaudeCode.Providers, provider)
		if err := a.saveClientConfig(clientType, cfg.ClaudeCode); err != nil {
			writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
	case "codex":
		cfg.Codex.Providers = append(cfg.Codex.Providers, provider)
		if err := a.saveClientConfig(clientType, cfg.Codex); err != nil {
			writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
	case "gemini":
		cfg.Gemini.Providers = append(cfg.Gemini.Providers, provider)
		if err := a.saveClientConfig(clientType, cfg.Gemini); err != nil {
			writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
	default:
		writeError(w, "unknown client type", http.StatusBadRequest)
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
	if strings.TrimSpace(req.APIKey) != "" {
		req.APIKey = strings.TrimSpace(req.APIKey)
	}
	if req.Priority < 0 {
		writeError(w, "priority must be >= 0", http.StatusBadRequest)
		return
	}

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	// Reject renames that collide with another provider in the same client config.
	if req.Name != "" && req.Name != providerName {
		existing := getProvidersForClient(cfg, clientType)
		if providerNameExists(existing, req.Name) {
			writeError(w, "provider name already exists", http.StatusConflict)
			return
		}
	}

	// Update provider in appropriate client config
	var updated bool
	switch clientType {
	case "claude-code":
		updated = updateProviderInList(cfg.ClaudeCode.Providers, providerName, req)
		if updated {
			if err := a.saveClientConfig(clientType, cfg.ClaudeCode); err != nil {
				writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
				return
			}
		}
	case "codex":
		updated = updateProviderInList(cfg.Codex.Providers, providerName, req)
		if updated {
			if err := a.saveClientConfig(clientType, cfg.Codex); err != nil {
				writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
				return
			}
		}
	case "gemini":
		updated = updateProviderInList(cfg.Gemini.Providers, providerName, req)
		if updated {
			if err := a.saveClientConfig(clientType, cfg.Gemini); err != nil {
				writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
				return
			}
		}
	default:
		writeError(w, "unknown client type", http.StatusBadRequest)
		return
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

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete provider from appropriate client config
	var deleted bool
	switch clientType {
	case "claude-code":
		cfg.ClaudeCode.Providers, deleted = deleteProviderFromList(cfg.ClaudeCode.Providers, providerName)
		if deleted {
			if err := a.saveClientConfig(clientType, cfg.ClaudeCode); err != nil {
				writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
				return
			}
		}
	case "codex":
		cfg.Codex.Providers, deleted = deleteProviderFromList(cfg.Codex.Providers, providerName)
		if deleted {
			if err := a.saveClientConfig(clientType, cfg.Codex); err != nil {
				writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
				return
			}
		}
	case "gemini":
		cfg.Gemini.Providers, deleted = deleteProviderFromList(cfg.Gemini.Providers, providerName)
		if deleted {
			if err := a.saveClientConfig(clientType, cfg.Gemini); err != nil {
				writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
				return
			}
		}
	default:
		writeError(w, "unknown client type", http.StatusBadRequest)
		return
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

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	// Reorder providers
	switch clientType {
	case "claude-code":
		reordered, err := reorderProviders(cfg.ClaudeCode.Providers, req.Providers)
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.ClaudeCode.Providers = reordered
		if err := a.saveClientConfig(clientType, cfg.ClaudeCode); err != nil {
			writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
	case "codex":
		reordered, err := reorderProviders(cfg.Codex.Providers, req.Providers)
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.Codex.Providers = reordered
		if err := a.saveClientConfig(clientType, cfg.Codex); err != nil {
			writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
	case "gemini":
		reordered, err := reorderProviders(cfg.Gemini.Providers, req.Providers)
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		cfg.Gemini.Providers = reordered
		if err := a.saveClientConfig(clientType, cfg.Gemini); err != nil {
			writeError(w, fmt.Sprintf("failed to save config: %v", err), http.StatusInternalServerError)
			return
		}
	default:
		writeError(w, "unknown client type", http.StatusBadRequest)
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

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	status := StatusResponse{
		Version:   a.version,
		Uptime:    time.Since(startTime).String(),
		ConfigDir: a.configDir,
		Clients:   make(map[string]ClientStatus),
	}

	// Claude Code status
	if len(cfg.ClaudeCode.Providers) > 0 {
		enabled := config.GetEnabledProviders(cfg.ClaudeCode)
		var enabledNames []string
		for _, p := range enabled {
			enabledNames = append(enabledNames, p.Name)
		}
		status.Clients["claude-code"] = ClientStatus{
			ProviderCount:    len(cfg.ClaudeCode.Providers),
			EnabledProviders: enabledNames,
			CurrentProvider:  getFirstEnabledProvider(enabled),
		}
	}

	// Codex status
	if len(cfg.Codex.Providers) > 0 {
		enabled := config.GetEnabledProviders(cfg.Codex)
		var enabledNames []string
		for _, p := range enabled {
			enabledNames = append(enabledNames, p.Name)
		}
		status.Clients["codex"] = ClientStatus{
			ProviderCount:    len(cfg.Codex.Providers),
			EnabledProviders: enabledNames,
			CurrentProvider:  getFirstEnabledProvider(enabled),
		}
	}

	// Gemini status
	if len(cfg.Gemini.Providers) > 0 {
		enabled := config.GetEnabledProviders(cfg.Gemini)
		var enabledNames []string
		for _, p := range enabled {
			enabledNames = append(enabledNames, p.Name)
		}
		status.Clients["gemini"] = ClientStatus{
			ProviderCount:    len(cfg.Gemini.Providers),
			EnabledProviders: enabledNames,
			CurrentProvider:  getFirstEnabledProvider(enabled),
		}
	}

	writeJSON(w, status)
}

// HandleExportConfig exports all configuration as JSON
func (a *API) HandleExportConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := config.Load(a.configDir)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
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
	json.NewEncoder(w).Encode(exportData)
}

// Helper functions

func (a *API) saveGlobalConfig(global config.GlobalConfig) error {
	path := filepath.Join(a.configDir, "config.yaml")
	data := formatGlobalConfigYAML(global)
	return atomicWriteFile(path, data, 0600)
}

func (a *API) saveClientConfig(clientType string, clientCfg config.ClientConfig) error {
	filename := fmt.Sprintf("%s.yaml", clientType)
	path := filepath.Join(a.configDir, filename)
	data := formatClientConfigYAML(clientType, clientCfg)
	return atomicWriteFile(path, data, 0600)
}

func extractClientType(path string) string {
	// Extract client type from path like /api/providers/claude-code
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "providers" {
		return parts[2]
	}
	return ""
}

func getProvidersForClient(cfg *config.Config, clientType string) []config.Provider {
	switch clientType {
	case "claude-code":
		return cfg.ClaudeCode.Providers
	case "codex":
		return cfg.Codex.Providers
	case "gemini":
		return cfg.Gemini.Providers
	default:
		return nil
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

func nextPriority(providers []config.Provider) int {
	max := 0
	for _, p := range providers {
		if p.Priority > max {
			max = p.Priority
		}
	}
	return max + 1
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

func updateProviderInList(providers []config.Provider, name string, req ProviderRequest) bool {
	for i := range providers {
		if providers[i].Name == name {
			if req.Name != "" {
				providers[i].Name = req.Name
			}
			if req.BaseURL != "" {
				providers[i].BaseURL = req.BaseURL
			}
			if req.APIKey != "" {
				providers[i].APIKey = req.APIKey
			}
			if req.Priority > 0 {
				providers[i].Priority = req.Priority
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

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
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
