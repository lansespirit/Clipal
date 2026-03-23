package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
)

func newUnifiedIngressTestRouter() *Router {
	cfg := &config.Config{
		Global: config.GlobalConfig{
			ListenAddr:            "127.0.0.1",
			Port:                  3333,
			LogLevel:              config.LogLevelDebug,
			ReactivateAfter:       "1h",
			UpstreamIdleTimeout:   "0",
			ResponseHeaderTimeout: "2m",
			MaxRequestBody:        32 * 1024 * 1024,
		},
		ClaudeCode: config.ClientConfig{
			Mode: config.ClientModeAuto,
			Providers: []config.Provider{
				{Name: "claude", BaseURL: "http://claude", APIKey: "k-claude", Priority: 1},
			},
		},
		Codex: config.ClientConfig{
			Mode: config.ClientModeAuto,
			Providers: []config.Provider{
				{Name: "codex", BaseURL: "http://codex", APIKey: "k-codex", Priority: 1},
			},
		},
		Gemini: config.ClientConfig{
			Mode: config.ClientModeAuto,
			Providers: []config.Provider{
				{Name: "gemini", BaseURL: "http://gemini", APIKey: "k-gemini", Priority: 1},
			},
		},
	}

	return NewRouter(cfg)
}

func installMarkerTransport(cp *ClientProxy, host string, body string, calls *int32) {
	cp.httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != host {
			return nil, io.ErrUnexpectedEOF
		}
		atomic.AddInt32(calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		}, nil
	})
}

func TestClipalClaudeRequestUsesClaudePool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/messages", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "claude-ok" {
		t.Fatalf("body: got %q want %q", got, "claude-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 1 {
		t.Fatalf("claude calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 0 {
		t.Fatalf("gemini calls: got %d want 0", got)
	}
}

func TestClipalResponsesRequestUsesCodexPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/responses", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "codex-ok" {
		t.Fatalf("body: got %q want %q", got, "codex-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 1 {
		t.Fatalf("codex calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 0 {
		t.Fatalf("gemini calls: got %d want 0", got)
	}
}

func TestClipalResponsesRequestRecordsCapabilityInRuntime(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var codexCalls int32
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/responses", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}

	snap := router.RuntimeSnapshot()
	lastRequest := snap.Clients[ClientCodex].LastRequest
	if lastRequest == nil {
		t.Fatalf("expected last request to be recorded")
	}

	field := reflect.ValueOf(*lastRequest).FieldByName("Capability")
	if !field.IsValid() {
		t.Fatalf("expected RequestOutcomeEvent to expose Capability")
	}
	if got := field.String(); got != "openai_responses" {
		t.Fatalf("capability: got %q want %q", got, "openai_responses")
	}
}

func TestUnknownEndpointSuggestsClipalIngress(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/unknown", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", rr.Result().StatusCode, http.StatusNotFound)
	}
	if got := rr.Body.String(); !bytes.Contains([]byte(got), []byte("/clipal")) {
		t.Fatalf("body: got %q want to mention /clipal", got)
	}
}

