package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/config"
	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// apiServer builds a Server wired to the test pool, with the read API enabled.
func apiServer(pool *pgxpool.Pool) *Server {
	cfg := &config.Config{CORSAllowOrigin: "*"}
	return NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, pool)
}

func ptr[T any](v T) *T { return &v }

// seedScored inserts one scored story under a named source and returns its id.
func seedScored(t *testing.T, pool *pgxpool.Pool, source, identity, title, topic string, score int, received time.Time) string {
	t.Helper()
	ctx := context.Background()
	src, err := db.NewSourceRepo(pool).GetOrCreate(ctx, db.SourceKindNewsletter, identity, source)
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	rec := received
	nl, _, err := db.NewNewsletterRepo(pool).Insert(ctx, &db.Newsletter{
		SourceID:   src.ID,
		RawHTML:    ptr("<p>" + title + " body</p>"),
		ReceivedAt: &rec,
	})
	if err != nil {
		t.Fatalf("seed newsletter: %v", err)
	}
	st := db.NewStoryRepo(pool)
	out, err := st.InsertMany(ctx, nl.ID, src.ID, []db.Story{{Position: 0, Kind: db.KindStory, Title: ptr(title), Snippet: ptr(title + " snippet")}})
	if err != nil {
		t.Fatalf("seed story: %v", err)
	}
	if err := st.ScoreUpdate(ctx, out[0].ID, db.Score{
		Summary: title + " summary", RelevanceScore: score, PrimaryTopic: topic,
		Labels: []string{"x"}, Model: "m", PromptVersion: "v1",
	}); err != nil {
		t.Fatalf("score story: %v", err)
	}
	return out[0].ID
}

