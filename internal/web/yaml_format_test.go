package web

import (
	"strings"
	"testing"

	"github.com/lansespirit/Clipal/internal/config"
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
	if !strings.Contains(got, "log_dir: \"\"") {
		t.Fatalf("expected log_dir empty string to be quoted, got:\n%s", got)
	}
	if !strings.Contains(got, "\nnotifications:\n") {
		t.Fatalf("expected notifications block, got:\n%s", got)
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
