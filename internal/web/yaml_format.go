package web

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/lansespirit/Clipal/internal/config"
)

func formatClientConfigYAML(clientType string, cc config.ClientConfig) []byte {
	var b bytes.Buffer

	header := clientConfigHeader(clientType)
	b.WriteString("# " + header + "\n")
	b.WriteString("# Providers are sorted by priority (lower number = higher priority)\n\n")

	if len(cc.Providers) == 0 {
		b.WriteString("providers: []\n")
		return b.Bytes()
	}

	providers := make([]config.Provider, len(cc.Providers))
	copy(providers, cc.Providers)
	sort.SliceStable(providers, func(i, j int) bool {
		return providers[i].Priority < providers[j].Priority
	})

	uniquePriorities := make([]int, 0, len(providers))
	for _, p := range providers {
		if len(uniquePriorities) == 0 || uniquePriorities[len(uniquePriorities)-1] != p.Priority {
			uniquePriorities = append(uniquePriorities, p.Priority)
		}
	}
	cut1, cut2 := priorityTierCuts(uniquePriorities)

	b.WriteString("providers:\n")

	prevTier := 0
	for i, p := range providers {
		if i > 0 {
			b.WriteString("\n")
		}

		tier := priorityTier(p.Priority, cut1, cut2)
		if tier != prevTier {
			b.WriteString(fmt.Sprintf("  # Priority level %d: %s\n", tier, priorityTierLabel(tier, cut1, cut2)))
			prevTier = tier
		}

		b.WriteString(fmt.Sprintf("  - name: %s\n", yamlDoubleQuote(p.Name)))
		b.WriteString(fmt.Sprintf("    base_url: %s\n", yamlDoubleQuote(p.BaseURL)))
		b.WriteString(fmt.Sprintf("    api_key: %s\n", yamlDoubleQuote(p.APIKey)))
		b.WriteString(fmt.Sprintf("    priority: %d\n", p.Priority))
		b.WriteString(fmt.Sprintf("    enabled: %v\n", p.IsEnabled()))
	}

	b.WriteString("\n")
	return b.Bytes()
}

func formatGlobalConfigYAML(gc config.GlobalConfig) []byte {
	var b bytes.Buffer

	b.WriteString("# Global configuration for clipal\n")
	b.WriteString("# Managed by the web UI (comments kept for readability)\n\n")

	b.WriteString(fmt.Sprintf("listen_addr: %s\n", strings.TrimSpace(gc.ListenAddr)))
	b.WriteString(fmt.Sprintf("port: %d\n", gc.Port))
	b.WriteString(fmt.Sprintf("log_level: %s # debug | info | warn | error\n", strings.TrimSpace(string(gc.LogLevel))))
	b.WriteString(fmt.Sprintf("reactivate_after: %s # set to 0 to disable temporary deactivation\n", strings.TrimSpace(gc.ReactivateAfter)))
	b.WriteString("# Cancel an upstream attempt if no response body bytes are received for this long.\n")
	b.WriteString("# Useful for SSE streams that can hang after headers. Set to 0 to disable.\n")
	b.WriteString(fmt.Sprintf("upstream_idle_timeout: %s\n", strings.TrimSpace(gc.UpstreamIdleTimeout)))
	b.WriteString("# Max request body size in bytes (clipal buffers request bodies for retries).\n")
	b.WriteString(fmt.Sprintf("max_request_body_bytes: %d\n\n", gc.MaxRequestBody))

	b.WriteString("# Default: <config-dir>/logs (e.g. ~/.clipal/logs)\n")
	b.WriteString(fmt.Sprintf("log_dir: %s\n", yamlMaybeQuoteEmpty(gc.LogDir)))
	b.WriteString(fmt.Sprintf("log_retention_days: %d\n", gc.LogRetentionDays))
	b.WriteString(fmt.Sprintf("log_stdout: %v\n\n", boolPtrOrTrue(gc.LogStdout)))

	b.WriteString("# Claude Code: if true, /v1/messages/count_tokens failures won't affect the main conversation provider.\n")
	b.WriteString(fmt.Sprintf("ignore_count_tokens_failover: %v\n\n", gc.IgnoreCountTokensFailover))

	b.WriteString("# Desktop notifications (best-effort, cross-platform via beeep)\n")
	b.WriteString("notifications:\n")
	b.WriteString(fmt.Sprintf("  enabled: %v\n", gc.Notifications.Enabled))
	b.WriteString(fmt.Sprintf("  min_level: %s # debug | info | warn | error\n", strings.TrimSpace(string(gc.Notifications.MinLevel))))
	b.WriteString(fmt.Sprintf("  provider_switch: %v\n", boolPtrOrTrue(gc.Notifications.ProviderSwitch)))

	b.WriteString("\n")
	return b.Bytes()
}

func clientConfigHeader(clientType string) string {
	switch clientType {
	case "claude-code":
		return "Claude Code API providers configuration"
	case "codex":
		return "Codex CLI API providers configuration"
	case "gemini":
		return "Gemini CLI API providers configuration"
	default:
		return fmt.Sprintf("%s providers configuration", clientType)
	}
}

func priorityTierCuts(uniquePriorities []int) (cut1 int, cut2 int) {
	if len(uniquePriorities) == 0 {
		return 0, 0
	}
	if len(uniquePriorities) == 1 {
		return uniquePriorities[0], uniquePriorities[0]
	}
	if len(uniquePriorities) == 2 {
		return uniquePriorities[0], uniquePriorities[1]
	}

	minP := uniquePriorities[0]
	maxP := uniquePriorities[len(uniquePriorities)-1]
	if minP >= maxP {
		return minP, maxP
	}

	// Split the numeric priority range into 3 buckets and snap to existing values.
	// Tiers may be empty if the configured priorities are sparse (e.g. 1,2,1000),
	// which is intentional for readability.
	target1 := float64(minP) + float64(maxP-minP)/3.0
	target2 := float64(minP) + 2.0*float64(maxP-minP)/3.0

	cut1 = minP
	cut2 = minP
	for _, p := range uniquePriorities {
		if float64(p) <= target1 {
			cut1 = p
		}
		if float64(p) <= target2 {
			cut2 = p
		}
	}
	if cut2 < cut1 {
		cut2 = cut1
	}
	return cut1, cut2
}

func priorityTier(priority int, cut1 int, cut2 int) int {
	switch {
	case priority <= cut1:
		return 1
	case priority <= cut2:
		return 2
	default:
		return 3
	}
}

func priorityTierLabel(tier int, cut1 int, cut2 int) string {
	switch tier {
	case 1:
		return fmt.Sprintf("highest priority (primary) [priority<=%d]", cut1)
	case 2:
		return fmt.Sprintf("fallback [priority<=%d]", cut2)
	default:
		return fmt.Sprintf("backup / other [priority>%d]", cut2)
	}
}

func yamlMaybeQuoteEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return `""`
	}
	return s
}

func yamlDoubleQuote(s string) string {
	// YAML double-quoted scalar with minimal escaping.
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				// Control characters should be escaped in YAML double quotes.
				b.WriteString(fmt.Sprintf(`\x%02x`, c))
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
