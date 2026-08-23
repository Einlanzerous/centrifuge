package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// chunkTestBody builds a digest-shaped body whose halves carry distinct marker
// words, so a test server can tell which chunk a prompt contains.
func chunkTestBody() string {
	half := func(marker string) string {
		return strings.TrimSpace(strings.Repeat(marker+" news develops here today. ", 10))
	}
	return half("ALPHAMARK") + " " + half("BETAMARK")
}

// chunkServer serves the scorer's calls for chunking tests: the shape probe
// answers shape, and item calls answer by which marker the prompt carries.
// calls counts only item (non-probe) generations.
func chunkServer(t *testing.T, shape string, byMarker map[string]string, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if strings.Contains(req.Prompt, "layout classifier") {
			_ = json.NewEncoder(w).Encode(generateResponse{Response: fmt.Sprintf(`{"shape":%q}`, shape), Done: true})
			return
		}
		calls.Add(1)
		for marker, resp := range byMarker {
			if strings.Contains(req.Prompt, marker) {
				_ = json.NewEncoder(w).Encode(generateResponse{Response: resp, Done: true})
				return
			}
		}
		t.Errorf("item prompt matched no marker: %.80s", req.Prompt)
		_ = json.NewEncoder(w).Encode(generateResponse{Response: `[]`, Done: true})
	}))
}

func item(title, summary string) string {
	return fmt.Sprintf(`{"title":%q,"kind":"story","relevance_score":10,"primary_topic":"t","summary":%q,"snippet":"s"}`, title, summary)
}

func TestScoreDigestIsChunkedAndMerged(t *testing.T) {
	var calls atomic.Int32
	srv := chunkServer(t, "digest", map[string]string{
		"ALPHAMARK": `[` + item("Alpha", "a") + `]`,
		"BETAMARK":  `[` + item("Beta", "b") + `]`,
	}, &calls)
	defer srv.Close()

	body := chunkTestBody()
	scorer := NewScorer(fastClient(srv), nil, WithChunkChars((len(body)+1)/2))
	res, err := scorer.Score(context.Background(), ScoreInput{Body: body})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if res.Shape != ShapeDigest || res.Chunks != 2 {
		t.Errorf("shape = %q chunks = %d, want digest/2", res.Shape, res.Chunks)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("item calls = %d, want 2", n)
	}
	if len(res.Items) != 2 || res.Items[0].Title != "Alpha" || res.Items[1].Title != "Beta" {
		t.Errorf("items = %+v, want Alpha then Beta in reading order", res.Items)
	}
}

func TestScoreEssayNeverChunked(t *testing.T) {
	var calls atomic.Int32
	// The essay path scores the WHOLE body in one call — the prompt carries both
	// markers at once, so key on the second (a chunked call would only see one).
	srv := chunkServer(t, "essay", map[string]string{
		"BETAMARK": `[` + item("The Essay", "one story") + `]`,
	}, &calls)
	defer srv.Close()

	body := chunkTestBody()
	scorer := NewScorer(fastClient(srv), nil, WithChunkChars((len(body)+1)/2))
	res, err := scorer.Score(context.Background(), ScoreInput{Body: body})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if res.Shape != ShapeEssay || res.Chunks != 1 {
		t.Errorf("shape = %q chunks = %d, want essay/1", res.Shape, res.Chunks)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("item calls = %d, want 1 (whole body)", n)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "The Essay" {
		t.Errorf("items = %+v, want the single essay story", res.Items)
	}
}

func TestScoreShortBodySkipsProbe(t *testing.T) {
	var probes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Prompt, "layout classifier") {
			probes.Add(1)
		}
		_ = json.NewEncoder(w).Encode(generateResponse{Response: `[` + item("Only", "s") + `]`, Done: true})
	}))
	defer srv.Close()

	scorer := NewScorer(fastClient(srv), nil, WithChunkChars(5000))
	res, err := scorer.Score(context.Background(), ScoreInput{Body: "tiny body."})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if probes.Load() != 0 {
		t.Error("shape probe ran for a body under the chunk target")
	}
	if res.Shape != ShapeUnknown || res.Chunks != 1 {
		t.Errorf("shape = %q chunks = %d, want unprobed/1", res.Shape, res.Chunks)
	}
}

