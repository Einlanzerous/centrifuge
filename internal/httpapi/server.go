// Package httpapi exposes the centrifuge HTTP surface.
//
// It provides the readiness endpoint plus the dual-format ingestion endpoints.
// The package is intentionally source-agnostic: handlers normalize inbound
// items and hand them to the ingestion core, which persists them and never
// scores inline.
package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Einlanzerous/centrifuge/internal/config"
	"github.com/Einlanzerous/centrifuge/internal/ingest"
)

// Server bundles the HTTP handler with the configuration, logger, and ingestion
// core it needs.
type Server struct {
	cfg      *config.Config
	logger   *slog.Logger
	ingestor *ingest.Ingestor
	handler  http.Handler
}

// NewServer constructs a Server with all routes registered. ingestor backs the
// /ingest endpoints; it may be nil when those endpoints are not needed.
func NewServer(cfg *config.Config, logger *slog.Logger, ingestor *ingest.Ingestor) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:      cfg,
		logger:   logger,
		ingestor: ingestor,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /ingest", s.requireIngestToken(s.handleIngestRaw))
	mux.HandleFunc("POST /ingest/html", s.requireIngestToken(s.handleIngestHTML))
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

// requireIngestToken guards ingestion endpoints with the shared INGEST_TOKEN.
// The token is accepted in the X-Ingest-Token header or a ?token= query param.
// When INGEST_TOKEN is unset the guard is disabled (local-dev convenience), so
// production must set it — the endpoint accepts arbitrary content and must not
// be an open relay for junk. The comparison is constant-time.
func (s *Server) requireIngestToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.IngestToken != "" {
			provided := r.Header.Get("X-Ingest-Token")
			if provided == "" {
				provided = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.IngestToken)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid or missing ingest token")
				return
			}
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
