package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
)

var (
	ErrCLIProxyAPINotCredential      = errors.New("not a cli-proxy-api oauth credential")
	ErrCLIProxyAPIUnsupportedType    = errors.New("unsupported cli-proxy-api oauth credential type")
	ErrCLIProxyAPIDisabledCredential = errors.New("cli-proxy-api oauth credential is disabled")
)

type cliProxyAPICredentialFile struct {
	Type         string         `json:"type"`
	Email        string         `json:"email"`
	AccountID    string         `json:"account_id"`
	ProjectID    string         `json:"project_id"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	IDToken      string         `json:"id_token"`
	Expired      string         `json:"expired"`
	ExpiresAt    string         `json:"expires_at"`
	Expiry       string         `json:"expiry"`
	LastRefresh  string         `json:"last_refresh"`
	TokenType    string         `json:"token_type"`
	Token        map[string]any `json:"token"`
	Auto         bool           `json:"auto"`
	Checked      bool           `json:"checked"`
	Disabled     bool           `json:"disabled"`
	Account      struct {
		UUID         string `json:"uuid"`
		EmailAddress string `json:"email_address"`
	} `json:"account"`
	Organization struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"organization"`
}

func ParseCLIProxyAPICredential(data []byte) (*Credential, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, ErrCLIProxyAPINotCredential
	}

	var raw cliProxyAPICredentialFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse cli-proxy-api credential json: %w", err)
	}

	provider := normalizeProvider(config.OAuthProvider(raw.Type))
	if provider == "" {
		return nil, ErrCLIProxyAPINotCredential
	}
	if raw.Disabled {
		return nil, ErrCLIProxyAPIDisabledCredential
	}

	switch provider {
	case config.OAuthProviderCodex:
		return parseCLIProxyAPICodexCredential(raw)
	case config.OAuthProviderGemini:
		return parseCLIProxyAPIGeminiCredential(raw)
	case config.OAuthProviderClaude:
		return parseCLIProxyAPIClaudeCredential(raw)
	default:
		return nil, fmt.Errorf("%w: %s", ErrCLIProxyAPIUnsupportedType, provider)
	}
}

func parseCLIProxyAPICodexCredential(raw cliProxyAPICredentialFile) (*Credential, error) {
	email := strings.TrimSpace(raw.Email)
	accountID := strings.TrimSpace(raw.AccountID)
	if tokenEmail, tokenAccountID := parseCodexIdentityToken(raw.IDToken); tokenEmail != "" || tokenAccountID != "" {
		if email == "" {
			email = tokenEmail
		}
		if accountID == "" {
			accountID = tokenAccountID
		}
	}

	if strings.TrimSpace(raw.AccessToken) == "" {
		return nil, fmt.Errorf("codex credential missing access_token")
	}
	if email == "" && accountID == "" {
		return nil, fmt.Errorf("codex credential missing email/account_id")
	}

	expiresAt, err := parseCLIProxyAPITime(firstNonEmpty(raw.Expired, raw.ExpiresAt))
	if err != nil {
		return nil, fmt.Errorf("parse codex credential expired time: %w", err)
	}
	lastRefresh, err := parseCLIProxyAPITime(raw.LastRefresh)
	if err != nil {
		return nil, fmt.Errorf("parse codex credential last_refresh time: %w", err)
	}

	cred := &Credential{
		Ref:          stableCredentialRef(config.OAuthProviderCodex, email, accountID),
		Provider:     config.OAuthProviderCodex,
		Email:        email,
		AccountID:    accountID,
		AccessToken:  strings.TrimSpace(raw.AccessToken),
		RefreshToken: strings.TrimSpace(raw.RefreshToken),
		ExpiresAt:    expiresAt,
		LastRefresh:  lastRefresh,
	}
	if idToken := strings.TrimSpace(raw.IDToken); idToken != "" {
		cred.Metadata = map[string]string{"id_token": idToken}
	}
	return cred, nil
}

