package web

import (
	"embed"
	"net"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

// Handler manages HTTP routes for the web management interface
type Handler struct {
	api *API
}

// NewHandler creates a new web handler
func NewHandler(configDir, version string) *Handler {
	return &Handler{
		api: NewAPI(configDir, version),
	}
}

func isLoopbackRemote(remoteAddr string) bool {
	host := strings.TrimSpace(remoteAddr)
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else {
		// Be defensive: RemoteAddr can be "IP" without port or "[::1]".
		host = strings.Trim(host, "[]")
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *Handler) localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The management interface is designed for local use only (no auth).
		// Enforce loopback access even if the proxy listens on 0.0.0.0/::.
		if !isLoopbackRemote(r.RemoteAddr) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeError(w, "forbidden: management interface is localhost-only", http.StatusForbidden)
			} else {
				http.Error(w, "forbidden", http.StatusForbidden)
			}
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next(w, r)
	}
}

// RegisterRoutes registers all web management routes to the provided mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Static files (UI)
	mux.HandleFunc("/", h.localOnly(h.serveIndex))
	mux.HandleFunc("/static/", h.localOnly(h.serveStatic))

	// API routes
	mux.HandleFunc("/api/config/global", h.localOnly(h.api.HandleGetGlobalConfig))
	mux.HandleFunc("/api/config/global/update", h.localOnly(h.api.HandleUpdateGlobalConfig))
	mux.HandleFunc("/api/config/export", h.localOnly(h.api.HandleExportConfig))

	mux.HandleFunc("/api/providers/", h.localOnly(h.routeProviders))
	mux.HandleFunc("/api/status", h.localOnly(h.api.HandleGetStatus))
}

// serveIndex serves the main management interface HTML
func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}

	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// serveStatic serves static assets (CSS, JS)
func (h *Handler) serveStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	if path == "" || path == "/" {
		http.NotFound(w, r)
		return
	}

	data, err := staticFiles.ReadFile("static/" + path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set content type based on file extension
	if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(path, ".css") {
		w.Header().Set("Content-Type", "text/css")
	}

	w.Write(data)
}

// routeProviders routes provider-related requests to appropriate handlers
func (h *Handler) routeProviders(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()

	// Check if this is a reorder request
	if strings.HasSuffix(path, "/_reorder") {
		h.api.HandleReorderProviders(w, r)
		return
	}

	// Extract client type and provider name
	clientType := extractClientType(path)
	_, providerName := extractClientAndProvider(path)

	if providerName != "" {
		// Operations on specific provider
		switch r.Method {
		case http.MethodPut:
			h.api.HandleUpdateProvider(w, r)
		case http.MethodDelete:
			h.api.HandleDeleteProvider(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	} else if clientType != "" {
		// Operations on provider list
		switch r.Method {
		case http.MethodGet:
			h.api.HandleGetProviders(w, r)
		case http.MethodPost:
			h.api.HandleAddProvider(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	} else {
		writeError(w, "invalid request path", http.StatusBadRequest)
	}
}
