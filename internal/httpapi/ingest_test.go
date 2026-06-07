package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Einlanzerous/centrifuge/internal/config"
	"github.com/Einlanzerous/centrifuge/internal/ingest"
)

const sampleEmail = "From: Example News <news@example.com>\r\n" +
	"Subject: Hello\r\n" +
	"Message-ID: <msg-1@example.com>\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>Hello world</p>\r\n"

func newServer(t *testing.T, token string, ingestor *ingest.Ingestor) *Server {
	t.Helper()
	return NewServer(&config.Config{IngestToken: token}, slog.New(slog.NewTextHandler(io.Discard, nil)), ingestor)
}

func TestIngestRequiresToken(t *testing.T) {
	// Auth runs before any ingestion, so a nil ingestor is fine here.
	s := newServer(t, "secret", nil)
	req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(sampleEmail))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestIngestMalformedBody(t *testing.T) {
	// Token disabled (empty); parse fails before the ingestor is touched.
	s := newServer(t, "", nil)
	req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader("this is not a valid email"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestIngestRawCreatedThenDuplicate(t *testing.T) {
	pool := dbPool(t)
	s := newServer(t, "secret", ingest.NewIngestor(pool))

	post := func() (*httptest.ResponseRecorder, ingestResponse) {
		req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(sampleEmail))
		req.Header.Set("X-Ingest-Token", "secret")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var body ingestResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v (raw %s)", err, rec.Body.String())
		}
		return rec, body
	}

	rec1, first := post()
	if rec1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec1.Code)
	}
	if first.Duplicate {
		t.Error("first ingest should not be a duplicate")
	}
	if first.ID == "" || first.SourceID == "" {
		t.Errorf("missing ids in response: %+v", first)
	}

	rec2, second := post()
	if rec2.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200", rec2.Code)
	}
	if !second.Duplicate {
		t.Error("re-posting the same Message-ID should report duplicate=true")
	}
	if second.ID != first.ID {
		t.Errorf("duplicate id = %s, want %s", second.ID, first.ID)
	}
}

func TestIngestRawTokenViaQuery(t *testing.T) {
	pool := dbPool(t)
	s := newServer(t, "secret", ingest.NewIngestor(pool))

	req := httptest.NewRequest(http.MethodPost, "/ingest?token=secret", strings.NewReader(sampleEmail))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (query-param token)", rec.Code)
	}
}
