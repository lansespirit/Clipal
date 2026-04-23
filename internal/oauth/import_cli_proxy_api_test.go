package oauth

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
)

func TestParseCLIProxyAPICredential_Codex(t *testing.T) {
	cred, err := ParseCLIProxyAPICredential([]byte(`{
  "type": "codex",
  "email": "sean@example.com",
  "account_id": "acct_123",
  "access_token": "access-1",
  "refresh_token": "refresh-1",
  "id_token": "` + testOAuthJWT("sean@example.com", "acct_123") + `",
  "expired": "2026-04-29T11:54:11+08:00",
  "last_refresh": "2026-04-21T11:54:11+08:00",
  "disabled": false
}`))
	if err != nil {
		t.Fatalf("ParseCLIProxyAPICredential: %v", err)
	}

	if got := cred.Provider; got != config.OAuthProviderCodex {
		t.Fatalf("provider = %q, want codex", got)
	}
	if got := cred.Ref; got != "codex-sean-example-com" {
		t.Fatalf("ref = %q, want codex-sean-example-com", got)
	}
	if got := cred.Email; got != "sean@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := cred.AccountID; got != "acct_123" {
		t.Fatalf("account_id = %q", got)
	}
	if got := cred.Metadata["id_token"]; got == "" {
		t.Fatalf("expected id_token metadata to be preserved")
	}
	wantExpiresAt := time.Date(2026, 4, 29, 3, 54, 11, 0, time.UTC)
	if !cred.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("expires_at = %s, want %s", cred.ExpiresAt, wantExpiresAt)
	}
	wantLastRefresh := time.Date(2026, 4, 21, 3, 54, 11, 0, time.UTC)
	if !cred.LastRefresh.Equal(wantLastRefresh) {
		t.Fatalf("last_refresh = %s, want %s", cred.LastRefresh, wantLastRefresh)
	}
}

func TestParseCLIProxyAPICredential_Claude(t *testing.T) {
	cred, err := ParseCLIProxyAPICredential([]byte(`{
  "type": "claude",
  "email": "sean@example.com",
  "account": {
    "uuid": "acct_123",
    "email_address": "sean@example.com"
  },
  "organization": {
    "uuid": "org_123",
    "name": "Anthropic"
  },
  "access_token": "access-1",
  "refresh_token": "refresh-1",
  "expired": "2026-04-29T11:54:11+08:00",
  "last_refresh": "2026-04-21T11:54:11+08:00",
  "disabled": false
}`))
	if err != nil {
		t.Fatalf("ParseCLIProxyAPICredential: %v", err)
	}

	if got := cred.Provider; got != config.OAuthProviderClaude {
		t.Fatalf("provider = %q, want claude", got)
	}
	if got := cred.Ref; got != "claude-sean-example-com" {
		t.Fatalf("ref = %q, want claude-sean-example-com", got)
	}
	if got := cred.Email; got != "sean@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := cred.AccountID; got != "acct_123" {
		t.Fatalf("account_id = %q, want acct_123", got)
	}
	if got := cred.Metadata["organization_id"]; got != "org_123" {
		t.Fatalf("metadata organization_id = %q, want org_123", got)
	}
	if got := cred.Metadata["organization_name"]; got != "Anthropic" {
		t.Fatalf("metadata organization_name = %q, want Anthropic", got)
	}
	wantExpiresAt := time.Date(2026, 4, 29, 3, 54, 11, 0, time.UTC)
	if !cred.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("expires_at = %s, want %s", cred.ExpiresAt, wantExpiresAt)
	}
	wantLastRefresh := time.Date(2026, 4, 21, 3, 54, 11, 0, time.UTC)
	if !cred.LastRefresh.Equal(wantLastRefresh) {
		t.Fatalf("last_refresh = %s, want %s", cred.LastRefresh, wantLastRefresh)
	}
}

