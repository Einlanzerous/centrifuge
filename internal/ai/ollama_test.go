package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastClient builds a Client pointed at srv with retry backoff stripped so the
// retry tests don't actually sleep.
func fastClient(srv *httptest.Server, opts ...Option) *Client {
	base := []Option{WithHTTPClient(srv.Client()), WithRetryWait(0)}
	return NewClient(srv.URL, "test-model", append(base, opts...)...)
}

func TestGenerateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("path = %q, want /api/generate", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		var req generateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("model = %q", req.Model)
		}
		if req.Format != "json" {
			t.Errorf("format = %q, want json", req.Format)
		}
		if req.Stream {
			t.Error("stream = true, want false")
		}
		_ = json.NewEncoder(w).Encode(generateResponse{Response: `[{"title":"x"}]`, Done: true})
	}))
	defer srv.Close()

	got, err := fastClient(srv).Generate(context.Background(), "prompt", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != `[{"title":"x"}]` {
		t.Errorf("response = %q", got)
	}
}

func TestGenerateRetriesOn5xxThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			http.Error(w, "model loading", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(generateResponse{Response: `{"ok":true}`})
	}))
	defer srv.Close()

	got, err := fastClient(srv, WithMaxRetries(3)).Generate(context.Background(), "p", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != `{"ok":true}` {
		t.Errorf("response = %q", got)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("calls = %d, want 3 (two 503s then success)", n)
	}
}

func TestGenerateExhaustsRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "still down", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := fastClient(srv, WithMaxRetries(2)).Generate(context.Background(), "p", nil)
	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TransportError", err)
	}
	if te.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", te.StatusCode)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("calls = %d, want 3 (1 + 2 retries)", n)
	}
}

func TestGenerate4xxNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "bad model", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fastClient(srv, WithMaxRetries(3)).Generate(context.Background(), "p", nil)
	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TransportError", err)
	}
	if te.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", te.StatusCode)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("calls = %d, want 1 (4xx is terminal)", n)
	}
}

func TestGenerateDecodeErrorNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, "this is not json")
	}))
	defer srv.Close()

	_, err := fastClient(srv, WithMaxRetries(3)).Generate(context.Background(), "p", nil)
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want *DecodeError", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("calls = %d, want 1 (decode error is terminal)", n)
	}
}

func TestGenerateEmptyResponseIsDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(generateResponse{Response: "   "})
	}))
	defer srv.Close()

	_, err := fastClient(srv).Generate(context.Background(), "p", nil)
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want *DecodeError for empty response", err)
	}
}

func TestGenerateContextCancel(t *testing.T) {
	// release unblocks the handler at cleanup so srv.Close() never waits on an
	// in-flight request (the server won't notice the client's disconnect on its
	// own while the handler does no I/O).
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := fastClient(srv).Generate(ctx, "p", nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !strings.Contains(err.Error(), "transport") {
		t.Errorf("err = %v, want transport error", err)
	}
}
