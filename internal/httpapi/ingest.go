package httpapi

import (
	"io"
	"net/http"

	"github.com/Einlanzerous/centrifuge/internal/ingest"
)

// maxIngestBytes caps an ingest request body. Newsletters embed inline images as
// base64, so the limit is generous, but it still bounds memory and keeps the
// endpoint from being a junk sink.
const maxIngestBytes = 25 << 20 // 25 MiB

// ingestResponse is the JSON returned by both ingestion endpoints. Duplicate is
// true when the message deduplicated against an existing delivery.
type ingestResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	SourceID  string `json:"source_id"`
	Duplicate bool   `json:"duplicate"`
}

// handleIngestRaw is the production webhook: it accepts a raw RFC822 / multipart
// message, parses it through the MIME parser, and hands it to the ingestion
// core. This is the contract the future live auto-feed (CTFG-24) POSTs to.
func (s *Server) handleIngestRaw(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxIngestBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large or unreadable")
		return
	}

	msg, err := ingest.ParseRFC822(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "malformed message: "+err.Error())
		return
	}

	s.ingestMessage(w, r, *msg)
}

// ingestMessage runs a normalized message through the ingestion core and writes
// the standard response. Shared by the raw and JSON-drop entrypoints.
func (s *Server) ingestMessage(w http.ResponseWriter, r *http.Request, msg ingest.InboundMessage) {
	if s.ingestor == nil {
		writeError(w, http.StatusServiceUnavailable, "ingestion is not available")
		return
	}

	res, err := s.ingestor.Ingest(r.Context(), msg)
	if err != nil {
		s.logger.Error("ingest", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to persist message")
		return
	}

	writeJSON(w, http.StatusOK, ingestResponse{
		ID:        res.Newsletter.ID,
		Status:    res.Newsletter.ProcessingStatus,
		SourceID:  res.SourceID,
		Duplicate: !res.Created,
	})
}
