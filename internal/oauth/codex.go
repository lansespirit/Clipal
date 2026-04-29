package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
)

const (
	defaultCodexAuthURL      = "https://auth.openai.com/oauth/authorize"
	defaultCodexTokenURL     = "https://auth.openai.com/oauth/token"
	defaultCodexUsageURL     = "https://chatgpt.com/backend-api/wham/usage"
	defaultCodexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultCodexCallbackHost = "localhost"
	defaultCodexCallbackPort = 1455
	defaultCodexCallbackPath = "/auth/callback"
)

type PKCECodes struct {
	CodeVerifier  string
	CodeChallenge string
}

type CodexClient struct {
	AuthURL      string
	TokenURL     string
	UsageURL     string
	ClientID     string
	CallbackHost string
	CallbackPort int
	CallbackPath string
	HTTPClient   *http.Client
	Now          func() time.Time
}

func (c *CodexClient) Provider() config.OAuthProvider {
	return config.OAuthProviderCodex
}

func (c *CodexClient) WithHTTPClient(httpClient *http.Client) ProviderClient {
	if c == nil || httpClient == nil {
		return c
	}
	clone := *c
	clone.HTTPClient = httpClient
	return &clone
}

func NewCodexClient() *CodexClient {
	client := &CodexClient{
		AuthURL:      defaultCodexAuthURL,
		TokenURL:     defaultCodexTokenURL,
		UsageURL:     defaultCodexUsageURL,
		ClientID:     defaultCodexClientID,
		CallbackHost: defaultCodexCallbackHost,
		CallbackPort: defaultCodexCallbackPort,
		CallbackPath: defaultCodexCallbackPath,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		Now:          time.Now,
	}
	applyCodexClientEnvOverrides(client)
	return client
}

