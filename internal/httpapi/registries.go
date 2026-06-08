package httpapi

import (
	"net/http"

	"github.com/Einlanzerous/centrifuge/internal/db"
)

// handleTopics serves the topic registry: every dynamic primary_topic on
// curated stories, with its story count and stable palette color.
func (s *Server) handleTopics(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	topics, err := db.NewStoryRepo(s.pool).TopicRegistry(r.Context())
	if err != nil {
		s.logger.Error("topics: registry", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load topics")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": toTopics(topics)})
}

// handleSources serves the source registry / "best sources" rollup: per-source
// story counts, average relevance, and engagement, best average first.
func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	stats, err := db.NewStoryRepo(s.pool).SourceAggregate(r.Context())
	if err != nil {
		s.logger.Error("sources: aggregate", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load sources")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": toSources(stats)})
}
