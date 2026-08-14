package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/YashGholap/ProxyRack-Manager/internal/config"
	"github.com/YashGholap/ProxyRack-Manager/internal/pool"
)

type Handler struct {
	pool   *pool.Manager
	cfg    *config.Config
	client *http.Client
}

func New(p *pool.Manager, cfg *config.Config) *Handler {
	return &Handler{
		pool: p,
		cfg:  cfg,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/scrape", h.handleScrape)
	mux.HandleFunc("/raw", h.handleRaw)
	mux.HandleFunc("/status", h.handleStatus)
	mux.HandleFunc("/config", h.handleConfig)
	mux.HandleFunc("/health", h.handleHealth)
}

func (h *Handler) handleScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		h.writeError(w, http.StatusBadRequest, "missing 'url' parameter")
		return
	}

	service := r.Header.Get("X-Service-Name")
	if service == "" {
		service = r.RemoteAddr
	}

	noWait := strings.EqualFold(r.Header.Get("X-No-Wait"), "true")

	var release func()
	var err error

	if noWait {
		release, err = h.pool.TryAcquire(service)
	} else {
		release, err = h.pool.Acquire(service)
	}

	if err != nil {
		status := h.pool.Status()
		h.writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":     "thread pool exhausted",
			"message":   err.Error(),
			"available": status.Available,
			"max":       status.Max,
			"in_use":    status.InUse,
			"waiting":   status.Waiting,
		})
		return
	}
	defer release()

	proxrackURL := h.buildProxyrackURL(r, targetURL)

	start := time.Now()
	resp, err := h.client.Get(proxrackURL)
	if err != nil {
		log.Printf("[%s] proxyrack error for %s: %v (took %v)", service, targetURL, err, time.Since(start))
		h.writeError(w, http.StatusBadGateway, fmt.Sprintf("proxyrack request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	log.Printf("[%s] proxyrack %d for %s (took %v)", service, resp.StatusCode, truncate(targetURL, 80), time.Since(start))

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	status := h.pool.Status()
	w.Header().Set("X-Pool-Available", fmt.Sprintf("%d", status.Available))
	w.Header().Set("X-Pool-InUse", fmt.Sprintf("%d", status.InUse))
	w.Header().Set("X-Pool-Max", fmt.Sprintf("%d", status.Max))

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handler) handleRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
		return
	}

	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		h.writeError(w, http.StatusBadRequest, "missing 'url' parameter")
		return
	}

	service := r.Header.Get("X-Service-Name")
	if service == "" {
		service = r.RemoteAddr
	}

	noWait := strings.EqualFold(r.Header.Get("X-No-Wait"), "true")

	var release func()
	var err error
	if noWait {
		release, err = h.pool.TryAcquire(service)
	} else {
		release, err = h.pool.Acquire(service)
	}
	if err != nil {
		status := h.pool.Status()
		h.writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":     "thread pool exhuasted",
			"available": status.Available,
			"max":       status.Max,
			"in_use":    status.InUse,
		})
		return
	}

	defer release()

	resp, err := h.client.Get(targetURL)
	if err != nil {
		h.writeError(w, http.StatusBadGateway, fmt.Sprintf("request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	h.writeJSON(w, http.StatusOK, h.pool.Status())
}

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.writeError(w, http.StatusMethodNotAllowed, "PUT only")
		return
	}

	var req struct {
		MaxThreads *int `json:"max_threads"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.MaxThreads != nil {
		if *req.MaxThreads < 1 {
			h.writeError(w, http.StatusBadRequest, "max_threads must be >= 1")
			return
		}
		h.pool.UpdateMaxThreads(*req.MaxThreads)
		log.Printf("max_threads updated to %d", *req.MaxThreads)
	}

	h.writeJSON(w, http.StatusOK, h.pool.Status())
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *Handler) buildProxyrackURL(r *http.Request, targetURL string) string {
	params := url.Values{}
	params.Set("api_key", h.cfg.ProxyrackAPIKey)
	params.Set("url", targetURL)

	for key, values := range r.URL.Query() {
		if key == "url" {
			continue
		}
		for _, v := range values {
			params.Set(key, v)
		}
	}

	// Default premium to true if not specified
	if params.Get("premium") == "" {
		params.Set("premium", "true")
	}

	return h.cfg.ProxyrackBaseURL + "?" + params.Encode()
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