func TestScoreChunkTruncationSalvagesAcrossChunks(t *testing.T) {
	var calls atomic.Int32
	srv := chunkServer(t, "digest", map[string]string{
		"ALPHAMARK": `[` + item("Alpha", "a") + `]`,
		"BETAMARK":  `[` + item("Beta", "b") + `,{"title":"cut`, // truncated mid-array
	}, &calls)
	defer srv.Close()

	body := chunkTestBody()
	scorer := NewScorer(fastClient(srv), nil, WithChunkChars((len(body)+1)/2))
	res, err := scorer.Score(context.Background(), ScoreInput{Body: body})
	var tr *TruncatedError
	if !errors.As(err, &tr) {
		t.Fatalf("err = %v, want *TruncatedError", err)
	}
	if !strings.Contains(tr.Reason, "1 of 2 chunk(s)") {
		t.Errorf("reason = %q, want chunk attribution", tr.Reason)
	}
	if len(res.Items) != 2 || tr.Recovered != 2 {
		t.Errorf("items = %d recovered = %d, want both complete items kept", len(res.Items), tr.Recovered)
	}
}

func TestScoreAllChunksEmptyIsEmptyError(t *testing.T) {
	var calls atomic.Int32
	srv := chunkServer(t, "digest", map[string]string{
		"ALPHAMARK": `[]`,
		"BETAMARK":  `[]`,
	}, &calls)
	defer srv.Close()

	body := chunkTestBody()
	scorer := NewScorer(fastClient(srv), nil, WithChunkChars((len(body)+1)/2))
	_, err := scorer.Score(context.Background(), ScoreInput{Body: body})
	var ee *EmptyError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v, want *EmptyError when every chunk is empty", err)
	}
}

func TestScoreOneEmptyChunkTolerated(t *testing.T) {
	var calls atomic.Int32
	srv := chunkServer(t, "digest", map[string]string{
		"ALPHAMARK": `[` + item("Alpha", "a") + `]`,
		"BETAMARK":  `[]`, // pure-boilerplate chunk
	}, &calls)
	defer srv.Close()

	body := chunkTestBody()
	scorer := NewScorer(fastClient(srv), nil, WithChunkChars((len(body)+1)/2))
	res, err := scorer.Score(context.Background(), ScoreInput{Body: body})
	if err != nil {
		t.Fatalf("Score: %v (one empty chunk must not fail the newsletter)", err)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "Alpha" {
		t.Errorf("items = %+v, want Alpha alone", res.Items)
	}
}

func TestScoreFailedChunkDoesNotSinkSiblings(t *testing.T) {
	var calls atomic.Int32
	srv := chunkServer(t, "digest", map[string]string{
		"ALPHAMARK": `[` + item("Alpha", "a") + `]`,
		"BETAMARK":  `{"nope":true}`, // terminal validation junk: no array anywhere
	}, &calls)
	defer srv.Close()

	body := chunkTestBody()
	scorer := NewScorer(fastClient(srv), nil, WithChunkChars((len(body)+1)/2))
	res, err := scorer.Score(context.Background(), ScoreInput{Body: body})
	var tr *TruncatedError
	if !errors.As(err, &tr) {
		t.Fatalf("err = %v, want *TruncatedError carrying the chunk loss", err)
	}
	if !strings.Contains(tr.Reason, "1 of 2 chunk(s)") {
		t.Errorf("reason = %q, want chunk attribution", tr.Reason)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "Alpha" {
		t.Errorf("items = %+v, want the healthy chunk's items kept", res.Items)
	}
	if res.FailedChunks != 1 {
		t.Errorf("failedChunks = %d, want 1", res.FailedChunks)
	}
}
