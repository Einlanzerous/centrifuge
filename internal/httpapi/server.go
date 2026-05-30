// Package httpapi exposes the centrifuge HTTP surface.
//
// Phase 0 provides only the readiness endpoint; ingestion and reflection
// handlers are added in later phases. The package is intentionally
// source-agnostic: handlers persist normalized items and never score inline.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Einlanzerous/centrifuge/internal/config"
)

// Server bundles the HTTP handler with the configuration and logger it needs.
type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	handler http.Handler
}

// NewServer constructs a Server with all routes registered.
func NewServer(cfg *config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:    cfg,
		logger: logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.handler = mux

	return s
}

// Handler returns the http.Handler that serves the API. Server itself also
// implements http.Handler via ServeHTTP, so it can be passed directly to
// http.Server.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