func GeneratePKCECodes() (PKCECodes, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return PKCECodes{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(verifier))
	return PKCECodes{
		CodeVerifier:  verifier,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func (c *CodexClient) GenerateAuthURL(state string, redirectURI string, pkce PKCECodes) (string, error) {
	if strings.TrimSpace(state) == "" {
		return "", fmt.Errorf("state is required")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return "", fmt.Errorf("redirect_uri is required")
	}
	if strings.TrimSpace(pkce.CodeVerifier) == "" || strings.TrimSpace(pkce.CodeChallenge) == "" {
		return "", fmt.Errorf("pkce codes are required")
	}
	params := url.Values{
		"client_id":                  {c.clientID()},
		"response_type":              {"code"},
		"redirect_uri":               {strings.TrimSpace(redirectURI)},
		"scope":                      {"openid email profile offline_access"},
		"state":                      {strings.TrimSpace(state)},
		"code_challenge":             {pkce.CodeChallenge},
		"code_challenge_method":      {"S256"},
		"prompt":                     {"login"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
	}
	return strings.TrimSpace(c.authURL()) + "?" + params.Encode(), nil
}

func (c *CodexClient) StartLogin(now time.Time, ttl time.Duration) (*LoginSession, error) {
	return startLoginSession(
		config.OAuthProviderCodex,
		now,
		ttl,
		c.callbackHost(),
		c.callbackPort(),
		c.callbackPath(),
		c.GenerateAuthURL,
	)
}

func (c *CodexClient) ExchangeCode(ctx context.Context, code string, redirectURI string, pkce PKCECodes) (*Credential, error) {
	token, err := c.exchange(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {c.clientID()},
		"code":          {strings.TrimSpace(code)},
		"redirect_uri":  {strings.TrimSpace(redirectURI)},
		"code_verifier": {pkce.CodeVerifier},
	})
	if err != nil {
		return nil, err
	}
	return c.credentialFromToken(token, nil), nil
}

func (c *CodexClient) ExchangeSessionCode(ctx context.Context, session *LoginSession, code string) (*Credential, error) {
	if session == nil {
		return nil, fmt.Errorf("login session is nil")
	}
	return c.ExchangeCode(ctx, code, session.redirectURI, session.pkce)
}

func (c *CodexClient) Refresh(ctx context.Context, cred *Credential) (*Credential, error) {
	if cred == nil {
		return nil, fmt.Errorf("credential is nil")
	}
	if strings.TrimSpace(cred.RefreshToken) == "" {
		return cred.Clone(), nil
	}
	token, err := c.exchange(ctx, url.Values{
		"client_id":     {c.clientID()},
		"grant_type":    {"refresh_token"},
		"refresh_token": {strings.TrimSpace(cred.RefreshToken)},
		"scope":         {"openid profile email"},
	})
	if err != nil {
		return nil, err
	}
	return c.credentialFromToken(token, cred), nil
}

type codexTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (c *CodexClient) exchange(ctx context.Context, form url.Values) (*codexTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var token codexTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (c *CodexClient) credentialFromToken(token *codexTokenResponse, previous *Credential) *Credential {
	now := c.now()
	email, accountID := parseCodexIdentityToken(token.IDToken)
	cred := &Credential{
		Ref:          stableCredentialRef(config.OAuthProviderCodex, email, accountID),
		Provider:     config.OAuthProviderCodex,
		Email:        email,
		AccountID:    accountID,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    now.Add(time.Duration(token.ExpiresIn) * time.Second),
		LastRefresh:  now,
		Metadata: map[string]string{
			"id_token": token.IDToken,
		},
	}
	if previous != nil {
		cred.Ref = previous.Ref
		if cred.Email == "" {
			cred.Email = previous.Email
		}
		if cred.AccountID == "" {
			cred.AccountID = previous.AccountID
		}
		if cred.RefreshToken == "" {
			cred.RefreshToken = previous.RefreshToken
		}
		if len(previous.Metadata) > 0 {
			if cred.Metadata == nil {
				cred.Metadata = make(map[string]string, len(previous.Metadata))
			}
			for k, v := range previous.Metadata {
				if _, exists := cred.Metadata[k]; !exists {
					cred.Metadata[k] = v
				}
			}
		}
	} else if cred.Ref == "" {
		cred.Ref = stableCredentialRef(config.OAuthProviderCodex, "", accountID)
	}
	return cred
}

func parseCodexIdentityToken(idToken string) (string, string) {
	claims, ok := parseCodexIdentityClaims(idToken)
	if !ok {
		return "", ""
	}
	accountID := strings.TrimSpace(claims.Auth.ChatGPTAccountID)
	if accountID == "" {
		accountID = strings.TrimSpace(claims.Sub)
	}
	email := strings.TrimSpace(claims.Email)
	if email == "" {
		email = strings.TrimSpace(claims.Profile.Email)
	}
	return email, accountID
}

func parseCodexPlanType(idToken string) string {
	claims, ok := parseCodexIdentityClaims(idToken)
	if !ok {
		return ""
	}
	return strings.TrimSpace(claims.Auth.ChatGPTPlanType)
}

func parseCodexIdentityClaims(idToken string) (*struct {
	Email string `json:"email"`
	Sub   string `json:"sub"`
	Auth  struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
	Profile struct {
		Email string `json:"email"`
	} `json:"https://api.openai.com/profile"`
}, bool) {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) < 2 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims struct {
		Email string `json:"email"`
		Sub   string `json:"sub"`
		Auth  struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
			ChatGPTPlanType  string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
		Profile struct {
			Email string `json:"email"`
		} `json:"https://api.openai.com/profile"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}
	return &claims, true
}

func stableCredentialRef(provider config.OAuthProvider, email string, accountID string) string {
	identity := strings.TrimSpace(email)
	if identity == "" {
		identity = strings.TrimSpace(accountID)
	}
	slug := slugify(identity)
	if slug == "" {
		var raw [6]byte
		if _, err := rand.Read(raw[:]); err == nil {
			slug = base64.RawURLEncoding.EncodeToString(raw[:])
		}
	}
	if slug == "" {
		slug = "account"
	}
	return slugify(string(provider) + "-" + slug)
}

func slugify(v string) string {
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
	out := strings.Trim(b.String(), "-")
	return strings.TrimSpace(out)
}

func (c *CodexClient) authURL() string {
	if c != nil && strings.TrimSpace(c.AuthURL) != "" {
		return strings.TrimSpace(c.AuthURL)
	}
	return defaultCodexAuthURL
}

func (c *CodexClient) tokenURL() string {
	if c != nil && strings.TrimSpace(c.TokenURL) != "" {
		return strings.TrimSpace(c.TokenURL)
	}
	return defaultCodexTokenURL
}

func (c *CodexClient) usageURL() string {
	if c != nil && strings.TrimSpace(c.UsageURL) != "" {
		return strings.TrimSpace(c.UsageURL)
	}
	return defaultCodexUsageURL
}

func (c *CodexClient) clientID() string {
	if c != nil && strings.TrimSpace(c.ClientID) != "" {
		return strings.TrimSpace(c.ClientID)
	}
	return defaultCodexClientID
}

func (c *CodexClient) callbackHost() string {
	if c != nil && strings.TrimSpace(c.CallbackHost) != "" {
		return strings.TrimSpace(c.CallbackHost)
	}
	return defaultCodexCallbackHost
}

func (c *CodexClient) callbackPort() int {
	if c != nil && c.CallbackPort >= 0 {
		return c.CallbackPort
	}
	return defaultCodexCallbackPort
}

func (c *CodexClient) callbackPath() string {
	if c != nil && strings.TrimSpace(c.CallbackPath) != "" {
		path := strings.TrimSpace(c.CallbackPath)
		if !strings.HasPrefix(path, "/") {
			return "/" + path
		}
		return path
	}
	return defaultCodexCallbackPath
}

func (c *CodexClient) httpClient() *http.Client {
	if c != nil && c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *CodexClient) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func applyCodexClientEnvOverrides(c *CodexClient) {
	if c == nil {
		return
	}
	if v, ok := lookupNonEmptyEnv("CLIPAL_OAUTH_CODEX_AUTH_URL"); ok {
		c.AuthURL = v
	}
	if v, ok := lookupNonEmptyEnv("CLIPAL_OAUTH_CODEX_TOKEN_URL"); ok {
		c.TokenURL = v
	}
	if v, ok := lookupNonEmptyEnv("CLIPAL_OAUTH_CODEX_USAGE_URL"); ok {
		c.UsageURL = v
	}
	if v, ok := lookupNonEmptyEnv("CLIPAL_OAUTH_CODEX_CLIENT_ID"); ok {
		c.ClientID = v
	}
	if v, ok := lookupNonEmptyEnv("CLIPAL_OAUTH_CODEX_CALLBACK_HOST"); ok {
		c.CallbackHost = v
	}
	if v, ok := lookupNonEmptyEnv("CLIPAL_OAUTH_CODEX_CALLBACK_PATH"); ok {
		c.CallbackPath = v
	}
	if v, ok := lookupNonEmptyEnv("CLIPAL_OAUTH_CODEX_CALLBACK_PORT"); ok {
		if port, err := strconv.Atoi(v); err == nil && port >= 0 {
			c.CallbackPort = port
		}
	}
}

func lookupNonEmptyEnv(key string) (string, bool) {
	value, ok := os.LookupEnv(strings.TrimSpace(key))
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}
