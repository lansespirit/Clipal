package oauth

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLiveGeminiUsageFetch(t *testing.T) {
	ref := strings.TrimSpace(os.Getenv("CLIPAL_LIVE_GEMINI_USAGE_REF"))
	if ref == "" {
		t.Skip("set CLIPAL_LIVE_GEMINI_USAGE_REF to run live Gemini usage fetch")
	}

	configDir := strings.TrimSpace(os.Getenv("CLIPAL_LIVE_CONFIG_DIR"))
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

	details, err := svc.GetGeminiUsage(ctx, ref)
	if err != nil {
		t.Fatalf("GetGeminiUsage(%q): %v", ref, err)
	}
	if details == nil {
		t.Fatalf("GetGeminiUsage(%q) returned nil details", ref)
	}
	if len(details.Buckets) == 0 {
		t.Fatalf("GetGeminiUsage(%q) returned no buckets", ref)
	}

	models := make(map[string]struct{})
	withRemaining := 0
	withReset := 0
	exhausted := 0
	var latestReset time.Time
	for _, bucket := range details.Buckets {
		if bucket.ModelID != "" {
			models[bucket.ModelID] = struct{}{}
		}
		if bucket.RemainingAmount != nil || bucket.RemainingFraction != nil {
			withRemaining++
		}
		if !bucket.ResetTime.IsZero() {
			withReset++
			if latestReset.IsZero() || bucket.ResetTime.After(latestReset) {
				latestReset = bucket.ResetTime
			}
		}
		if liveGeminiBucketExhausted(bucket) {
			exhausted++
		}
	}

	if withRemaining == 0 {
		t.Fatalf("GetGeminiUsage(%q) returned buckets without remaining quota data", ref)
	}

	modelList := make([]string, 0, len(models))
	for model := range models {
		modelList = append(modelList, model)
	}
	sort.Strings(modelList)

	t.Logf(
		"live gemini usage ok: ref=%s buckets=%d models=%v buckets_with_reset=%d exhausted_buckets=%d latest_reset=%s",
		ref,
		len(details.Buckets),
		modelList,
		withReset,
		exhausted,
		latestReset.Format(time.RFC3339),
	)
}

func liveGeminiBucketExhausted(bucket GeminiUsageBucket) bool {
	if bucket.RemainingFraction != nil && *bucket.RemainingFraction <= 0.05 {
		return true
	}
	return bucket.RemainingAmount != nil && *bucket.RemainingAmount <= 0
}
