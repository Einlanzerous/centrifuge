package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// partialServer returns an httptest server whose generate endpoint replies with
// a done:false envelope carrying resp — Ollama's server-side cut (CTFG-63).
func partialServer(t *testing.T, resp string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(generateResponse{Response: resp, Done: false})
	}))
}

func TestScorePartialEnvelopeSalvagesAsTruncated(t *testing.T) {
	// The partial carries one complete item and a second cut mid-object; Score
	// must salvage the finished item and surface the cut through the truncation
	// path, attributed to the done:false envelope.
	srv := partialServer(t, `[{"title":"kept","kind":"story","relevance_score":10,"primary_topic":"t","summary":"s"},{"title":"cut`)
	defer srv.Close()

	res, err := NewScorer(fastClient(srv), nil).Score(context.Background(), ScoreInput{Body: "b"})
	items := res.Items
	var tr *TruncatedError
	if !errors.As(err, &tr) {
		t.Fatalf("err = %v, want *TruncatedError", err)
	}
	if tr.Reason == "" {
		t.Error("Reason is empty, want done:false attribution")
	}
	if len(items) != 1 || items[0].Title != "kept" {
		t.Errorf("items = %+v, want the one complete item salvaged", items)
	}
}

func TestScorePartialEnvelopeCompleteJSONStillTruncated(t *testing.T) {
	// Even when the partial text happens to parse as a complete array, done:false
	// means the model was cut before finishing — later items may be missing — so
	// the result must not be mistaken for a complete segmentation.
	srv := partialServer(t, `[{"title":"only","kind":"story","relevance_score":10,"primary_topic":"t","summary":"s"}]`)
	defer srv.Close()

	res, err := NewScorer(fastClient(srv), nil).Score(context.Background(), ScoreInput{Body: "b"})
	items := res.Items
	var tr *TruncatedError
	if !errors.As(err, &tr) {
		t.Fatalf("err = %v, want *TruncatedError", err)
	}
	if tr.Recovered != 1 || len(items) != 1 {
		t.Errorf("recovered = %d, items = %d, want 1 and 1", tr.Recovered, len(items))
	}
}

func TestRawPartialEnvelopeReturnsPartialText(t *testing.T) {
	srv := partialServer(t, `[{"title":"partial`)
	defer srv.Close()

	raw, err := NewScorer(fastClient(srv), nil).Raw(context.Background(), ScoreInput{Body: "b"})
	var pe *PartialError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want *PartialError", err)
	}
	if raw != `[{"title":"partial` {
		t.Errorf("raw = %q, want the partial text passed through", raw)
	}
}