func TestParseCLIProxyAPICredential_Gemini(t *testing.T) {
	cred, err := ParseCLIProxyAPICredential([]byte(`{
  "type": "gemini",
  "email": "sean@example.com",
  "project_id": "gen-lang-client-123",
  "access_token": "access-1",
  "refresh_token": "refresh-1",
  "expiry": "2026-04-29T11:54:11+08:00",
  "last_refresh": "2026-04-21T11:54:11+08:00",
  "token": {
    "access_token": "access-1",
    "refresh_token": "refresh-1",
    "token_type": "Bearer"
  },
  "token_type": "Bearer",
  "auto": true,
  "checked": true,
  "disabled": false
}`))
	if err != nil {
		t.Fatalf("ParseCLIProxyAPICredential: %v", err)
	}

	if got := cred.Provider; got != config.OAuthProviderGemini {
		t.Fatalf("provider = %q, want gemini", got)
	}
	if got := cred.Ref; got != "gemini-sean-example-com-gen-lang-client-123" {
		t.Fatalf("ref = %q, want gemini-sean-example-com-gen-lang-client-123", got)
	}
	if got := cred.Email; got != "sean@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := cred.AccountID; got != "gen-lang-client-123" {
		t.Fatalf("account_id = %q", got)
	}
	if got := cred.Metadata["project_id"]; got != "gen-lang-client-123" {
		t.Fatalf("metadata project_id = %q", got)
	}
	if got := cred.Metadata["auto_project"]; got != "true" {
		t.Fatalf("metadata auto_project = %q", got)
	}
	if got := cred.Metadata["checked"]; got != "true" {
		t.Fatalf("metadata checked = %q", got)
	}
	if got := cred.Metadata["token_type"]; got != "Bearer" {
		t.Fatalf("metadata token_type = %q", got)
	}
	if got := cred.Metadata["token_json"]; got == "" {
		t.Fatalf("expected token_json metadata to be preserved")
	}
	wantExpiresAt := time.Date(2026, 4, 29, 3, 54, 11, 0, time.UTC)
	if !cred.ExpiresAt.Equal(wantExpiresAt) {
		t.Fatalf("expires_at = %s, want %s", cred.ExpiresAt, wantExpiresAt)
	}
	wantLastRefresh := time.Date(2026, 4, 21, 3, 54, 11, 0, time.UTC)
	if !cred.LastRefresh.Equal(wantLastRefresh) {
		t.Fatalf("last_refresh = %s, want %s", cred.LastRefresh, wantLastRefresh)
	}
}

func TestParseCLIProxyAPICredential_FillsIdentityFromIDToken(t *testing.T) {
	cred, err := ParseCLIProxyAPICredential([]byte(`{
  "type": "codex",
  "access_token": "access-1",
  "refresh_token": "refresh-1",
  "id_token": "` + testOAuthJWT("sean@example.com", "acct_123") + `"
}`))
	if err != nil {
		t.Fatalf("ParseCLIProxyAPICredential: %v", err)
	}
	if got := cred.Email; got != "sean@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := cred.AccountID; got != "acct_123" {
		t.Fatalf("account_id = %q", got)
	}
	if got := cred.Ref; got != "codex-sean-example-com" {
		t.Fatalf("ref = %q", got)
	}
}

func TestParseCLIProxyAPICredential_SkipCases(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{
			name: "missing type",
			body: `{"access_token":"access-1"}`,
			want: ErrCLIProxyAPINotCredential,
		},
		{
			name: "unsupported type",
			body: `{"type":"vertex","access_token":"access-1"}`,
			want: ErrCLIProxyAPIUnsupportedType,
		},
		{
			name: "disabled",
			body: `{"type":"codex","access_token":"access-1","disabled":true}`,
			want: ErrCLIProxyAPIDisabledCredential,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCLIProxyAPICredential([]byte(tc.body))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func testOAuthJWT(email string, accountID string) string {
	header := `{"alg":"none","typ":"JWT"}`
	payload := `{"email":"` + email + `","sub":"sub_123","https://api.openai.com/auth":{"chatgpt_account_id":"` + accountID + `"}}`
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + "."
}
