package oauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCodexFetchUsage_ComputesResetTimeFromRelativeSeconds(t *testing.T) {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
			t.Fatalf("authorization = %q, want Bearer access-1", got)
		}
		if got := r.Header.Get("Chatgpt-Account-Id"); got != "account-123" {
			t.Fatalf("chatgpt-account-id = %q, want account-123", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
  "plan_type": "pro",
  "rate_limit": {
    "primary_window": {
      "used_percent": 42,
      "limit_window_seconds": 3600,
      "reset_after_seconds": 120
    }
  }
}`)
	}))
	defer usageServer.Close()

	client := &CodexClient{
		UsageURL:   usageServer.URL,
		HTTPClient: usageServer.Client(),
		Now:        func() time.Time { return now },
	}

	details, err := client.FetchUsage(t.Context(), &Credential{
		Ref:         "codex-sean-example-com",
		AccessToken: "access-1",
		AccountID:   "account-123",
	})
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if details == nil || details.Primary == nil {
		t.Fatalf("expected primary usage details, got %#v", details)
	}
	if got := details.Primary.WindowMinutes; got != 60 {
		t.Fatalf("window_minutes = %d, want 60", got)
	}
	if got := details.Primary.ResetsAt; !got.Equal(now.Add(120 * time.Second)) {
		t.Fatalf("resets_at = %s, want %s", got.Format(time.RFC3339), now.Add(120*time.Second).Format(time.RFC3339))
	}
}
