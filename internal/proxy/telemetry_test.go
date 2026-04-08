package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/telemetry"
)

func TestRecordCompletedUsageCountsOnlySuccessfulGenerationRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clientType  ClientType
		capability  RequestCapability
		method      string
		statusCode  int
		wantRequest int64
		wantSuccess int64
	}{
		{
			name:        "openai responses success counts",
			clientType:  ClientOpenAI,
			capability:  CapabilityOpenAIResponses,
			method:      http.MethodPost,
			statusCode:  http.StatusOK,
			wantRequest: 1,
			wantSuccess: 1,
		},
		{
			name:        "openai responses failure skips success count",
			clientType:  ClientOpenAI,
			capability:  CapabilityOpenAIResponses,
			method:      http.MethodPost,
			statusCode:  http.StatusBadRequest,
			wantRequest: 1,
			wantSuccess: 0,
		},
		{
			name:        "openai models success skips success count",
			clientType:  ClientOpenAI,
			capability:  CapabilityOpenAIModels,
			method:      http.MethodGet,
			statusCode:  http.StatusOK,
			wantRequest: 1,
			wantSuccess: 0,
		},
		{
			name:        "claude messages success counts",
			clientType:  ClientClaude,
			capability:  CapabilityClaudeMessages,
			method:      http.MethodPost,
			statusCode:  http.StatusOK,
			wantRequest: 1,
			wantSuccess: 1,
		},
		{
			name:        "gemini generate content success counts",
			clientType:  ClientGemini,
			capability:  CapabilityGeminiGenerateContent,
			method:      http.MethodPost,
			statusCode:  http.StatusOK,
			wantRequest: 1,
			wantSuccess: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, err := telemetry.NewStore("")
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}
			cp := newClientProxy(tc.clientType, config.ClientModeAuto, "", []config.Provider{
				{Name: "p1", BaseURL: "https://example.com", APIKey: "provider-key", Priority: 1},
			}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{}, store)

			req := httptest.NewRequest(tc.method, "http://proxy/test", nil)
			req = withRequestContext(req, RequestContext{
				ClientType: tc.clientType,
				Capability: tc.capability,
			})

			cp.recordCompletedUsage(req, "p1", tc.statusCode, telemetry.UsageSnapshot{
				UsageDelta: telemetry.UsageDelta{
					InputTokens:  3,
					OutputTokens: 4,
				},
			}, time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC))

			got, ok := store.ProviderSnapshot(string(tc.clientType), "p1")
			if !ok {
				t.Fatalf("ProviderSnapshot missing")
			}
			if got.RequestCount != tc.wantRequest || got.SuccessCount != tc.wantSuccess {
				t.Fatalf("snapshot = %#v", got)
			}
		})
	}
}