func parseCLIProxyAPIClaudeCredential(raw cliProxyAPICredentialFile) (*Credential, error) {
	email := strings.TrimSpace(firstNonEmpty(raw.Email, raw.Account.EmailAddress))
	accountID := strings.TrimSpace(firstNonEmpty(raw.AccountID, raw.Account.UUID))
	organizationID := strings.TrimSpace(raw.Organization.UUID)
	if strings.TrimSpace(raw.AccessToken) == "" {
		return nil, fmt.Errorf("claude credential missing access_token")
	}
	if email == "" {
		return nil, fmt.Errorf("claude credential missing email")
	}

	expiresAt, err := parseCLIProxyAPITime(firstNonEmpty(raw.Expired, raw.ExpiresAt))
	if err != nil {
		return nil, fmt.Errorf("parse claude credential expired time: %w", err)
	}
	lastRefresh, err := parseCLIProxyAPITime(raw.LastRefresh)
	if err != nil {
		return nil, fmt.Errorf("parse claude credential last_refresh time: %w", err)
	}

	cred := &Credential{
		Ref:          stableCredentialRef(config.OAuthProviderClaude, email, firstNonEmpty(accountID, organizationID)),
		Provider:     config.OAuthProviderClaude,
		Email:        email,
		AccountID:    accountID,
		AccessToken:  strings.TrimSpace(raw.AccessToken),
		RefreshToken: strings.TrimSpace(raw.RefreshToken),
		ExpiresAt:    expiresAt,
		LastRefresh:  lastRefresh,
	}
	if idToken := strings.TrimSpace(raw.IDToken); idToken != "" || accountID != "" || organizationID != "" || strings.TrimSpace(raw.Organization.Name) != "" {
		cred.Metadata = map[string]string{}
		if idToken != "" {
			cred.Metadata["id_token"] = idToken
		}
		if accountID != "" {
			cred.Metadata["account_id"] = accountID
		}
		if organizationID != "" {
			cred.Metadata["organization_id"] = organizationID
		}
		if organizationName := strings.TrimSpace(raw.Organization.Name); organizationName != "" {
			cred.Metadata["organization_name"] = organizationName
		}
		if accountEmail := strings.TrimSpace(raw.Account.EmailAddress); accountEmail != "" {
			cred.Metadata["account_email"] = accountEmail
		}
	}
	return cred, nil
}

func parseCLIProxyAPIGeminiCredential(raw cliProxyAPICredentialFile) (*Credential, error) {
	email := strings.TrimSpace(raw.Email)
	projectID := strings.TrimSpace(raw.ProjectID)
	accessToken := strings.TrimSpace(firstNonEmpty(raw.AccessToken, stringValue(raw.Token, "access_token")))
	refreshToken := strings.TrimSpace(firstNonEmpty(raw.RefreshToken, stringValue(raw.Token, "refresh_token")))
	tokenType := strings.TrimSpace(firstNonEmpty(raw.TokenType, stringValue(raw.Token, "token_type")))
	expiryValue := firstNonEmpty(raw.Expired, raw.ExpiresAt, raw.Expiry, stringValue(raw.Token, "expiry"))

	if accessToken == "" {
		return nil, fmt.Errorf("gemini credential missing access_token")
	}
	if email == "" {
		return nil, fmt.Errorf("gemini credential missing email")
	}
	if projectID == "" {
		return nil, fmt.Errorf("gemini credential missing project_id")
	}

	expiresAt, err := parseCLIProxyAPITime(expiryValue)
	if err != nil {
		return nil, fmt.Errorf("parse gemini credential expiry time: %w", err)
	}
	lastRefresh, err := parseCLIProxyAPITime(raw.LastRefresh)
	if err != nil {
		return nil, fmt.Errorf("parse gemini credential last_refresh time: %w", err)
	}

	cred := &Credential{
		Ref:          geminiCredentialRef(email, projectID),
		Provider:     config.OAuthProviderGemini,
		Email:        email,
		AccountID:    projectID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		LastRefresh:  lastRefresh,
		Metadata: map[string]string{
			"project_id":   projectID,
			"checked":      strconv.FormatBool(raw.Checked),
			"auto_project": strconv.FormatBool(raw.Auto),
		},
	}
	if tokenType != "" {
		cred.Metadata["token_type"] = tokenType
	}
	if len(raw.Token) > 0 {
		if encodedToken, err := json.Marshal(raw.Token); err == nil {
			cred.Metadata["token_json"] = string(encodedToken)
		}
	}
	return cred, nil
}

func parseCLIProxyAPITime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[strings.TrimSpace(key)]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
