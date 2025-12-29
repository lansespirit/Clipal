package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lansespirit/Clipal/internal/config"
	"github.com/lansespirit/Clipal/internal/logger"
)

// ClientType represents the type of CLI client
type ClientType string

const (
	ClientClaudeCode ClientType = "claudecode"
	ClientCodex      ClientType = "codex"
	ClientGemini     ClientType = "gemini"
)

// Router manages multiple client proxies
type Router struct {
	cfg        *config.Config
	configDir  string
	proxies    map[ClientType]*ClientProxy
	server     *http.Server
	mu         sync.RWMutex
	watchMu    sync.Mutex
	watchStop  chan struct{}
	watchDone  chan struct{}
	lastMod    map[string]time.Time
	watchEvery time.Duration
}

// ClientProxy handles requests for a specific client type
type ClientProxy struct {
	clientType       ClientType
	providers        []config.Provider
	currentIndex     int
	countTokensIndex int
	mu               sync.RWMutex
	httpClient       *http.Client
	deactivated      []providerDeactivation
	reactivateAfter  time.Duration
	upstreamIdle     time.Duration
}

// Close releases resources held by the ClientProxy.
func (cp *ClientProxy) Close() {
	if cp.httpClient != nil {
		cp.httpClient.CloseIdleConnections()
	}
}

// NewRouter creates a new Router instance
func NewRouter(cfg *config.Config) *Router {
	reactivateAfter, err := time.ParseDuration(cfg.Global.ReactivateAfter)
	if err != nil || reactivateAfter < 0 {
		reactivateAfter = time.Hour // Default to 1 hour if invalid.
		logger.Warn("invalid reactivate_after %q, defaulting to 1h", cfg.Global.ReactivateAfter)
	}
	upstreamIdle, err := time.ParseDuration(cfg.Global.UpstreamIdleTimeout)
	if err != nil || upstreamIdle < 0 {
		upstreamIdle = 3 * time.Minute
		logger.Warn("invalid upstream_idle_timeout %q, defaulting to 3m", cfg.Global.UpstreamIdleTimeout)
	}
	r := &Router{
		cfg:        cfg,
		configDir:  cfg.ConfigDir(),
		proxies:    make(map[ClientType]*ClientProxy),
		lastMod:    make(map[string]time.Time),
		watchEvery: 5 * time.Second,
	}

	// Initialize client proxies
	claudeProviders := config.GetEnabledProviders(cfg.ClaudeCode)
	if len(claudeProviders) > 0 {
		r.proxies[ClientClaudeCode] = newClientProxy(ClientClaudeCode, claudeProviders, reactivateAfter, upstreamIdle)
	}

	codexProviders := config.GetEnabledProviders(cfg.Codex)
	if len(codexProviders) > 0 {
		r.proxies[ClientCodex] = newClientProxy(ClientCodex, codexProviders, reactivateAfter, upstreamIdle)
	}

	geminiProviders := config.GetEnabledProviders(cfg.Gemini)
	if len(geminiProviders) > 0 {
		r.proxies[ClientGemini] = newClientProxy(ClientGemini, geminiProviders, reactivateAfter, upstreamIdle)
	}

	return r
}

func newClientProxy(clientType ClientType, providers []config.Provider, reactivateAfter time.Duration, upstreamIdle time.Duration) *ClientProxy {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &ClientProxy{
		clientType:       clientType,
		providers:        providers,
		currentIndex:     0,
		countTokensIndex: 0,
		deactivated:      make([]providerDeactivation, len(providers)),
		reactivateAfter:  reactivateAfter,
		upstreamIdle:     upstreamIdle,
		httpClient: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           dialer.DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 2 * time.Minute,
				ExpectContinueTimeout: 1 * time.Second,
				// Keep response bytes unchanged unless the client explicitly asks for compression.
				DisableCompression: true,
			},
		},
	}
}

