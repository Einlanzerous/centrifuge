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
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server bundles the HTTP handler with the configuration, logger, datastore,
// and ingestion core it needs.
type Server struct {
	cfg      *config.Config
	logger   *slog.Logger
	ingestor *ingest.Ingestor
	pool     *pgxpool.Pool
	handler  http.Handler
}

// NewServer constructs a Server with all routes registered. ingestor backs the
// /ingest endpoints and pool backs the read API; either may be nil when those
// routes are not needed (the read endpoints report 503 without a pool).
func NewServer(cfg *config.Config, logger *slog.Logger, ingestor *ingest.Ingestor, pool *pgxpool.Pool) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:      cfg,
		logger:   logger,
		ingestor: ingestor,
		pool:     pool,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /ingest", s.requireIngestToken(s.handleIngestRaw))
	mux.HandleFunc("POST /ingest/html", s.requireIngestToken(s.handleIngestHTML))

	// Read API backing the UI (CTFG-26).
	mux.HandleFunc("GET /api/today", s.handleToday)
	mux.HandleFunc("POST /api/today/seen", s.handleTodaySeen)
	mux.HandleFunc("GET /api/archive", s.handleArchive)
	mux.HandleFunc("GET /api/items/{id}", s.handleItem)
	mux.HandleFunc("POST /api/items/{id}/bookmark", s.handleBookmark)
	mux.HandleFunc("POST /api/items/{id}/rate", s.handleRate)
	mux.HandleFunc("POST /api/items/{id}/mark-ad", s.handleMarkAd)
	mux.HandleFunc("GET /api/topics", s.handleTopics)
	mux.HandleFunc("GET /api/sources", s.handleSources)
	mux.HandleFunc("GET /feed.xml", s.handleFeed)

	s.handler = s.withCORS(mux)

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

// withCORS allows the browser frontend (CTFG-27), which may be served from a
// different origin, to call the API. The allowed origin is configurable
// (CORS_ALLOW_ORIGIN, default "*"); preflight OPTIONS requests short-circuit
// here. The API carries no cookies/credentials yet, so "*" is safe.
func (s *Server) withCORS(next http.Handler) http.Handler {
	origin := s.cfg.CORSAllowOrigin
	if origin == "" {
		origin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Ingest-Token")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireDB guards the read endpoints: without a pool there is nothing to read.
func (s *Server) requireDB(w http.ResponseWriter) bool {
	if s.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "datastore is not available")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
