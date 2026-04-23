package integration

import (
	"os"
	"strings"
	"testing"
)

func TestLiveGeminiOAuthSmokeScriptUsesTemporaryCredentialCopyAndRefreshProbe(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../scripts/live_gemini_oauth_smoke.sh")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	script := string(body)
	for _, want := range []string{
		`--email EMAIL`,
		`default: gemini-3-flash-preview`,
		`CLIPAL_LIVE_OAUTH_EMAIL`,
		`CLIPAL_LIVE_VERBOSE`,
		`list_credentials "$creds_json" "$OAUTH_EMAIL" "$OAUTH_REF" "$OAUTH_FILE"`,
		`cfgdir="$tmpdir/config"`,
		`oauth email not found`,
		`credential_path="$cfgdir/oauth/gemini/$(basename "$oauth_source_path")"`,
		`clipal_log_level="info"`,
		`clipal_log_level="debug"`,
		`log_level: $clipal_log_level`,
		`--log-level "$clipal_log_level"`,
		`artifacts:`,
		`set CLIPAL_LIVE_VERBOSE=1 to print clipal.log tail and request headers/body`,
		`auth_type: "oauth"`,
		`oauth_provider: "gemini"`,
		`"http://127.0.0.1:$clipal_port/clipal/v1beta/models/$MODEL:generateContent"`,
		`"http://127.0.0.1:$clipal_port/clipal/v1beta/models/$MODEL:streamGenerateContent"`,
		`clipal-live-invalid-token`,
		`credential access_token was not replaced after refresh retry`,
		`ok refreshed temp credential updated`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("live gemini oauth smoke script missing %q", want)
		}
	}
	for _, unwanted := range []string{
		`sync_updated_credential_back`,
		`synced updated oauth credential back to source`,
		`request artifacts: $headers_file $body_file`,
		`log_level: debug`,
		`--log-level debug`,
		`ok refreshed credential persisted`,
	} {
		if strings.Contains(script, unwanted) {
			t.Fatalf("live gemini oauth smoke script should not contain %q", unwanted)
		}
	}
}
