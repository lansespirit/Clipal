package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
	"github.com/lansespirit/Clipal/internal/telemetry"
)

type oauthProviderAction string

const (
	oauthProviderActionCreated  oauthProviderAction = "created"
	oauthProviderActionReused   oauthProviderAction = "reused"
	oauthProviderActionRelinked oauthProviderAction = "relinked"
)

type oauthProviderLinkResult struct {
	Provider config.Provider
	Action   oauthProviderAction
	Changed  bool
}

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
	a.setOAuthTargetClient(session.ID, clientType, session.ExpiresAt)
	writeJSON(w, OAuthStartResponse{
		SessionID: session.ID,
		Provider:  session.Provider,
		AuthURL:   session.AuthURL,
		ExpiresAt: formatTimeRFC3339(session.ExpiresAt),
	})
}

func (a *API) HandleGetOAuthSession(w http.ResponseWriter, r *http.Request) {
	sessionID, subresource := extractOAuthSessionPath(r.URL.EscapedPath())
	if subresource == "code" {
		a.HandleSubmitOAuthSessionCode(w, r)
		return
	}
	if subresource == "link" {
		a.HandleLinkOAuthSession(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if sessionID == "" || subresource != "" {
		writeError(w, "invalid oauth session", http.StatusBadRequest)
		return
	}
	clientType, _ := a.getOAuthTargetClient(sessionID)

	session, err := a.oauth.PollLogin(sessionID)
	if err != nil {
		a.deleteOAuthTargetClient(sessionID)
		writeError(w, err.Error(), http.StatusNotFound)
		return
	}
	a.writeOAuthSessionResponse(w, clientType, session, false)
	if oauthSessionTerminal(session) {
		a.deleteOAuthTargetClient(sessionID)
	}
}

func (a *API) HandleSubmitOAuthSessionCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID, subresource := extractOAuthSessionPath(r.URL.EscapedPath())
	if sessionID == "" || subresource != "code" {
		writeError(w, "invalid oauth session", http.StatusBadRequest)
		return
	}

	var req OAuthSessionCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	clientType, ok := a.resolveOAuthTargetClient(sessionID, req.ClientType)
	if !ok {
		writeError(w, "oauth session not found", http.StatusNotFound)
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		writeError(w, "authorization code is required", http.StatusBadRequest)
		return
	}

	session, err := a.oauth.CompleteLoginWithCode(r.Context(), sessionID, req.Code)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, oauthpkg.ErrSessionNotFound) {
			a.deleteOAuthTargetClient(sessionID)
			status = http.StatusNotFound
		} else if errors.Is(err, oauthpkg.ErrInvalidAuthorizationResponse) {
			status = http.StatusBadRequest
		}
		writeError(w, err.Error(), status)
		return
	}
	a.writeOAuthSessionResponse(w, clientType, session, true)
	if oauthSessionTerminal(session) {
		a.deleteOAuthTargetClient(sessionID)
	}
}

func (a *API) HandleLinkOAuthSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID, subresource := extractOAuthSessionPath(r.URL.EscapedPath())
	if sessionID == "" || subresource != "link" {
		writeError(w, "invalid oauth session", http.StatusBadRequest)
		return
	}

	var req OAuthSessionLinkRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
	}
	clientType, ok := a.resolveOAuthTargetClient(sessionID, req.ClientType)
	if !ok {
		writeError(w, "oauth session not found", http.StatusNotFound)
		return
	}

	session, err := a.oauth.PollLogin(sessionID)
	if err != nil {
		a.deleteOAuthTargetClient(sessionID)
		writeError(w, err.Error(), http.StatusNotFound)
		return
	}
	a.writeOAuthSessionResponse(w, clientType, session, true)
	if oauthSessionTerminal(session) {
		a.deleteOAuthTargetClient(sessionID)
	}
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

