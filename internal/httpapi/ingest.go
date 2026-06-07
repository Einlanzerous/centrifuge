package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

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

// ingestHTMLRequest is the JSON body of POST /ingest/html. Only html is
// required; the rest let a hand-drop carry the metadata a real email would have.
type ingestHTMLRequest struct {
	HTML       string `json:"html"`
	Subject    string `json:"subject"`
	From       string `json:"from"`
	FromName   string `json:"from_name"`
	MessageID  string `json:"message_id"`
	ReceivedAt string `json:"received_at"`
}

// handleIngestHTML is the hand-drop entrypoint: a JSON {html, subject?, from?,
// received_at?} is wrapped as an InboundMessage and run through the same
// dedupe + persistence path as the raw webhook. It exists to fire real
// newsletters at the pipeline (and prove scoring) before any live feed exists.
func (s *Server) handleIngestHTML(w http.ResponseWriter, r *http.Request) {
	var req ingestHTMLRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIngestBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.HTML) == "" {
		writeError(w, http.StatusBadRequest, "html is required")
		return
	}

	msg := ingest.InboundMessage{
		Subject:   req.Subject,
		RawHTML:   req.HTML,
		MessageID: req.MessageID,
	}
	// `from` may be a bare address or a "Name <addr>" pair; parse leniently.
	if req.From != "" {
		if addr, err := mail.ParseAddress(req.From); err == nil {
			msg.FromAddr, msg.FromName = addr.Address, addr.Name
		} else {
			msg.FromAddr = req.From
		}
	}
	if req.FromName != "" {
		msg.FromName = req.FromName
	}
	if req.ReceivedAt != "" {
		t, err := time.Parse(time.RFC3339, req.ReceivedAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "received_at must be RFC3339")
			return
		}
		msg.ReceivedAt = t
	}

	s.ingestMessage(w, r, msg)
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
