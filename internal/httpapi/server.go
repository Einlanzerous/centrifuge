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
	"github.com/Einlanzerous/centrifuge/internal/version"
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

// healthzResponse is the body of `GET /healthz`.
//
// ── Why this grew a version and a sha (CTFG-64) ────────────────────────────
//
// Switchyard's delivery reconciler polls this endpoint and records what is
// actually running, which is the observed half of the estate's delivery ledger
// (SWY-192 defines the contract; SERV-128 owns the rollout across services).
// Before these two fields centrifuge probed as `no_version`: reachable and
// speaking, but unable to say WHICH build was speaking — so no deploy of
// centrifuge could ever be corroborated.
//
// The field names and types are the contract, not a local choice:
//
//	version  bare semver ("1.5.8") or the literal "dev". Never a "v" prefix —
//	         it is compared with strict equality against the image's
//	         org.opencontainers.image.version label, which docker's
//	         metadata-action stamps bare. A prefix here files every deploy
//	         report as `claimed_not_confirmed`, permanently.
//	sha      the full 40-char commit, or JSON null. Never abbreviated: the
//	         cross-service comparison is an equality test, not a prefix match.
//
// A struct rather than the previous map[string]string, because `sha` has to be
// able to marshal as null and a map of strings cannot express that.
//
// This body also covers `centrifuge-frontend`, which is a static bundle and
// cannot answer a probe at all (tier B). It ships from this same release and
// pins to the same tag, so the backend's row already says what release the
// pair is on — a separate row for the frontend would measure nothing new.
type healthzResponse struct {
	Status  string  `json:"status"`
	Version string  `json:"version"`
	SHA     *string `json:"sha"`
}

// handleHealthz answers the liveness probe and the build-identity contract.
//
// Deliberately consults neither Postgres nor Ollama. Liveness and readiness
// answer different questions, and a liveness probe that fails on a degraded
// dependency gets the container killed and restarted at exactly the moment
// somebody wants to look at it.
//
// That is also why there is only a 200 path here rather than the 200/503 pair
// the contract permits. The contract's rule is that a 503 must carry the SAME
// body shape — a degraded service is still running a version, and it is the one
// most worth identifying — so if a readiness verdict is ever added it belongs
// in this struct on both branches, not in a second shape.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	id := version.Get()
	writeJSON(w, http.StatusOK, healthzResponse{
		Status:  "ok",
		Version: id.Version,
		SHA:     id.SHA,
	})
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