func ensureOAuthProviderLinked(cc *config.ClientConfig, cred *oauthpkg.Credential, lookup func(config.OAuthProvider, string) (*oauthpkg.Credential, error)) oauthProviderLinkResult {
	if cc == nil || cred == nil {
		return oauthProviderLinkResult{}
	}
	if provider := findLinkedOAuthProvider(cc.Providers, cred.Provider, cred.Ref); provider != nil {
		changed := backfillOAuthProviderIdentity(provider, cred)
		return oauthProviderLinkResult{
			Provider: *provider,
			Action:   oauthProviderActionReused,
			Changed:  changed,
		}
	}

	if provider := findRelinkableOAuthProvider(cc.Providers, cred, lookup); provider != nil {
		applyOAuthCredentialToProvider(provider, cred)
		return oauthProviderLinkResult{
			Provider: *provider,
			Action:   oauthProviderActionRelinked,
			Changed:  true,
		}
	}

	name := nextAvailableOAuthProviderName(cc.Providers, desiredOAuthProviderName(cred))
	provider := config.Provider{
		Name:          name,
		AuthType:      config.ProviderAuthTypeOAuth,
		OAuthProvider: cred.Provider,
		OAuthRef:      cred.Ref,
		OAuthIdentity: oauthpkg.AccountIdentityKey(cred),
		Priority:      nextProviderPriority(cc.Providers),
		Enabled:       ptr(true),
	}
	cc.Providers = append(cc.Providers, provider)
	return oauthProviderLinkResult{
		Provider: provider,
		Action:   oauthProviderActionCreated,
		Changed:  true,
	}
}

func findLinkedOAuthProvider(providers []config.Provider, oauthProvider config.OAuthProvider, ref string) *config.Provider {
	for i := range providers {
		if providers[i].UsesOAuth() &&
			providers[i].NormalizedOAuthProvider() == oauthProvider &&
			providers[i].NormalizedOAuthRef() == ref {
			return &providers[i]
		}
	}
	return nil
}

func findRelinkableOAuthProvider(providers []config.Provider, cred *oauthpkg.Credential, lookup func(config.OAuthProvider, string) (*oauthpkg.Credential, error)) *config.Provider {
	if cred == nil {
		return nil
	}
	desiredName := desiredOAuthProviderName(cred)
	var fallback *config.Provider
	ambiguousFallback := false

	for i := range providers {
		provider := &providers[i]
		if !provider.UsesOAuth() || provider.NormalizedOAuthProvider() != cred.Provider {
			continue
		}
		if desiredName != "" && provider.Name == desiredName {
			if canRelinkOAuthProvider(provider, cred, lookup) {
				return provider
			}
			continue
		}
		if !canRelinkOAuthProvider(provider, cred, lookup) {
			continue
		}
		if fallback == nil {
			fallback = provider
			continue
		}
		ambiguousFallback = true
	}
	if ambiguousFallback {
		return nil
	}
	return fallback
}

func applyOAuthCredentialToProvider(provider *config.Provider, cred *oauthpkg.Credential) {
	if provider == nil || cred == nil {
		return
	}
	config.NormalizeProviderAuthSettings(provider)
	provider.AuthType = config.ProviderAuthTypeOAuth
	provider.OAuthProvider = cred.Provider
	provider.OAuthRef = cred.Ref
	provider.OAuthIdentity = oauthpkg.AccountIdentityKey(cred)
	config.NormalizeProviderAuthSettings(provider)
}

func backfillOAuthProviderIdentity(provider *config.Provider, cred *oauthpkg.Credential) bool {
	if provider == nil || cred == nil {
		return false
	}
	next := oauthpkg.AccountIdentityKey(cred)
	if next == "" || provider.NormalizedOAuthIdentity() == next {
		return false
	}
	provider.OAuthIdentity = next
	config.NormalizeProviderAuthSettings(provider)
	return true
}

func canRelinkOAuthProvider(provider *config.Provider, cred *oauthpkg.Credential, lookup func(config.OAuthProvider, string) (*oauthpkg.Credential, error)) bool {
	if provider == nil || cred == nil {
		return false
	}
	if !provider.UsesOAuth() || provider.NormalizedOAuthProvider() != cred.Provider {
		return false
	}

	currentRef := provider.NormalizedOAuthRef()
	switch {
	case currentRef == "":
		return providerMatchesOAuthIdentity(provider, cred)
	case currentRef == cred.Ref:
		return true
	case lookup == nil:
		return providerMatchesOAuthIdentity(provider, cred)
	}

	currentCred, err := lookup(cred.Provider, currentRef)
	if err != nil {
		return errors.Is(err, os.ErrNotExist) && providerMatchesOAuthIdentity(provider, cred)
	}
	return oauthpkg.SameAccountIdentity(currentCred, cred)
}

func providerMatchesOAuthIdentity(provider *config.Provider, cred *oauthpkg.Credential) bool {
	if provider == nil || cred == nil {
		return false
	}
	identity := provider.NormalizedOAuthIdentity()
	return identity != "" && identity == oauthpkg.AccountIdentityKey(cred)
}

