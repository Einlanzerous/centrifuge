package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Einlanzerous/centrifuge/internal/config"
	"github.com/Einlanzerous/centrifuge/internal/version"
)

// What the delivery reconciler actually parses. Decoded into a raw map rather
// than into healthzResponse so this asserts the JSON *wire* shape — reusing the
// struct would make a renamed json tag invisible, which is exactly the break
// that would silently stop observations.
func decodeHealthz(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not JSON: %v (body=%s)", err, body)
	}
	return got
}

func TestHandleHealthzReportsBuildIdentity(t *testing.T) {
	origVersion, origCommit := version.Version, version.Commit
	t.Cleanup(func() { version.Version, version.Commit = origVersion, origCommit })

	const sha = "6fc737b5f2767cb6a3536cd0b2b0901ee620b841"
	version.Version, version.Commit = "1.5.8", sha

	// A zero Server is enough: handleHealthz deliberately consults nothing.
	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		// The reconciler falls back to sniffing a leading "<" when the content
		// type is absent and files a markup body as `unreachable`. This header
		// is what keeps a healthy service off the red list.
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	got := decodeHealthz(t, rec.Body.Bytes())
	if got["version"] != "1.5.8" {
		t.Errorf("version = %v, want 1.5.8", got["version"])
	}
	if got["sha"] != sha {
		t.Errorf("sha = %v, want the full 40-char commit %s", got["sha"], sha)
	}
	// The pre-existing field keeps working — the compose HEALTHCHECK reads this
	// endpoint and should not notice the change.
	if got["status"] != "ok" {
		t.Errorf("status = %v, want ok", got["status"])
	}
}

// An unstamped build must say so, and `sha` must be JSON null rather than "".
// The reconciler reads a blank version as "reports no version"; an empty STRING
// for either field is a third state nothing expects.
func TestHandleHealthzUnstampedBuild(t *testing.T) {
	origVersion, origCommit := version.Version, version.Commit
	t.Cleanup(func() { version.Version, version.Commit = origVersion, origCommit })

	// Exactly what an image built with no --build-arg produces.
	version.Version, version.Commit = "", ""

	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	got := decodeHealthz(t, rec.Body.Bytes())
	if got["version"] != "dev" {
		t.Errorf("version = %v, want dev — a blank ARG must not report an empty version", got["version"])
	}
	if _, present := got["sha"]; !present {
		t.Error("sha key is missing; the contract wants it present and null")
	}
	if got["sha"] != nil {
		t.Errorf("sha = %v, want null", got["sha"])
	}
}

// /healthz must stay unguarded. It is registered outside `requireIngestToken`,
// and the reconciler, the compose HEALTHCHECK and uptime-kuma all reach it with
// no credentials — a token requirement here reads as `unreachable` on the
// matrix for a service that is running perfectly well.
func TestHealthzNeedsNoIngestToken(t *testing.T) {
	s := NewServer(&config.Config{IngestToken: "a-token-that-is-set"}, nil, nil, nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d with an ingest token configured, want 200", rec.Code)
	}
}
