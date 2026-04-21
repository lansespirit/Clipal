package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
	"github.com/lansespirit/Clipal/internal/telemetry"
)

func (a *API) HandleListOAuthProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType, ok := config.CanonicalClientType(strings.TrimSpace(r.URL.Query().Get("client_type")))
	if !ok {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}

	descriptors := oauthpkg.SupportedProvidersForClient(clientType)
	resp := make([]OAuthProviderOptionResponse, 0, len(descriptors))
	for _, descriptor := range descriptors {
		resp = append(resp, OAuthProviderOptionResponse{Provider: descriptor.Provider})
	}
	writeJSON(w, resp)
}

func (a *API) HandleStartOAuthProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OAuthStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	clientType, ok := config.CanonicalClientType(strings.TrimSpace(req.ClientType))
	if !ok {
		writeError(w, "invalid client type", http.StatusBadRequest)
		return
	}
	provider := config.OAuthProvider(strings.ToLower(strings.TrimSpace(string(req.Provider))))
	if err := validateOAuthProviderForClient(clientType, provider); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	session, err := a.oauth.StartLogin(provider)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.setOAuthTargetClient(session.ID, clientType)
	writeJSON(w, OAuthStartResponse{
		SessionID: session.ID,
		Provider:  session.Provider,
		AuthURL:   session.AuthURL,
		ExpiresAt: formatTimeRFC3339(session.ExpiresAt),
	})
}

func (a *API) HandleGetOAuthSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := extractOAuthSessionID(r.URL.EscapedPath())
	if sessionID == "" {
		writeError(w, "invalid oauth session", http.StatusBadRequest)
		return
	}
	clientType, ok := a.getOAuthTargetClient(sessionID)
	if !ok {
		writeError(w, "oauth session not found", http.StatusNotFound)
		return
	}

	session, err := a.oauth.PollLogin(sessionID)
	if err != nil {
		writeError(w, err.Error(), http.StatusNotFound)
		return
	}
	resp := OAuthSessionResponse{
		SessionID:     session.ID,
		Provider:      session.Provider,
		Status:        string(session.Status),
		AuthURL:       session.AuthURL,
		ExpiresAt:     formatTimeRFC3339(session.ExpiresAt),
		CredentialRef: session.CredentialRef,
		Email:         session.Email,
		Error:         session.Error,
	}
	if session.Status != oauthpkg.LoginStatusCompleted {
		writeJSON(w, resp)
		return
	}

	cred, err := a.oauth.Load(session.Provider, session.CredentialRef)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
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
	provider, changed := ensureOAuthProviderLinked(cc, cred)
	if changed {
		if !a.saveClientConfigOrWriteError(w, clientType, cfg) {
			return
		}
	}
	resp.ProviderName = provider.Name
	resp.DisplayName = oauthDisplayName(cred, provider)
	writeJSON(w, resp)
}

func (a *API) HandleListOAuthAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provider := extractOAuthAccountsProvider(r.URL.EscapedPath())
	if provider == "" {
		writeError(w, "invalid oauth provider", http.StatusBadRequest)
		return
	}

	accounts, err := a.oauth.List(provider)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}
	linked := linkedOAuthProviders(cfg, provider)
	resp := make([]OAuthAccountResponse, 0, len(accounts))
	for _, account := range accounts {
		resp = append(resp, OAuthAccountResponse{
			Provider:        account.Provider,
			Ref:             account.Ref,
			Email:           account.Email,
			ExpiresAt:       formatTimeRFC3339(account.ExpiresAt),
			LastRefresh:     formatTimeRFC3339(account.LastRefresh),
			LinkedProviders: linked[account.Ref],
		})
	}
	writeJSON(w, resp)
}

func (a *API) HandleDeleteOAuthAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provider, ref := extractOAuthAccountRef(r.URL.EscapedPath())
	if provider == "" || ref == "" {
		writeError(w, "invalid oauth account path", http.StatusBadRequest)
		return
	}

	a.configMu.Lock()
	defer a.configMu.Unlock()

	cfg := a.loadConfigOrWriteError(w)
	if cfg == nil {
		return
	}
	_, err := a.deleteOAuthAccountLocked(cfg, provider, ref)
	if err != nil {
		writeAPIError(w, newAPIError(http.StatusInternalServerError, fmt.Sprintf("failed to delete oauth account: %v", err), err))
		return
	}
	writeJSON(w, SuccessResponse{Message: "oauth account deleted successfully"})
}

func (a *API) HandleGetProviderOAuthMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientType, providerName, subresource := extractClientProviderSubresource(r.URL.EscapedPath())
	if clientType == "" || providerName == "" || subresource != "oauth-metadata" {
		writeError(w, "invalid client type or provider name", http.StatusBadRequest)
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
	provider := providerByName(cc.Providers, providerName)
	if provider == nil {
		writeError(w, "provider not found", http.StatusNotFound)
		return
	}
	if !provider.UsesOAuth() {
		writeError(w, "provider does not use oauth", http.StatusBadRequest)
		return
	}
	if provider.NormalizedOAuthProvider() != config.OAuthProviderCodex {
		writeError(w, "oauth metadata is unavailable for this provider", http.StatusBadRequest)
		return
	}

	usageCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	details, err := a.oauth.GetCodexUsage(usageCtx, provider.NormalizedOAuthRef())
	cancel()
	if err != nil {
		writeAPIError(w, newAPIError(http.StatusBadGateway, fmt.Sprintf("failed to load oauth metadata: %v", err), err))
		return
	}

	writeJSON(w, ProviderOAuthMetadataResponse{
		OAuthPlanType:   details.PlanType,
		OAuthRateLimits: mapProviderOAuthLimitsResponse(details),
	})
}

func (a *API) toProviderResponses(providers []config.Provider, usageByProvider map[string]telemetry.ProviderUsage) []ProviderResponse {
	out := toProviderResponses(providers, usageByProvider)
	now := time.Now()
	for i := range providers {
		if !providers[i].UsesOAuth() {
			continue
		}
		out[i].OAuthAuthStatus = oauthAuthStatus(nil, now)
		cred, err := a.oauth.Load(providers[i].NormalizedOAuthProvider(), providers[i].NormalizedOAuthRef())
		if err != nil {
			continue
		}
		out[i].DisplayName = oauthDisplayName(cred, providers[i])
		out[i].OAuthAuthStatus = oauthAuthStatus(cred, now)
		out[i].OAuthExpiresAt = formatTimeRFC3339(cred.ExpiresAt)
		out[i].OAuthLastRefresh = formatTimeRFC3339(cred.LastRefresh)
	}
	return out
}

func mapProviderOAuthLimitsResponse(details *oauthpkg.CodexUsageDetails) *ProviderOAuthLimits {
	if details == nil {
		return nil
	}
	resp := &ProviderOAuthLimits{
		Primary:   mapProviderOAuthLimitWindow(details.Primary),
		Secondary: mapProviderOAuthLimitWindow(details.Secondary),
	}
	for _, item := range details.Additional {
		resp.Additional = append(resp.Additional, ProviderOAuthAdditionalLimit{
			LimitID:   strings.TrimSpace(item.LimitID),
			LimitName: strings.TrimSpace(item.LimitName),
			Primary:   mapProviderOAuthLimitWindow(item.Primary),
			Secondary: mapProviderOAuthLimitWindow(item.Secondary),
		})
	}
	if resp.Primary == nil && resp.Secondary == nil && len(resp.Additional) == 0 {
		return nil
	}
	return resp
}

func mapProviderOAuthLimitWindow(window *oauthpkg.CodexUsageWindow) *ProviderOAuthLimitWindow {
	if window == nil {
		return nil
	}
	return &ProviderOAuthLimitWindow{
		UsedPercent:   window.UsedPercent,
		WindowMinutes: window.WindowMinutes,
		ResetsAt:      formatTimeRFC3339(window.ResetsAt),
	}
}

func oauthDisplayName(cred *oauthpkg.Credential, provider config.Provider) string {
	if cred != nil {
		if email := strings.TrimSpace(cred.Email); email != "" {
			return email
		}
	}
	return strings.TrimSpace(provider.Name)
}

func oauthAuthStatus(cred *oauthpkg.Credential, now time.Time) string {
	if cred == nil {
		return "reauth_needed"
	}

	hasAccessToken := strings.TrimSpace(cred.AccessToken) != ""
	hasRefreshToken := strings.TrimSpace(cred.RefreshToken) != ""
	needsRefresh := cred.NeedsRefresh(now, 0)

	switch {
	case !hasAccessToken && hasRefreshToken:
		return "refresh_due"
	case !hasAccessToken:
		return "reauth_needed"
	case needsRefresh && hasRefreshToken:
		return "refresh_due"
	case needsRefresh:
		return "reauth_needed"
	default:
		return "ready"
	}
}

func ensureOAuthProviderLinked(cc *config.ClientConfig, cred *oauthpkg.Credential) (config.Provider, bool) {
	if cc == nil || cred == nil {
		return config.Provider{}, false
	}
	for _, provider := range cc.Providers {
		if provider.UsesOAuth() &&
			provider.NormalizedOAuthProvider() == cred.Provider &&
			provider.NormalizedOAuthRef() == cred.Ref {
			return provider, false
		}
	}
	name := nextAvailableOAuthProviderName(cc.Providers, desiredOAuthProviderName(cred))
	provider := config.Provider{
		Name:          name,
		AuthType:      config.ProviderAuthTypeOAuth,
		OAuthProvider: cred.Provider,
		OAuthRef:      cred.Ref,
		Priority:      nextProviderPriority(cc.Providers),
		Enabled:       ptr(true),
	}
	cc.Providers = append(cc.Providers, provider)
	return provider, true
}