func desiredOAuthProviderName(cred *oauthpkg.Credential) string {
	if cred == nil {
		return "oauth-account"
	}

	providerPart := slugOAuthProviderNamePart(string(cred.Provider))
	identityPart := slugOAuthProviderNamePart(cred.Email)
	if cred.Provider == config.OAuthProviderGemini {
		projectPart := slugOAuthProviderNamePart(cred.AccountID)
		if projectPart == "" {
			projectPart = slugOAuthProviderNamePart(cred.Metadata["project_id"])
		}
		switch {
		case identityPart != "" && projectPart != "":
			identityPart = normalizeOAuthProviderNameBase(identityPart + "-" + projectPart)
		case projectPart != "":
			identityPart = projectPart
		}
	}
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

func (a *API) writeOAuthSessionResponse(w http.ResponseWriter, clientType string, session *oauthpkg.LoginSession, linkCompleted bool) {
	if session == nil {
		writeError(w, "oauth session not found", http.StatusNotFound)
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

	if clientType == "" {
		resp.DisplayName = strings.TrimSpace(session.Email)
		writeJSON(w, resp)
		return
	}

	if !linkCompleted {
		resp.DisplayName = strings.TrimSpace(session.Email)
		cfg := a.loadConfigOrWriteError(w)
		if cfg == nil {
			return
		}
		cc, err := getClientConfigRef(cfg, clientType)
		if err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if provider := findLinkedOAuthProvider(cc.Providers, session.Provider, session.CredentialRef); provider != nil {
			resp.ProviderName = provider.Name
			resp.ProviderAction = string(oauthProviderActionReused)
			if resp.DisplayName == "" {
				resp.DisplayName = oauthDisplayName(nil, *provider)
			}
		}
		writeJSON(w, resp)
		return
	}

	cred, err := a.oauth.Load(session.Provider, session.CredentialRef)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		writeError(w, err.Error(), status)
		return
	}
	resp.DisplayName = strings.TrimSpace(cred.Email)

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

	link := ensureOAuthProviderLinked(cc, cred, a.oauth.Load)
	if link.Changed {
		if !a.saveClientConfigOrWriteError(w, clientType, cfg) {
			return
		}
	}
	resp.ProviderName = link.Provider.Name
	resp.ProviderAction = string(link.Action)
	resp.DisplayName = oauthDisplayName(cred, link.Provider)
	writeJSON(w, resp)
}

func extractOAuthSessionPath(path string) (string, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "oauth" && parts[2] == "sessions" {
		sessionID := strings.TrimSpace(parts[3])
		if len(parts) == 4 {
			return sessionID, ""
		}
		if len(parts) == 5 {
			return sessionID, strings.TrimSpace(parts[4])
		}
	}
	return "", ""
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

func (a *API) setOAuthTargetClient(sessionID string, clientType string, expiresAt time.Time) {
	a.oauthMu.Lock()
	defer a.oauthMu.Unlock()
	a.sweepOAuthTargetsLocked(time.Now())
	a.oauthTargets[strings.TrimSpace(sessionID)] = oauthTargetClient{
		ClientType: strings.TrimSpace(clientType),
		ExpiresAt:  expiresAt,
	}
}

func (a *API) getOAuthTargetClient(sessionID string) (string, bool) {
	a.oauthMu.Lock()
	defer a.oauthMu.Unlock()
	a.sweepOAuthTargetsLocked(time.Now())
	target, ok := a.oauthTargets[strings.TrimSpace(sessionID)]
	if !ok || strings.TrimSpace(target.ClientType) == "" {
		return "", false
	}
	return target.ClientType, true
}

func (a *API) resolveOAuthTargetClient(sessionID string, fallback string) (string, bool) {
	if clientType, ok := a.getOAuthTargetClient(sessionID); ok {
		return clientType, true
	}
	clientType, ok := config.CanonicalClientType(strings.TrimSpace(fallback))
	if !ok {
		return "", false
	}
	return clientType, true
}

func (a *API) deleteOAuthTargetClient(sessionID string) {
	a.oauthMu.Lock()
	defer a.oauthMu.Unlock()
	delete(a.oauthTargets, strings.TrimSpace(sessionID))
}

func (a *API) sweepOAuthTargetsLocked(now time.Time) {
	for sessionID, target := range a.oauthTargets {
		if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(target.ClientType) == "" {
			delete(a.oauthTargets, sessionID)
			continue
		}
		if !target.ExpiresAt.IsZero() && !target.ExpiresAt.After(now) {
			delete(a.oauthTargets, sessionID)
		}
	}
}

func oauthSessionTerminal(session *oauthpkg.LoginSession) bool {
	if session == nil {
		return false
	}
	switch session.Status {
	case oauthpkg.LoginStatusCompleted, oauthpkg.LoginStatusExpired, oauthpkg.LoginStatusError:
		return true
	default:
		return false
	}
}
