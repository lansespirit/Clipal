package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lansespirit/Clipal/internal/config"
	"gopkg.in/yaml.v3"
)

func boolPtr(v bool) *bool { return &v }

func TestFormatClientConfigYAML_QuotesSpacingAndTiers(t *testing.T) {
	cc := config.ClientConfig{
		Providers: []config.Provider{
			{Name: "p1", BaseURL: "https://a.example", APIKey: "k1", Priority: 1, Enabled: boolPtr(true)},
			{Name: "p2", BaseURL: "https://b.example", APIKey: "k2", Priority: 2, Enabled: boolPtr(false)},
			{Name: "p3", BaseURL: "https://c.example", APIKey: "k3", Priority: 3, Enabled: boolPtr(true)},
			{Name: "p4", BaseURL: "https://d.example", APIKey: "k4", Priority: 3, Enabled: boolPtr(true)},
		},
	}

	got := string(formatClientConfigYAML("codex", cc))

	// Quoted strings for readability (match examples/*.yaml style).
	for _, want := range []string{
		`name: "p1"`,
		`base_url: "https://a.example"`,
		`api_key: "k1"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q\n%s", want, got)
		}
	}

	// Tier comments.
	for _, want := range []string{
		`# Priority level 1:`,
		`# Priority level 2:`,
		`# Priority level 3:`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q\n%s", want, got)
		}
	}

	// Blank lines between providers (including within same tier).
	if !strings.Contains(got, "enabled: true\n\n  - name: \"p4\"") {
		t.Fatalf("expected blank line between providers\n%s", got)
	}
}

func TestFormatGlobalConfigYAML_HasHeaderAndQuotesEmptyStrings(t *testing.T) {
	gc := config.DefaultGlobalConfig()
	gc.LogDir = ""

	got := string(formatGlobalConfigYAML(gc))
	if !strings.HasPrefix(got, "# Global configuration for clipal\n") {
		t.Fatalf("expected header comment, got:\n%s", got)
	}
	if !strings.Contains(got, "listen_addr: \"127.0.0.1\"") {
		t.Fatalf("expected listen_addr to be quoted, got:\n%s", got)
	}
	if !strings.Contains(got, "log_dir: \"\"") {
		t.Fatalf("expected log_dir empty string to be quoted, got:\n%s", got)
	}
	if !strings.Contains(got, "\nnotifications:\n") {
		t.Fatalf("expected notifications block, got:\n%s", got)
	}
}

func TestFormatGlobalConfigYAML_EscapesNewlines(t *testing.T) {
	gc := config.DefaultGlobalConfig()
	gc.ListenAddr = "127.0.0.1\nport: 1"

	got := string(formatGlobalConfigYAML(gc))
	if strings.Contains(got, "listen_addr: 127.0.0.1\nport: 1") {
		t.Fatalf("expected listen_addr to be quoted/escaped (no YAML injection), got:\n%s", got)
	}
	if !strings.Contains(got, "listen_addr: \"127.0.0.1\\nport: 1\"") {
		t.Fatalf("expected escaped newline in listen_addr, got:\n%s", got)
	}
}

func TestFormatClientConfigYAML_SparsePriorities_CanSkipMiddleTier(t *testing.T) {
	cc := config.ClientConfig{
		Providers: []config.Provider{
			{Name: "p1", BaseURL: "https://a.example", APIKey: "k1", Priority: 1, Enabled: boolPtr(true)},
			{Name: "p2", BaseURL: "https://b.example", APIKey: "k2", Priority: 2, Enabled: boolPtr(true)},
			{Name: "p3", BaseURL: "https://c.example", APIKey: "k3", Priority: 1000, Enabled: boolPtr(true)},
		},
	}
	got := string(formatClientConfigYAML("codex", cc))

	if !strings.Contains(got, "# Priority level 1:") {
		t.Fatalf("expected tier 1 comment\n%s", got)
	}
	if !strings.Contains(got, "# Priority level 3:") {
		t.Fatalf("expected tier 3 comment\n%s", got)
	}
	if strings.Contains(got, "# Priority level 2:") {
		t.Fatalf("did not expect tier 2 comment for sparse priorities\n%s", got)
	}
}

