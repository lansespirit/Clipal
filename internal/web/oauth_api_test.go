package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	oauthpkg "github.com/lansespirit/Clipal/internal/oauth"
	"github.com/lansespirit/Clipal/internal/telemetry"
	"github.com/lansespirit/Clipal/internal/testutil"
)

func TestHandleStartOAuthProvider_ReturnsAuthURLAndSessionID(t *testing.T) {
	api := newTestOAuthAPI(t, nil)

	body := []byte(`{"client_type":"openai","provider":"codex"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/providers/start", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleStartOAuthProvider(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
	if got["provider"] != "codex" {
		t.Fatalf("provider = %v, want codex", got["provider"])
	}
	if got["session_id"] == "" {
		t.Fatalf("expected session_id, got %#v", got)
	}
	authURL, _ := got["auth_url"].(string)
	if authURL == "" {
		t.Fatalf("expected auth_url, got %#v", got)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if parsed.Query().Get("redirect_uri") == "" {
		t.Fatalf("expected redirect_uri in auth_url, got %s", authURL)
	}
}

func TestHandleListOAuthProviders_ReturnsAvailableProvidersForClient(t *testing.T) {
	api := newTestOAuthAPI(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/providers?client_type=openai", nil)
	w := httptest.NewRecorder()
	api.HandleListOAuthProviders(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	var got []OAuthProviderOptionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Provider != config.OAuthProviderCodex {
		t.Fatalf("providers = %#v, want [codex]", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/oauth/providers?client_type=gemini", nil)
	w = httptest.NewRecorder()
	api.HandleListOAuthProviders(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("gemini status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("gemini body = %s, want []", w.Body.String())
	}
}

func TestHandleGetOAuthSession_AutoCreatesProvider(t *testing.T) {
	now := time.Date(2026, 4, 18, 21, 0, 0, 0, time.UTC)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "authorization_code" {
			t.Fatalf("grant_type = %q, want authorization_code", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"access_token":"access-1","refresh_token":"refresh-1","id_token":"%s","expires_in":3600}`, testOAuthJWT("sean@example.com", "acct_123")))
	}))
	defer tokenServer.Close()

	api := newTestOAuthAPI(t, &oauthpkg.CodexClient{
		AuthURL:      "https://auth.openai.com/oauth/authorize",
		TokenURL:     tokenServer.URL,
		ClientID:     "test-client",
		CallbackHost: "127.0.0.1",
		CallbackPort: 0,
		CallbackPath: "/auth/callback",
		HTTPClient:   tokenServer.Client(),
		Now:          func() time.Time { return now },
	})

	start := startOAuthSession(t, api)
	completeOAuthCallback(t, start)

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/sessions/"+start.SessionID, nil)
	w := httptest.NewRecorder()
	api.HandleGetOAuthSession(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
	if got["status"] != "completed" {
		t.Fatalf("status = %v, want completed", got["status"])
	}
	if got["provider_name"] != "codex-sean-example-com" {
		t.Fatalf("provider_name = %v, want codex-sean-example-com", got["provider_name"])
	}
	if got["display_name"] != "sean@example.com" {
		t.Fatalf("display_name = %v, want sean@example.com", got["display_name"])
	}

	cfg, err := config.Load(api.configDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 1 {
		t.Fatalf("providers len = %d, want 1", len(cfg.OpenAI.Providers))
	}
	provider := cfg.OpenAI.Providers[0]
	if provider.Name != "codex-sean-example-com" {
		t.Fatalf("provider name = %q", provider.Name)
	}
	if got := provider.NormalizedAuthType(); got != config.ProviderAuthTypeOAuth {
		t.Fatalf("auth_type = %q, want oauth", got)
	}
	if got := provider.NormalizedOAuthRef(); got != "codex-sean-example-com" {
		t.Fatalf("oauth_ref = %q", got)
	}
}

func TestEnsureOAuthProviderLinked_GeneratesNameFromEmailInsteadOfRef(t *testing.T) {
	cc := &config.ClientConfig{}
	cred := &oauthpkg.Credential{
		Ref:      "codex-acct-123",
		Provider: config.OAuthProviderCodex,
		Email:    "sean@example.com",
	}

	provider, changed := ensureOAuthProviderLinked(cc, cred)
	if !changed {
		t.Fatalf("expected provider to be created")
	}
	if got := provider.Name; got != "codex-sean-example-com" {
		t.Fatalf("provider name = %q, want codex-sean-example-com", got)
	}
	if got := provider.NormalizedOAuthRef(); got != "codex-acct-123" {
		t.Fatalf("oauth_ref = %q, want codex-acct-123", got)
	}
}

func TestHandleGetOAuthSession_ReusesExistingProviderForDuplicateAuthorization(t *testing.T) {
	now := time.Date(2026, 4, 18, 21, 30, 0, 0, time.UTC)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"access_token":"access-1","refresh_token":"refresh-1","id_token":"%s","expires_in":3600}`, testOAuthJWT("sean@example.com", "acct_123")))
	}))
	defer tokenServer.Close()

	api := newTestOAuthAPI(t, &oauthpkg.CodexClient{
		AuthURL:      "https://auth.openai.com/oauth/authorize",
		TokenURL:     tokenServer.URL,
		ClientID:     "test-client",
		CallbackHost: "127.0.0.1",
		CallbackPort: 0,
		CallbackPath: "/auth/callback",
		HTTPClient:   tokenServer.Client(),
		Now:          func() time.Time { return now },
	})

	first := startOAuthSession(t, api)
	completeOAuthCallback(t, first)
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/sessions/"+first.SessionID, nil)
	w := httptest.NewRecorder()
	api.HandleGetOAuthSession(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("first status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	second := startOAuthSession(t, api)
	completeOAuthCallback(t, second)
	req = httptest.NewRequest(http.MethodGet, "/api/oauth/sessions/"+second.SessionID, nil)
	w = httptest.NewRecorder()
	api.HandleGetOAuthSession(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("second status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := config.Load(api.configDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 1 {
		t.Fatalf("providers len = %d, want 1", len(cfg.OpenAI.Providers))
	}
}

func TestHandleGetOAuthSession_AppendsNumericSuffixWhenProviderNameAlreadyExists(t *testing.T) {
	now := time.Date(2026, 4, 18, 22, 0, 0, 0, time.UTC)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"access_token":"access-1","refresh_token":"refresh-1","id_token":"%s","expires_in":3600}`, testOAuthJWT("sean@example.com", "acct_123")))
	}))
	defer tokenServer.Close()

	api := newTestOAuthAPI(t, &oauthpkg.CodexClient{
		AuthURL:      "https://auth.openai.com/oauth/authorize",
		TokenURL:     tokenServer.URL,
		ClientID:     "test-client",
		CallbackHost: "127.0.0.1",
		CallbackPort: 0,
		CallbackPath: "/auth/callback",
		HTTPClient:   tokenServer.Client(),
		Now:          func() time.Time { return now },
	})

	if err := os.WriteFile(filepath.Join(api.configDir, "openai.yaml"), []byte(`
providers:
  - name: codex-sean-example-com
    base_url: https://api.example.com/v1
    api_key: provider-key
    priority: 1
`), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	start := startOAuthSession(t, api)
	completeOAuthCallback(t, start)

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/sessions/"+start.SessionID, nil)
	w := httptest.NewRecorder()
	api.HandleGetOAuthSession(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
	if got["provider_name"] != "codex-sean-example-com-2" {
		t.Fatalf("provider_name = %v, want codex-sean-example-com-2", got["provider_name"])
	}

	cfg, err := config.Load(api.configDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 2 {
		t.Fatalf("providers len = %d, want 2", len(cfg.OpenAI.Providers))
	}
	if cfg.OpenAI.Providers[1].Name != "codex-sean-example-com-2" {
		t.Fatalf("oauth provider name = %q, want codex-sean-example-com-2", cfg.OpenAI.Providers[1].Name)
	}
}

func TestHandleListOAuthAccounts_AndDeleteRemovesLinkedProviders(t *testing.T) {
	dir := t.TempDir()
	api := NewAPI(dir, "test", nil)
	cred := &oauthpkg.Credential{
		Ref:          "codex-sean-example-com",
		Provider:     config.OAuthProviderCodex,
		Email:        "sean@example.com",
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Date(2026, 4, 18, 23, 0, 0, 0, time.UTC),
		LastRefresh:  time.Date(2026, 4, 18, 22, 0, 0, 0, time.UTC),
	}
	if err := api.oauth.Store().Save(cred); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openai.yaml"), []byte(`
providers:
  - name: codex-sean-example-com
    auth_type: oauth
    oauth_provider: codex
    oauth_ref: codex-sean-example-com
    priority: 1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := api.telemetry.RecordUsage("openai", "codex-sean-example-com", telemetry.UsageSnapshot{
		UsageDelta: telemetry.UsageDelta{InputTokens: 1, OutputTokens: 2},
	}, time.Now()); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/accounts/codex", nil)
	w := httptest.NewRecorder()
	api.HandleListOAuthAccounts(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	var accounts []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &accounts); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(accounts))
	}
	linked, ok := accounts[0]["linked_providers"].([]any)
	if !ok || len(linked) != 1 || linked[0] != "openai/codex-sean-example-com" {
		t.Fatalf("linked_providers = %#v", accounts[0]["linked_providers"])
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/oauth/accounts/codex/codex-sean-example-com", nil)
	w = httptest.NewRecorder()
	api.HandleDeleteOAuthAccount(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 0 {
		t.Fatalf("providers=%#v", cfg.OpenAI.Providers)
	}
	if _, err := api.oauth.Load(config.OAuthProviderCodex, "codex-sean-example-com"); !os.IsNotExist(err) {
		t.Fatalf("Load err = %v, want not-exist", err)
	}
	if _, ok := api.telemetry.ProviderSnapshot("openai", "codex-sean-example-com"); ok {
		t.Fatalf("expected usage snapshot for deleted oauth provider to be removed")
	}
}

func TestHandleImportCLIProxyAPICredentials_ImportsAndLinksProvider(t *testing.T) {
	api := NewAPI(t.TempDir(), "test", nil)

	req := newOAuthImportRequest(t,
		"openai",
		"codex",
		importedOAuthFile{
			Name: "codex-sean.json",
			Body: `{
  "type": "codex",
  "email": "sean@example.com",
  "account_id": "acct_123",
  "access_token": "access-1",
  "refresh_token": "refresh-1",
  "id_token": "` + testOAuthJWT("sean@example.com", "acct_123") + `",
  "expired": "2026-04-29T11:54:11+08:00",
  "last_refresh": "2026-04-21T11:54:11+08:00"
}`,
		},
	)
	w := httptest.NewRecorder()
	api.HandleImportCLIProxyAPICredentials(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	var resp OAuthImportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ImportedCount != 1 || resp.LinkedCount != 1 || resp.SkippedCount != 0 || resp.FailedCount != 0 {
		t.Fatalf("unexpected import counts: %#v", resp)
	}
	if len(resp.Results) != 1 || resp.Results[0].Status != "imported" {
		t.Fatalf("results = %#v", resp.Results)
	}
	if got := resp.Results[0].ProviderName; got != "codex-sean-example-com" {
		t.Fatalf("provider_name = %q, want codex-sean-example-com", got)
	}

	cfg, err := config.Load(api.configDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 1 {
		t.Fatalf("providers len = %d, want 1", len(cfg.OpenAI.Providers))
	}
	provider := cfg.OpenAI.Providers[0]
	if provider.Name != "codex-sean-example-com" {
		t.Fatalf("provider name = %q", provider.Name)
	}
	cred, err := api.oauth.Load(config.OAuthProviderCodex, "codex-sean-example-com")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cred.Email; got != "sean@example.com" {
		t.Fatalf("email = %q", got)
	}
}

func TestHandleImportCLIProxyAPICredentials_ReportsSkippedAndFailedFiles(t *testing.T) {
	api := NewAPI(t.TempDir(), "test", nil)

	req := newOAuthImportRequest(t,
		"openai",
		"codex",
		importedOAuthFile{
			Name: "good.json",
			Body: `{
  "type": "codex",
  "email": "sean@example.com",
  "account_id": "acct_123",
  "access_token": "access-1",
  "refresh_token": "refresh-1"
}`,
		},
		importedOAuthFile{
			Name: "duplicate.json",
			Body: `{
  "type": "codex",
  "email": "sean@example.com",
  "account_id": "acct_123",
  "access_token": "access-2",
  "refresh_token": "refresh-2"
}`,
		},
		importedOAuthFile{
			Name: "gemini.json",
			Body: `{
  "type": "gemini",
  "access_token": "access-1"
}`,
		},
		importedOAuthFile{
			Name: "notes.txt",
			Body: `not-json`,
		},
		importedOAuthFile{
			Name: "broken.json",
			Body: `{`,
		},
	)
	w := httptest.NewRecorder()
	api.HandleImportCLIProxyAPICredentials(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}

	var resp OAuthImportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if resp.ImportedCount != 1 || resp.LinkedCount != 1 || resp.SkippedCount != 3 || resp.FailedCount != 1 {
		t.Fatalf("unexpected import counts: %#v", resp)
	}
	if got := findOAuthImportResult(resp.Results, "duplicate.json"); got.Status != "skipped" {
		t.Fatalf("duplicate result = %#v", got)
	}
	if got := findOAuthImportResult(resp.Results, "gemini.json"); got.Status != "skipped" {
		t.Fatalf("gemini result = %#v", got)
	}
	if got := findOAuthImportResult(resp.Results, "notes.txt"); got.Status != "skipped" {
		t.Fatalf("notes result = %#v", got)
	}
	if got := findOAuthImportResult(resp.Results, "broken.json"); got.Status != "failed" {
		t.Fatalf("broken result = %#v", got)
	}

	cfg, err := config.Load(api.configDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.OpenAI.Providers) != 1 {
		t.Fatalf("providers len = %d, want 1", len(cfg.OpenAI.Providers))
	}
}

type startedOAuthSession struct {
	SessionID string
	AuthURL   string
}

type importedOAuthFile struct {
	Name string
	Body string
}

func newTestOAuthAPI(t *testing.T, client *oauthpkg.CodexClient) *API {
	t.Helper()
	api := NewAPI(t.TempDir(), "test", nil)
	if client == nil {
		client = oauthpkg.NewCodexClient()
		client.CallbackHost = "127.0.0.1"
		client.CallbackPort = 0
	}
	opts := []oauthpkg.Option{}
	opts = append(opts, oauthpkg.WithCodexClient(client))
	api.oauth = oauthpkg.NewService(api.configDir, opts...)
	return api
}

func startOAuthSession(t *testing.T, api *API) startedOAuthSession {
	t.Helper()
	body := []byte(`{"client_type":"openai","provider":"codex"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/providers/start", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleStartOAuthProvider(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("start status=%d body=%s", w.Result().StatusCode, w.Body.String())
	}
	got := testutil.DecodeJSONMap(t, w.Body.Bytes())
	sessionID, ok1 := got["session_id"].(string)
	authURL, ok2 := got["auth_url"].(string)
	if !ok1 || !ok2 {
		t.Fatalf("missing or invalid session_id/auth_url in response: %#v", got)
	}
	return startedOAuthSession{
		SessionID: sessionID,
		AuthURL:   authURL,
	}
}

func completeOAuthCallback(t *testing.T, started startedOAuthSession) {
	t.Helper()
	parsed, err := url.Parse(started.AuthURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	redirectURI := parsed.Query().Get("redirect_uri")
	resp, err := http.Get(redirectURI + "?code=auth-code&state=" + url.QueryEscape(started.SessionID))
	if err != nil {
		t.Fatalf("http.Get callback: %v", err)
	}
	_ = resp.Body.Close()
}

func testOAuthJWT(email string, accountID string) string {
	return testOAuthJWTWithPlan(email, accountID, "")
}

func testOAuthJWTWithPlan(email string, accountID string, planType string) string {
	header := `{"alg":"none","typ":"JWT"}`
	auth := fmt.Sprintf(`"chatgpt_account_id":"%s"`, accountID)
	if strings.TrimSpace(planType) != "" {
		auth += fmt.Sprintf(`,"chatgpt_plan_type":"%s"`, strings.TrimSpace(planType))
	}
	payload := fmt.Sprintf(`{"email":"%s","sub":"sub_123","https://api.openai.com/auth":{%s}}`, email, auth)
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + "."
}

func newOAuthImportRequest(t *testing.T, clientType string, provider string, files ...importedOAuthFile) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("client_type", clientType); err != nil {
		t.Fatalf("WriteField client_type: %v", err)
	}
	if err := writer.WriteField("provider", provider); err != nil {
		t.Fatalf("WriteField provider: %v", err)
	}
	for _, file := range files {
		part, err := writer.CreateFormFile("files", file.Name)
		if err != nil {
			t.Fatalf("CreateFormFile(%q): %v", file.Name, err)
		}
		if _, err := io.WriteString(part, file.Body); err != nil {
			t.Fatalf("WriteString(%q): %v", file.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/import/cli-proxy-api", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func findOAuthImportResult(results []OAuthImportFileResultResponse, file string) OAuthImportFileResultResponse {
	for _, result := range results {
		if result.File == file {
			return result
		}
	}
	return OAuthImportFileResultResponse{}
}
