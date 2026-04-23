package integration

import (
	"os"
	"strings"
	"testing"
)

func TestSmokeScriptBuildsUpstreamBinaryBeforeLaunch(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../scripts/smoke_test.sh")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	script := string(body)
	if strings.Contains(script, `go run "$tmpdir/upstream.go"`) {
		t.Fatalf("smoke script still starts upstream via go run")
	}
	if !strings.Contains(script, `go build -o "$tmpdir/upstream" "$tmpdir/upstream.go"`) {
		t.Fatalf("smoke script does not build upstream binary before launch")
	}
	if !strings.Contains(script, `"$tmpdir/upstream" >"$tmpdir/upstream.log" 2>&1 &`) {
		t.Fatalf("smoke script does not launch built upstream binary")
	}
}

func TestMakefileDispatchesLiveOAuthSmokeTargetByProvider(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	makefile := string(body)
	for _, want := range []string{
		`ifeq ($(PROVIDER),codex)`,
		`$(MAKE) test-live-codex-oauth`,
		`$(or $(CODEX_CONFIG_DIR),$(CONFIG_DIR))`,
		`$(or $(CODEX_OAUTH_EMAIL),$(OAUTH_EMAIL))`,
		`$(or $(CODEX_OAUTH_REF),$(OAUTH_REF))`,
		`$(or $(CODEX_OAUTH_FILE),$(OAUTH_FILE))`,
		`$(or $(CODEX_MODEL),$(MODEL))`,
		`else ifeq ($(PROVIDER),claude)`,
		`$(MAKE) test-live-claude-oauth`,
		`$(or $(CLAUDE_CONFIG_DIR),$(CONFIG_DIR))`,
		`$(or $(CLAUDE_OAUTH_EMAIL),$(OAUTH_EMAIL))`,
		`$(or $(CLAUDE_OAUTH_REF),$(OAUTH_REF))`,
		`$(or $(CLAUDE_OAUTH_FILE),$(OAUTH_FILE))`,
		`$(or $(CLAUDE_MODEL),$(MODEL))`,
		`$(or $(CLAUDE_SKIP_COUNT_TOKENS),$(SKIP_COUNT_TOKENS))`,
		`else ifeq ($(PROVIDER),gemini)`,
		`$(MAKE) test-live-gemini-oauth`,
		`$(or $(GEMINI_CONFIG_DIR),$(CONFIG_DIR))`,
		`$(or $(GEMINI_OAUTH_EMAIL),$(OAUTH_EMAIL))`,
		`$(or $(GEMINI_OAUTH_REF),$(OAUTH_REF))`,
		`$(or $(GEMINI_OAUTH_FILE),$(OAUTH_FILE))`,
		`$(or $(GEMINI_MODEL),$(MODEL))`,
		`usage: make test-live-oauth PROVIDER=codex|claude|gemini`,
	} {
		if !strings.Contains(makefile, want) {
			t.Fatalf("Makefile missing %q", want)
		}
	}
}