func TestFormatClientConfigYAML_UsesAPIKeysForMultiKeyProvider(t *testing.T) {
	cc := config.ClientConfig{
		Providers: []config.Provider{
			{Name: "p1", BaseURL: "https://a.example", APIKeys: []string{"k1", "k2"}, Priority: 1, Enabled: boolPtr(true)},
		},
	}

	got := string(formatClientConfigYAML("codex", cc))
	if !strings.Contains(got, "api_keys:\n") {
		t.Fatalf("expected api_keys block, got:\n%s", got)
	}
	if strings.Contains(got, `api_key: "k1"`) {
		t.Fatalf("did not expect single api_key field for multi-key provider, got:\n%s", got)
	}
	if !strings.Contains(got, `- "k1"`) || !strings.Contains(got, `- "k2"`) {
		t.Fatalf("expected both keys in yaml, got:\n%s", got)
	}
}

func TestFormatClientConfigYAML_SingleNormalizedKeyUsesAPIKeyField(t *testing.T) {
	cc := config.ClientConfig{
		Providers: []config.Provider{
			{Name: "p1", BaseURL: "https://a.example", APIKeys: []string{"", " single ", "single"}, Priority: 1, Enabled: boolPtr(true)},
		},
	}

	got := string(formatClientConfigYAML("codex", cc))
	if strings.Contains(got, "api_keys:\n") {
		t.Fatalf("did not expect api_keys block, got:\n%s", got)
	}
	if !strings.Contains(got, `api_key: "single"`) {
		t.Fatalf("expected normalized api_key field, got:\n%s", got)
	}
}