func desiredOAuthProviderName(cred *oauthpkg.Credential) string {
	if cred == nil {
		return "oauth-account"
	}

	providerPart := slugOAuthProviderNamePart(string(cred.Provider))
	identityPart := slugOAuthProviderNamePart(cred.Email)
	if identityPart == "" {
		identityPart = slugOAuthProviderNamePart(cred.Ref)
	}

	switch {
	case providerPart != "" && identityPart != "":
		return normalizeOAuthProviderNameBase(providerPart + "-" + identityPart)
	case identityPart != "":
		return normalizeOAuthProviderNameBase(identityPart)
	case providerPart != "":
		return normalizeOAuthProviderNameBase(providerPart + "-account")
	default:
		return "oauth-account"
	}
}

func nextAvailableOAuthProviderName(providers []config.Provider, desired string) string {
	name := normalizeOAuthProviderNameBase(desired)
	if !providerNameExists(providers, name) {
		return name
	}

	for suffix := 2; ; suffix++ {
		candidate := providerNameWithNumericSuffix(name, suffix)
		if !providerNameExists(providers, candidate) {
			return candidate
		}
	}
}

func normalizeOAuthProviderNameBase(base string) string {
	base = slugOAuthProviderNamePart(base)
	if base == "" {
		base = "oauth-account"
	}
	if len(base) > providerNameMaxLen {
		base = base[:providerNameMaxLen]
	}
	base = strings.Trim(base, "-._")
	if base == "" {
		return "oauth-account"
	}
	return base
}

func slugOAuthProviderNamePart(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range v {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			_, _ = b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			_ = b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func providerNameWithNumericSuffix(base string, suffix int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "oauth-account"
	}

	suffixText := fmt.Sprintf("-%d", suffix)
	maxBaseLen := providerNameMaxLen - len(suffixText)
	if maxBaseLen < 1 {
		maxBaseLen = 1
	}
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
	}
	base = strings.TrimRight(base, "-._")
	if base == "" {
		base = "oauth"
		if len(base) > maxBaseLen {
			base = base[:maxBaseLen]
		}
	}
	return base + suffixText
}

func linkedOAuthProviders(cfg *config.Config, provider config.OAuthProvider) map[string][]string {
	out := make(map[string][]string)
	if cfg == nil {
		return out
	}
	appendClient := func(clientType string, providers []config.Provider) {
		for _, p := range providers {
			if !p.UsesOAuth() || p.NormalizedOAuthProvider() != provider {
				continue
			}
			out[p.NormalizedOAuthRef()] = append(out[p.NormalizedOAuthRef()], clientType+"/"+p.Name)
		}
	}
	appendClient("claude", cfg.Claude.Providers)
	appendClient("openai", cfg.OpenAI.Providers)
	appendClient("gemini", cfg.Gemini.Providers)
	for ref := range out {
		sort.Strings(out[ref])
	}
	return out
}

func formatTimeRFC3339(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format(time.RFC3339)
}

func extractOAuthSessionID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "oauth" && parts[2] == "sessions" {
		return strings.TrimSpace(parts[3])
	}
	return ""
}

func extractOAuthAccountsProvider(path string) config.OAuthProvider {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 4 && parts[0] == "api" && parts[1] == "oauth" && parts[2] == "accounts" {
		return config.OAuthProvider(strings.ToLower(strings.TrimSpace(parts[3])))
	}
	return ""
}

func extractOAuthAccountRef(path string) (config.OAuthProvider, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 5 && parts[0] == "api" && parts[1] == "oauth" && parts[2] == "accounts" {
		return config.OAuthProvider(strings.ToLower(strings.TrimSpace(parts[3]))), strings.TrimSpace(parts[4])
	}
	return "", ""
}

func (a *API) setOAuthTargetClient(sessionID string, clientType string) {
	a.oauthMu.Lock()
	defer a.oauthMu.Unlock()
	a.oauthTargets[strings.TrimSpace(sessionID)] = strings.TrimSpace(clientType)
}

func (a *API) getOAuthTargetClient(sessionID string) (string, bool) {
	a.oauthMu.Lock()
	defer a.oauthMu.Unlock()
	clientType, ok := a.oauthTargets[strings.TrimSpace(sessionID)]
	return clientType, ok
}
