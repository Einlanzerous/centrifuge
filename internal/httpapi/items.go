package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/jackc/pgx/v5"
)

// uuidRe validates a path id before it reaches the database, so a malformed id
// is a clean 400 rather than a Postgres syntax error.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// itemID extracts and validates the {id} path value, writing the error response
// itself when invalid (returns "", false).
func (s *Server) itemID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return "", false
	}
	return id, true
}

// handleItem returns a single story with its raw HTML body for the Reader
// modal, and stamps it opened (the natural "read" engagement signal).
func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id, ok := s.itemID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	repo := db.NewStoryRepo(s.pool)

	v, err := repo.GetEnriched(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		s.logger.Error("item: get", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load item")
		return
	}

	// Reading is opening — best-effort, never blocks the response. Reflect the
	// stamp in the returned view so the client sees it as read immediately.
	if v.OpenedAt == nil {
		now := time.Now().UTC()
		if err := repo.MarkOpened(ctx, id, now); err != nil {
			s.logger.Warn("item: mark opened", "item", id, "error", err)
		} else {
			v.OpenedAt = &now
		}
	}

	writeJSON(w, http.StatusOK, toItem(*v, true))
}

// handleBookmark toggles a story's bookmark flag, returning the new state. The
// toggle is also an engagement signal (CTFG-29).
func (s *Server) handleBookmark(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id, ok := s.itemID(w, r)
	if !ok {
		return
	}
	bookmarked, err := db.NewStoryRepo(s.pool).ToggleBookmark(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		s.logger.Error("bookmark: toggle", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update bookmark")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "bookmarked": bookmarked})
}

type rateRequest struct {
	Rating string `json:"rating"` // "up" | "down" | "none"
}

// handleRate records a thumbs up/down/none rating, an engagement signal that
// feeds the focus model (CTFG-28 / CTFG-29).
func (s *Server) handleRate(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id, ok := s.itemID(w, r)
	if !ok {
		return
	}

	var req rateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	repo := db.NewStoryRepo(s.pool)
	ctx := r.Context()
	var err error
	switch req.Rating {
	case "up":
		err = repo.SetRating(ctx, id, 1)
	case "down":
		err = repo.SetRating(ctx, id, -1)
	case "none", "":
		err = repo.ClearRating(ctx, id)
	default:
		writeError(w, http.StatusBadRequest, `rating must be "up", "down", or "none"`)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		s.logger.Error("rate: set", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update rating")
		return
	}

	rating := req.Rating
	if rating == "" {
		rating = "none"
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "rating": rating})
}

// handleMarkAd applies the reader's "mark as ad" correction: it flips the
// story's kind to ad (removing it from the curated feeds) and is a strong
// negative engagement signal (CTFG-29).
func (s *Server) handleMarkAd(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id, ok := s.itemID(w, r)
	if !ok {
		return
	}
	err := db.NewStoryRepo(s.pool).SetKind(r.Context(), id, db.KindAd)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		s.logger.Error("mark-ad: set kind", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to mark as ad")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "kind": db.KindAd})
}
