package oauth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveClaudeUsageFetch(t *testing.T) {
	ref := os.Getenv("CLIPAL_LIVE_CLAUDE_USAGE_REF")
	if ref == "" {
		t.Skip("set CLIPAL_LIVE_CLAUDE_USAGE_REF to run live Claude usage fetch")
	}

	configDir := os.Getenv("CLIPAL_LIVE_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("UserHomeDir: %v", err)
		}
		configDir = filepath.Join(home, ".clipal")
	}

	svc := NewService(configDir)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	details, err := svc.GetClaudeUsage(ctx, ref)
	if err != nil {
		t.Fatalf("GetClaudeUsage(%q): %v", ref, err)
	}
	if details == nil {
		t.Fatalf("GetClaudeUsage(%q) returned nil details", ref)
	}

	nonNilWindows := 0
	exhaustedWindows := 0
	var latestReset time.Time
	logWindow := func(name string, window *ClaudeUsageWindow) {
		if window == nil {
			return
		}
		nonNilWindows++
		if !window.ResetsAt.IsZero() && (latestReset.IsZero() || window.ResetsAt.After(latestReset)) {
			latestReset = window.ResetsAt
		}
		if window.Utilization >= 95 {
			exhaustedWindows++
		}
		t.Logf("%s: utilization=%.2f reset=%s", name, window.Utilization, window.ResetsAt.Format(time.RFC3339))
	}

	logWindow("five_hour", details.FiveHour)
	logWindow("seven_day", details.SevenDay)
	logWindow("seven_day_oauth_apps", details.SevenDayOAuthApps)
	logWindow("seven_day_opus", details.SevenDayOpus)
	logWindow("seven_day_sonnet", details.SevenDaySonnet)

	if details.ExtraUsage != nil {
		t.Logf(
			"extra_usage: enabled=%v monthly_limit=%v used_credits=%v utilization=%v",
			details.ExtraUsage.IsEnabled,
			details.ExtraUsage.MonthlyLimit,
			details.ExtraUsage.UsedCredits,
			details.ExtraUsage.Utilization,
		)
	}

	if nonNilWindows == 0 && details.ExtraUsage == nil {
		t.Fatalf("GetClaudeUsage(%q) returned no usage windows", ref)
	}

	t.Logf(
		"live claude usage ok: ref=%s windows=%d exhausted_windows=%d latest_reset=%s",
		ref,
		nonNilWindows,
		exhaustedWindows,
		latestReset.Format(time.RFC3339),
	)
}