func TestFormatClientConfigYAML_RoundTripAndEscapesSpecialCharacters(t *testing.T) {
	cc := config.ClientConfig{
		Providers: []config.Provider{
			{Name: "single", BaseURL: "https://a.example", APIKey: "only-one", Priority: 2, Enabled: boolPtr(true)},
			{
				Name:     "multi",
				BaseURL:  "https://b.example",
				APIKeys:  []string{`quote"`, `slash\`, "line\r\nbreak", "tab\tkey", "control\x01"},
				Priority: 1,
				Enabled:  boolPtr(false),
			},
			{Name: "empty", BaseURL: "https://c.example", APIKey: "", Priority: 3, Enabled: boolPtr(true)},
		},
	}

	got := string(formatClientConfigYAML("codex", cc))
	for _, want := range []string{
		`mode: auto`,
		`pinned_provider: ""`,
		`api_key: ""`,
		`\\"`,
		`\\`,
		`\r\n`,
		`\t`,
		`\x01`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q\n%s", want, got)
		}
	}

	var parsed config.ClientConfig
	if err := yaml.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\n%s", err, got)
	}
	if parsed.Mode != config.ClientModeAuto {
		t.Fatalf("mode = %q, want %q", parsed.Mode, config.ClientModeAuto)
	}
	if parsed.PinnedProvider != "" {
		t.Fatalf("pinned_provider = %q, want empty", parsed.PinnedProvider)
	}
	if len(parsed.Providers) != 3 {
		t.Fatalf("providers len = %d, want 3", len(parsed.Providers))
	}

	if parsed.Providers[0].Name != "multi" {
		t.Fatalf("providers[0].name = %q, want multi", parsed.Providers[0].Name)
	}
	if gotKeys := strings.Join(parsed.Providers[0].APIKeys, "|"); gotKeys != strings.Join([]string{`quote"`, `slash\`, "line\r\nbreak", "tab\tkey", "control\x01"}, "|") {
		t.Fatalf("providers[0].api_keys = %#v", parsed.Providers[0].APIKeys)
	}
	if parsed.Providers[1].APIKey != "only-one" {
		t.Fatalf("providers[1].api_key = %q, want %q", parsed.Providers[1].APIKey, "only-one")
	}
	if parsed.Providers[2].APIKey != "" {
		t.Fatalf("providers[2].api_key = %q, want empty", parsed.Providers[2].APIKey)
	}
}

func TestFormatClientConfigYAML_RoundTripViaConfigLoadWithNormalizedKeys(t *testing.T) {
	dir := t.TempDir()
	yamlBytes := formatClientConfigYAML("codex", config.ClientConfig{
		Mode:           config.ClientModeManual,
		PinnedProvider: "p2",
		Providers: []config.Provider{
			{Name: "p1", BaseURL: "https://a.example", APIKey: " key-1 ", Priority: 2, Enabled: boolPtr(true)},
			{Name: "p2", BaseURL: "https://b.example", APIKeys: []string{"dup", "", "dup", " second "}, Priority: 1, Enabled: boolPtr(true)},
		},
	})
	if err := os.WriteFile(filepath.Join(dir, "codex.yaml"), yamlBytes, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if loaded.Codex.Mode != config.ClientModeManual {
		t.Fatalf("mode = %q, want %q", loaded.Codex.Mode, config.ClientModeManual)
	}
	if loaded.Codex.PinnedProvider != "p2" {
		t.Fatalf("pinned_provider = %q, want p2", loaded.Codex.PinnedProvider)
	}
	if len(loaded.Codex.Providers) != 2 {
		t.Fatalf("providers len = %d, want 2", len(loaded.Codex.Providers))
	}
	if loaded.Codex.Providers[0].Name != "p2" {
		t.Fatalf("providers[0].name = %q, want p2", loaded.Codex.Providers[0].Name)
	}
	if got := strings.Join(loaded.Codex.Providers[0].NormalizedAPIKeys(), "|"); got != "dup|second" {
		t.Fatalf("providers[0] normalized keys = %q, want dup|second", got)
	}
	if got := loaded.Codex.Providers[1].PrimaryAPIKey(); got != "key-1" {
		t.Fatalf("providers[1] primary api key = %q, want key-1", got)
	}
}

func TestFormatClientConfigYAML_EmptyProvidersStableOutput(t *testing.T) {
	got := string(formatClientConfigYAML("codex", config.ClientConfig{}))
	for _, want := range []string{
		`mode: auto`,
		`pinned_provider: ""`,
		"providers: []",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q\n%s", want, got)
		}
	}

	var parsed config.ClientConfig
	if err := yaml.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\n%s", err, got)
	}
	if parsed.Mode != config.ClientModeAuto {
		t.Fatalf("mode = %q, want %q", parsed.Mode, config.ClientModeAuto)
	}
	if len(parsed.Providers) != 0 {
		t.Fatalf("providers len = %d, want 0", len(parsed.Providers))
	}
}

func TestFormatGlobalConfigYAML_RoundTripAndEscapesSpecialCharacters(t *testing.T) {
	gc := config.DefaultGlobalConfig()
	gc.ListenAddr = `host"quoted"\path`
	gc.LogDir = "logs\r\nfolder\tcontrol\x01"
	gc.LogRetentionDays = 7
	gc.LogStdout = boolPtr(false)
	gc.Notifications.ProviderSwitch = boolPtr(false)

	got := string(formatGlobalConfigYAML(gc))
	for _, want := range []string{
		`listen_addr: "host\"quoted\"\\path"`,
		`log_dir: "logs\r\nfolder\tcontrol\x01"`,
		`log_retention_days: 7 # default 7 days`,
		`log_stdout: false`,
		`provider_switch: false`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q\n%s", want, got)
		}
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(got), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if loaded.Global.ListenAddr != gc.ListenAddr {
		t.Fatalf("listen_addr = %q, want %q", loaded.Global.ListenAddr, gc.ListenAddr)
	}
	if loaded.Global.LogDir != gc.LogDir {
		t.Fatalf("log_dir = %q, want %q", loaded.Global.LogDir, gc.LogDir)
	}
	if loaded.Global.LogRetentionDays != 7 {
		t.Fatalf("log_retention_days = %d, want 7", loaded.Global.LogRetentionDays)
	}
	if loaded.Global.LogStdout == nil || *loaded.Global.LogStdout {
		t.Fatalf("log_stdout = %v, want false", loaded.Global.LogStdout)
	}
	if loaded.Global.Notifications.ProviderSwitch == nil || *loaded.Global.Notifications.ProviderSwitch {
		t.Fatalf("provider_switch = %v, want false", loaded.Global.Notifications.ProviderSwitch)
	}
}
