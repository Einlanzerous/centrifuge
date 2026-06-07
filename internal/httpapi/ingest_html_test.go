package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Einlanzerous/centrifuge/internal/ingest"
)

func postJSON(t *testing.T, s *Server, body string) (*httptest.ResponseRecorder, ingestResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ingest/html", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	var out ingestResponse
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec, out
}

func TestIngestHTMLRequiresHTML(t *testing.T) {
	s := newServer(t, "", nil) // auth off; validation fails before the ingestor
	rec, _ := postJSON(t, s, `{"subject":"no body"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestIngestHTMLInvalidReceivedAt(t *testing.T) {
	s := newServer(t, "", nil)
	rec, _ := postJSON(t, s, `{"html":"<p>x</p>","received_at":"yesterday"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for non-RFC3339 received_at", rec.Code)
	}
}

func TestIngestHTMLCreatedAndDedupe(t *testing.T) {
	pool := dbPool(t)
	s := newServer(t, "", ingest.NewIngestor(pool))

	body := `{"html":"<h1>Morning Brew</h1><p>markets up</p>",` +
		`"subject":"Daily","from":"Morning Brew <crew@morningbrew.com>",` +
		`"received_at":"2026-06-06T12:00:00Z"}`

	rec1, first := postJSON(t, s, body)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec1.Code)
	}
	if first.Duplicate {
		t.Error("first drop should not be a duplicate")
	}
	if first.ID == "" || first.SourceID == "" {
		t.Errorf("missing ids: %+v", first)
	}

	// No Message-ID: re-dropping identical HTML dedupes on the content hash.
	rec2, second := postJSON(t, s, body)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200", rec2.Code)
	}
	if !second.Duplicate {
		t.Error("identical re-drop should report duplicate=true")
	}
	if second.ID != first.ID {
		t.Errorf("duplicate id = %s, want %s", second.ID, first.ID)
	}
}