// do runs a request against the server and returns the recorder.
func do(s *Server, method, target string, body string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func TestTodayAndSeen(t *testing.T) {
	pool := dbPool(t)
	s := apiServer(pool)
	now := time.Now().UTC()
	seedScored(t, pool, "Brew", "brew@x", "AI agents land", "AI engineering", 90, now)
	seedScored(t, pool, "Brew", "brew@x", "Transit bill", "transit", 70, now)

	// First visit: never looked, so every scored story is new.
	rec := do(s, http.MethodGet, "/api/today", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("today status %d: %s", rec.Code, rec.Body)
	}
	var first todayResponse
	decode(t, rec, &first)
	if first.IsEmpty || len(first.Items) != 2 {
		t.Fatalf("first today: empty=%v items=%d", first.IsEmpty, len(first.Items))
	}
	if first.Since != nil {
		t.Fatalf("since should be nil on first visit: %v", first.Since)
	}
	if len(first.Topics) != 2 || first.Topics[0].Count != 1 {
		t.Fatalf("topic chips: %+v", first.Topics)
	}
	if first.Items[0].TopicColor == "" {
		t.Fatalf("item missing topic color: %+v", first.Items[0])
	}

	// Mark seen.
	if rec := do(s, http.MethodPost, "/api/today/seen", ""); rec.Code != http.StatusOK {
		t.Fatalf("seen status %d: %s", rec.Code, rec.Body)
	}

	// Second visit: nothing scored after "seen" → empty, no items.
	var second todayResponse
	decode(t, do(s, http.MethodGet, "/api/today", ""), &second)
	if !second.IsEmpty || len(second.Items) != 0 {
		t.Fatalf("second today should be empty: empty=%v items=%d", second.IsEmpty, len(second.Items))
	}
	if second.Since == nil {
		t.Fatalf("since should be set after seen")
	}

	// Brief fills the empty state with older unsurfaced stories.
	var brief todayResponse
	decode(t, do(s, http.MethodGet, "/api/today?brief=1", ""), &brief)
	if !brief.Brief || len(brief.Items) != 2 {
		t.Fatalf("brief: brief=%v items=%d", brief.Brief, len(brief.Items))
	}
}

func TestArchiveGroupingAndFilter(t *testing.T) {
	pool := dbPool(t)
	s := apiServer(pool)
	now := time.Now().UTC()
	seedScored(t, pool, "Brew", "brew@x", "Today story", "AI engineering", 90, now)
	seedScored(t, pool, "Obs", "obs@x", "Old nuclear story", "nuclear", 70, now.Add(-72*time.Hour))

	var resp archiveResponse
	decode(t, do(s, http.MethodGet, "/api/archive", ""), &resp)
	if resp.Total != 2 || len(resp.Days) != 2 {
		t.Fatalf("archive total=%d days=%d", resp.Total, len(resp.Days))
	}
	if resp.Days[0].Label != "Today" {
		t.Fatalf("first day label = %q, want Today", resp.Days[0].Label)
	}
	if len(resp.Sources) != 2 || len(resp.Topics) != 2 {
		t.Fatalf("facets: sources=%d topics=%d", len(resp.Sources), len(resp.Topics))
	}

	// Topic filter.
	var filtered archiveResponse
	decode(t, do(s, http.MethodGet, "/api/archive?topic=nuclear", ""), &filtered)
	if filtered.Total != 1 || filtered.Days[0].Items[0].Title != "Old nuclear story" {
		t.Fatalf("topic filter: %+v", filtered)
	}

	// Bad date param.
	if rec := do(s, http.MethodGet, "/api/archive?from=notadate", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad from: status %d", rec.Code)
	}
}

func TestItemDetailAndEngagement(t *testing.T) {
	pool := dbPool(t)
	s := apiServer(pool)
	id := seedScored(t, pool, "Brew", "brew@x", "Readable", "AI engineering", 90, time.Now().UTC())

	// Detail carries the raw HTML body and marks the item read.
	var item itemDTO
	rec := do(s, http.MethodGet, "/api/items/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status %d: %s", rec.Code, rec.Body)
	}
	decode(t, rec, &item)
	if !strings.Contains(item.Body, "Readable body") {
		t.Fatalf("body missing: %q", item.Body)
	}
	if !item.Read {
		t.Fatalf("item should be read after detail fetch")
	}

	// Bookmark toggles on then off.
	var bm struct {
		Bookmarked bool `json:"bookmarked"`
	}
	decode(t, do(s, http.MethodPost, "/api/items/"+id+"/bookmark", ""), &bm)
	if !bm.Bookmarked {
		t.Fatalf("bookmark should be on")
	}
	decode(t, do(s, http.MethodPost, "/api/items/"+id+"/bookmark", ""), &bm)
	if bm.Bookmarked {
		t.Fatalf("bookmark should toggle off")
	}

	// Rate up, then none.
	var rt struct {
		Rating string `json:"rating"`
	}
	decode(t, do(s, http.MethodPost, "/api/items/"+id+"/rate", `{"rating":"up"}`), &rt)
	if rt.Rating != "up" {
		t.Fatalf("rating = %q", rt.Rating)
	}
	decode(t, do(s, http.MethodPost, "/api/items/"+id+"/rate", `{"rating":"none"}`), &rt)
	if rt.Rating != "none" {
		t.Fatalf("rating = %q after clear", rt.Rating)
	}
	if rec := do(s, http.MethodPost, "/api/items/"+id+"/rate", `{"rating":"sideways"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad rating: status %d", rec.Code)
	}

	// Mark as ad removes it from the curated archive.
	if rec := do(s, http.MethodPost, "/api/items/"+id+"/mark-ad", ""); rec.Code != http.StatusOK {
		t.Fatalf("mark-ad status %d", rec.Code)
	}
	var arch archiveResponse
	decode(t, do(s, http.MethodGet, "/api/archive", ""), &arch)
	if arch.Total != 0 {
		t.Fatalf("ad should be excluded from archive, got %d", arch.Total)
	}
}

func TestItemNotFoundAndBadID(t *testing.T) {
	pool := dbPool(t)
	s := apiServer(pool)
	if rec := do(s, http.MethodGet, "/api/items/00000000-0000-0000-0000-000000000000", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: status %d", rec.Code)
	}
	if rec := do(s, http.MethodGet, "/api/items/not-a-uuid", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: status %d", rec.Code)
	}
}

func TestTopicsAndSources(t *testing.T) {
	pool := dbPool(t)
	s := apiServer(pool)
	seedScored(t, pool, "Brew", "brew@x", "A", "AI engineering", 90, time.Now().UTC())
	seedScored(t, pool, "Brew", "brew@x", "B", "AI engineering", 50, time.Now().UTC())

	var topics struct {
		Topics []topicDTO `json:"topics"`
	}
	decode(t, do(s, http.MethodGet, "/api/topics", ""), &topics)
	if len(topics.Topics) != 1 || topics.Topics[0].Count != 2 || topics.Topics[0].Color == "" {
		t.Fatalf("topics: %+v", topics.Topics)
	}

	var sources struct {
		Sources []sourceDTO `json:"sources"`
	}
	decode(t, do(s, http.MethodGet, "/api/sources", ""), &sources)
	if len(sources.Sources) != 1 || sources.Sources[0].AvgRelevance == nil || *sources.Sources[0].AvgRelevance != 70 {
		t.Fatalf("sources: %+v", sources.Sources)
	}
}

func TestFeed(t *testing.T) {
	pool := dbPool(t)
	s := apiServer(pool)
	seedScored(t, pool, "Brew", "brew@x", "Feed headline", "AI engineering", 90, time.Now().UTC())

	rec := do(s, http.MethodGet, "/feed.xml", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("feed status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "rss+xml") {
		t.Fatalf("feed content-type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<rss") || !strings.Contains(body, "Feed headline") {
		t.Fatalf("feed body unexpected: %s", body)
	}
}

func TestReadEndpointsWithoutPool(t *testing.T) {
	s := apiServer(nil)
	if rec := do(s, http.MethodGet, "/api/today", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no-pool today: status %d", rec.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	s := apiServer(nil)
	rec := do(s, http.MethodOptions, "/api/today", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS origin header")
	}
}