func TestClipalResponsesRoutingDoesNotShiftChatCursor(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()
	router.proxies[ClientCodex] = newClientProxy(ClientCodex, config.ClientModeAuto, "", []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})

	var p1ResponsesCalls, p1ChatCalls, p2ResponsesCalls, p2ChatCalls int32
	router.proxies[ClientCodex].httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			switch r.URL.Path {
			case "/v1/responses":
				atomic.AddInt32(&p1ResponsesCalls, 1)
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("down"))),
				}, nil
			case "/v1/chat/completions":
				atomic.AddInt32(&p1ChatCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("chat-p1"))),
				}, nil
			}
		case "p2":
			switch r.URL.Path {
			case "/v1/responses":
				atomic.AddInt32(&p2ResponsesCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("responses-p2"))),
				}, nil
			case "/v1/chat/completions":
				atomic.AddInt32(&p2ChatCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("chat-p2"))),
				}, nil
			}
		}
		return nil, io.ErrUnexpectedEOF
	})

	rr1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/responses", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr1, req1)
	if rr1.Result().StatusCode != http.StatusOK {
		t.Fatalf("responses status: got %d want %d body=%s", rr1.Result().StatusCode, http.StatusOK, rr1.Body.String())
	}
	if got := rr1.Body.String(); got != "responses-p2" {
		t.Fatalf("responses body: got %q want %q", got, "responses-p2")
	}

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/chat/completions", bytes.NewReader([]byte(`{"x":2}`)))
	router.handleRequest(rr2, req2)
	if rr2.Result().StatusCode != http.StatusOK {
		t.Fatalf("chat status: got %d want %d body=%s", rr2.Result().StatusCode, http.StatusOK, rr2.Body.String())
	}
	if got := rr2.Body.String(); got != "chat-p1" {
		t.Fatalf("chat body: got %q want %q", got, "chat-p1")
	}
	if got := atomic.LoadInt32(&p1ResponsesCalls); got != 1 {
		t.Fatalf("p1 responses calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&p2ResponsesCalls); got != 1 {
		t.Fatalf("p2 responses calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&p1ChatCalls); got != 1 {
		t.Fatalf("p1 chat calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&p2ChatCalls); got != 0 {
		t.Fatalf("p2 chat calls: got %d want 0", got)
	}
}

func TestClipalImagesRequestUsesCodexPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/images/edits", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "codex-ok" {
		t.Fatalf("body: got %q want %q", got, "codex-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 1 {
		t.Fatalf("codex calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 0 {
		t.Fatalf("gemini calls: got %d want 0", got)
	}
}

func TestClipalAudioRequestUsesCodexPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/audio/transcriptions", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "codex-ok" {
		t.Fatalf("body: got %q want %q", got, "codex-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 1 {
		t.Fatalf("codex calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 0 {
		t.Fatalf("gemini calls: got %d want 0", got)
	}
}

func TestClipalFilesRequestUsesCodexPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/clipal/v1/files", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "codex-ok" {
		t.Fatalf("body: got %q want %q", got, "codex-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 1 {
		t.Fatalf("codex calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 0 {
		t.Fatalf("gemini calls: got %d want 0", got)
	}
}

func TestClipalGeminiRequestUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1beta/models/gemini-2.5-pro:generateContent", bytes.NewReader([]byte(`{"contents":[]}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestClipalGeminiV1RequestUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1/models/gemini-2.5-pro:generateContent", bytes.NewReader([]byte(`{"contents":[]}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestClipalGeminiGenerateDoesNotShiftStreamCursor(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()
	router.proxies[ClientGemini] = newClientProxy(ClientGemini, config.ClientModeAuto, "", []config.Provider{
		{Name: "p1", BaseURL: "http://p1", APIKey: "k1", Priority: 1},
		{Name: "p2", BaseURL: "http://p2", APIKey: "k2", Priority: 2},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})

	var p1GenerateCalls, p1StreamCalls, p2GenerateCalls, p2StreamCalls int32
	router.proxies[ClientGemini].httpClient.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Host {
		case "p1":
			switch r.URL.Path {
			case "/v1beta/models/gemini-2.5-pro:generateContent":
				atomic.AddInt32(&p1GenerateCalls, 1)
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("down"))),
				}, nil
			case "/v1beta/models/gemini-2.5-pro:streamGenerateContent":
				atomic.AddInt32(&p1StreamCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("stream-p1"))),
				}, nil
			}
		case "p2":
			switch r.URL.Path {
			case "/v1beta/models/gemini-2.5-pro:generateContent":
				atomic.AddInt32(&p2GenerateCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("generate-p2"))),
				}, nil
			case "/v1beta/models/gemini-2.5-pro:streamGenerateContent":
				atomic.AddInt32(&p2StreamCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte("stream-p2"))),
				}, nil
			}
		}
		return nil, io.ErrUnexpectedEOF
	})

	rr1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1beta/models/gemini-2.5-pro:generateContent", bytes.NewReader([]byte(`{"contents":[]}`)))
	router.handleRequest(rr1, req1)
	if rr1.Result().StatusCode != http.StatusOK {
		t.Fatalf("generate status: got %d want %d body=%s", rr1.Result().StatusCode, http.StatusOK, rr1.Body.String())
	}
	if got := rr1.Body.String(); got != "generate-p2" {
		t.Fatalf("generate body: got %q want %q", got, "generate-p2")
	}

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/v1beta/models/gemini-2.5-pro:streamGenerateContent", bytes.NewReader([]byte(`{"contents":[]}`)))
	router.handleRequest(rr2, req2)
	if rr2.Result().StatusCode != http.StatusOK {
		t.Fatalf("stream status: got %d want %d body=%s", rr2.Result().StatusCode, http.StatusOK, rr2.Body.String())
	}
	if got := rr2.Body.String(); got != "stream-p1" {
		t.Fatalf("stream body: got %q want %q", got, "stream-p1")
	}
	if got := atomic.LoadInt32(&p1GenerateCalls); got != 1 {
		t.Fatalf("p1 generate calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&p2GenerateCalls); got != 1 {
		t.Fatalf("p2 generate calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&p1StreamCalls); got != 1 {
		t.Fatalf("p1 stream calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&p2StreamCalls); got != 0 {
		t.Fatalf("p2 stream calls: got %d want 0", got)
	}
}

func TestClipalModelsRequestUsesCodexPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/clipal/v1/models", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "codex-ok" {
		t.Fatalf("body: got %q want %q", got, "codex-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 1 {
		t.Fatalf("codex calls: got %d want 1", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 0 {
		t.Fatalf("gemini calls: got %d want 0", got)
	}
}

func TestClipalGeminiModelsListUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/clipal/v1beta/models", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestClipalGeminiModelGetUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/clipal/v1/models/gemini-2.5-flash-image", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestClipalGeminiFilesRequestUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/clipal/v1beta/files", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestClipalGeminiUploadFilesRequestUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/clipal/upload/v1beta/files", bytes.NewReader([]byte("file-bytes")))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestClipalGeminiCachedContentsRequestUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/clipal/v1beta/cachedContents", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestClipalGeminiTunedModelsRequestUsesGeminiPool(t *testing.T) {
	t.Parallel()

	router := newUnifiedIngressTestRouter()

	var claudeCalls, codexCalls, geminiCalls int32
	installMarkerTransport(router.proxies[ClientClaudeCode], "claude", "claude-ok", &claudeCalls)
	installMarkerTransport(router.proxies[ClientCodex], "codex", "codex-ok", &codexCalls)
	installMarkerTransport(router.proxies[ClientGemini], "gemini", "gemini-ok", &geminiCalls)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://proxy/clipal/v1beta/tunedModels", nil)
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "gemini-ok" {
		t.Fatalf("body: got %q want %q", got, "gemini-ok")
	}
	if got := atomic.LoadInt32(&claudeCalls); got != 0 {
		t.Fatalf("claude calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&codexCalls); got != 0 {
		t.Fatalf("codex calls: got %d want 0", got)
	}
	if got := atomic.LoadInt32(&geminiCalls); got != 1 {
		t.Fatalf("gemini calls: got %d want 1", got)
	}
}

func TestCompatibilityAliasUnknownSubpathStillForwards(t *testing.T) {
	t.Parallel()

	cp := newClientProxy(ClientCodex, config.ClientModeAuto, "", []config.Provider{
		{Name: "codex", BaseURL: "http://codex", APIKey: "k-codex", Priority: 1},
	}, time.Hour, 0, testResponseHeaderTimeout, circuitBreakerConfig{})
	var codexCalls int32
	installMarkerTransport(cp, "codex", "codex-ok", &codexCalls)

	router := &Router{
		cfg: &config.Config{
			Global: config.GlobalConfig{
				ListenAddr:      "127.0.0.1",
				Port:            3333,
				LogLevel:        config.LogLevelDebug,
				ReactivateAfter: "1h",
				MaxRequestBody:  32 * 1024 * 1024,
			},
		},
		proxies: map[ClientType]*ClientProxy{
			ClientCodex: cp,
		},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy/codex/v1/images", bytes.NewReader([]byte(`{"x":1}`)))
	router.handleRequest(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rr.Result().StatusCode, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "codex-ok" {
		t.Fatalf("body: got %q want %q", got, "codex-ok")
	}
	if got := atomic.LoadInt32(&codexCalls); got != 1 {
		t.Fatalf("codex calls: got %d want 1", got)
	}
}