// Start starts the proxy server
func (r *Router) Start() error {
	port := r.cfg.Global.Port

	mux := http.NewServeMux()
	mux.HandleFunc("/", r.handleRequest)
	mux.HandleFunc("/health", r.handleHealth)

	addr := net.JoinHostPort(r.cfg.Global.ListenAddr, strconv.Itoa(port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	r.mu.Lock()
	r.server = srv
	r.mu.Unlock()

	logger.Info("clipal starting on %s", addr)

	// Log loaded providers
	for clientType, proxy := range r.proxies {
		logger.Info("loaded %d providers for %s", len(proxy.providers), clientType)
	}

	r.startProviderConfigWatcher()

	return srv.ListenAndServe()
}

// Stop gracefully stops the proxy server
func (r *Router) Stop() error {
	r.stopProviderConfigWatcher()
	r.mu.RLock()
	srv := r.server
	r.mu.RUnlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
	return nil
}

func (r *Router) startProviderConfigWatcher() {
	r.watchMu.Lock()
	defer r.watchMu.Unlock()

	if r.configDir == "" || r.watchEvery <= 0 {
		return
	}
	if r.watchStop != nil {
		return
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	r.watchStop = stopCh
	r.watchDone = doneCh

	// Initialize snapshot so we don't reload immediately on start.
	r.snapshotProviderConfigModTimes()

	go func(stopCh <-chan struct{}, doneCh chan struct{}) {
		defer close(doneCh)
		ticker := time.NewTicker(r.watchEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.reloadIfProviderConfigsChanged()
				r.sweepReactivations()
			case <-stopCh:
				return
			}
		}
	}(stopCh, doneCh)
}

func (r *Router) stopProviderConfigWatcher() {
	r.watchMu.Lock()
	stopCh := r.watchStop
	doneCh := r.watchDone
	r.watchMu.Unlock()

	if stopCh == nil {
		return
	}

	select {
	case <-stopCh:
		// already closed
	default:
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}

	r.watchMu.Lock()
	if r.watchStop == stopCh {
		r.watchStop = nil
		r.watchDone = nil
	}
	r.watchMu.Unlock()
}

func (r *Router) providerConfigFiles() []string {
	return []string{"claude-code.yaml", "codex.yaml", "gemini.yaml"}
}

func (r *Router) snapshotProviderConfigModTimes() {
	for _, name := range r.providerConfigFiles() {
		path := filepath.Join(r.configDir, name)
		if fi, err := os.Stat(path); err == nil {
			r.lastMod[path] = fi.ModTime()
		} else {
			delete(r.lastMod, path)
		}
	}
}

func (r *Router) reloadIfProviderConfigsChanged() {
	changed := false
	for _, name := range r.providerConfigFiles() {
		path := filepath.Join(r.configDir, name)
		fi, err := os.Stat(path)
		if err != nil {
			if _, ok := r.lastMod[path]; ok {
				delete(r.lastMod, path)
				changed = true
			}
			continue
		}
		last, ok := r.lastMod[path]
		if !ok || fi.ModTime().After(last) {
			r.lastMod[path] = fi.ModTime()
			changed = true
		}
	}
	if !changed {
		return
	}

	newCfg, err := config.Load(r.configDir)
	if err != nil {
		logger.Warn("provider config reload failed: %v", err)
		return
	}

	// Keep listen settings stable at runtime, but allow log level and failover policy changes.
	r.mu.RLock()
	currentGlobal := r.cfg.Global
	r.mu.RUnlock()

	newCfg.Global.ListenAddr = currentGlobal.ListenAddr
	newCfg.Global.Port = currentGlobal.Port

	if err := newCfg.Validate(); err != nil {
		logger.Warn("provider config reload failed validation: %v", err)
		return
	}

	logger.SetLevel(newCfg.Global.LogLevel)
	reactivateAfter, err := time.ParseDuration(newCfg.Global.ReactivateAfter)
	if err != nil || reactivateAfter < 0 {
		reactivateAfter = time.Hour
		logger.Warn("invalid reactivate_after %q, defaulting to 1h", newCfg.Global.ReactivateAfter)
	}
	upstreamIdle, err := time.ParseDuration(newCfg.Global.UpstreamIdleTimeout)
	if err != nil || upstreamIdle < 0 {
		upstreamIdle = 3 * time.Minute
		logger.Warn("invalid upstream_idle_timeout %q, defaulting to 3m", newCfg.Global.UpstreamIdleTimeout)
	}

	newProxies := make(map[ClientType]*ClientProxy)
	if ps := config.GetEnabledProviders(newCfg.ClaudeCode); len(ps) > 0 {
		newProxies[ClientClaudeCode] = newClientProxy(ClientClaudeCode, ps, reactivateAfter, upstreamIdle)
	}
	if ps := config.GetEnabledProviders(newCfg.Codex); len(ps) > 0 {
		newProxies[ClientCodex] = newClientProxy(ClientCodex, ps, reactivateAfter, upstreamIdle)
	}
	if ps := config.GetEnabledProviders(newCfg.Gemini); len(ps) > 0 {
		newProxies[ClientGemini] = newClientProxy(ClientGemini, ps, reactivateAfter, upstreamIdle)
	}

	r.mu.Lock()
	oldProxies := r.proxies
	r.cfg = newCfg
	r.proxies = newProxies
	r.mu.Unlock()

	// Close old proxies to release idle connections.
	for _, p := range oldProxies {
		p.Close()
	}

	logger.Info("provider configs reloaded from %s", r.configDir)
	for ct, p := range newProxies {
		logger.Info("loaded %d providers for %s", len(p.providers), ct)
	}
}

func (r *Router) sweepReactivations() {
	r.mu.RLock()
	proxies := make([]*ClientProxy, 0, len(r.proxies))
	for _, p := range r.proxies {
		proxies = append(proxies, p)
	}
	r.mu.RUnlock()

	for _, p := range proxies {
		p.reactivateExpired()
	}
}

// handleHealth handles health check requests
func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}

// handleRequest routes requests to the appropriate client proxy
func (r *Router) handleRequest(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	// Determine client type from path prefix
	var clientType ClientType
	var stripPrefix string

	switch {
	case pathMatchesPrefix(path, "/claudecode"):
		clientType = ClientClaudeCode
		stripPrefix = "/claudecode"
	case pathMatchesPrefix(path, "/codex"):
		clientType = ClientCodex
		stripPrefix = "/codex"
	case pathMatchesPrefix(path, "/gemini"):
		clientType = ClientGemini
		stripPrefix = "/gemini"
	default:
		logger.Warn("unknown path prefix: %s", path)
		http.Error(w, "Unknown endpoint. Use /claudecode, /codex, or /gemini", http.StatusNotFound)
		return
	}

	r.mu.RLock()
	proxy, exists := r.proxies[clientType]
	ignoreCountTokensFailover := r.cfg.Global.IgnoreCountTokensFailover
	maxBody := r.cfg.Global.MaxRequestBody
	r.mu.RUnlock()

	if !exists || len(proxy.providers) == 0 {
		logger.Warn("[%s] no providers configured", clientType)
		http.Error(w, fmt.Sprintf("No providers configured for %s", clientType), http.StatusServiceUnavailable)
		return
	}

	if maxBody > 0 {
		req.Body = http.MaxBytesReader(w, req.Body, maxBody)
	}

	// Strip the prefix from the path
	newPath := stripClientPrefix(path, stripPrefix)

	logger.Debug("[%s] request received: %s %s", clientType, req.Method, newPath)

	// Claude Code: treat count_tokens as best-effort to avoid provider switching,
	// which can reduce context-cache effectiveness and increase token usage.
	if clientType == ClientClaudeCode && ignoreCountTokensFailover && isClaudeCountTokensPath(newPath) {
		proxy.forwardCountTokensWithFailover(w, req, newPath)
		return
	}

	// Forward request with failover.
	proxy.forwardWithFailover(w, req, newPath)
}

func isClaudeCountTokensPath(path string) bool {
	return path == "/v1/messages/count_tokens" || path == "/v1/messages/count_tokens/"
}

// createProxyRequest creates a new request to forward to the provider
func (cp *ClientProxy) createProxyRequest(original *http.Request, provider config.Provider, path string, body []byte) (*http.Request, error) {
	targetURL, err := buildTargetURL(provider.BaseURL, path, original.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	// Create the request
	proxyReq, err := http.NewRequestWithContext(original.Context(), original.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Copy headers from original request
	for key, values := range original.Header {
		// Skip hop-by-hop headers
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Add standard proxy headers
	addForwardedHeaders(proxyReq, original)

	// Set/override the API key based on the original header format
	// Claude uses x-api-key or Authorization
	// OpenAI uses Authorization: Bearer
	if provider.APIKey != "" {
		// Check if original request uses x-api-key (Claude style)
		if original.Header.Get("x-api-key") != "" {
			proxyReq.Header.Set("x-api-key", provider.APIKey)
		} else {
			// Default to Bearer token (OpenAI style)
			proxyReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
		}
	}

	// Set content length
	proxyReq.ContentLength = int64(len(body))
	proxyReq.Header.Del("Content-Length")

	return proxyReq, nil
}

// getCurrentIndex returns the current provider index
func (cp *ClientProxy) getCurrentIndex() int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return cp.currentIndex
}

// setCurrentIndex sets the current provider index
func (cp *ClientProxy) setCurrentIndex(index int) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.currentIndex = index
}

// hopByHopHeaders is a set of hop-by-hop headers that should not be forwarded.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// isHopByHopHeader checks if a header is hop-by-hop
func isHopByHopHeader(header string) bool {
	return hopByHopHeaders[http.CanonicalHeaderKey(header)]
}

func pathMatchesPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func stripClientPrefix(path, prefix string) string {
	if path == prefix || path == prefix+"/" {
		return "/"
	}
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

func buildTargetURL(baseURL string, path string, rawQuery string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("base_url is empty")
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base_url %q: %w", baseURL, err)
	}
	if parsedBase.Scheme == "" {
		parsedBase, err = url.Parse("https://" + baseURL)
		if err != nil {
			return "", fmt.Errorf("invalid base_url %q: %w", baseURL, err)
		}
	}
	if parsedBase.Host == "" {
		return "", fmt.Errorf("invalid base_url %q: host is empty", baseURL)
	}

	// Join base path and request path, keeping exactly one slash.
	parsedBase.Path = singleJoiningSlash(parsedBase.Path, path)
	parsedBase.RawQuery = rawQuery
	parsedBase.Fragment = ""

	return parsedBase.String(), nil
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		if a == "" {
			return "/" + b
		}
		return a + "/" + b
	default:
		return a + b
	}
}

func copyHeaders(dst http.Header, src http.Header) {
	for k := range dst {
		dst.Del(k)
	}
	for k, vv := range src {
		if isHopByHopHeader(k) {
			continue
		}
		dst[k] = append([]string(nil), vv...)
	}
}

func addForwardedHeaders(proxyReq *http.Request, original *http.Request) {
	// X-Forwarded-For
	if ip, _, err := net.SplitHostPort(original.RemoteAddr); err == nil && ip != "" {
		prior := proxyReq.Header.Get("X-Forwarded-For")
		if prior != "" {
			proxyReq.Header.Set("X-Forwarded-For", prior+", "+ip)
		} else {
			proxyReq.Header.Set("X-Forwarded-For", ip)
		}
	}

	// X-Forwarded-Proto
	proto := "http"
	if original.TLS != nil {
		proto = "https"
	}
	proxyReq.Header.Set("X-Forwarded-Proto", proto)

	// X-Forwarded-Host
	if original.Host != "" {
		proxyReq.Header.Set("X-Forwarded-Host", original.Host)
	}

	// Forward an explicit port hint (some upstreams log it).
	if host := original.Host; host != "" {
		if _, port, err := netSplitHostPortMaybe(host); err == nil && port != "" {
			proxyReq.Header.Set("X-Forwarded-Port", port)
		}
	}
}

func netSplitHostPortMaybe(hostport string) (host string, port string, err error) {
	// Treat bare host as no port.
	if h, p, e := net.SplitHostPort(hostport); e == nil {
		return h, p, nil
	}
	return hostport, "", nil
}
