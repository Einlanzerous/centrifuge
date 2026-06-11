package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stubScorer is a deterministic stand-in for *ai.Scorer.
type stubScorer struct {
	items []ai.ScoredItem
	err   error
	calls int
}

func (s *stubScorer) Score(_ context.Context, _ ai.ScoreInput) ([]ai.ScoredItem, error) {
	s.calls++
	return s.items, s.err
}

func (s *stubScorer) Model() string { return "stub-model" }

func quietWorker(pool *pgxpool.Pool, scorer Scorer) *Worker {
	return New(pool, scorer, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
}

// seedPending inserts a source and a pending_scoring newsletter, returning it.
func seedPending(t *testing.T, pool *pgxpool.Pool, subject, body string) db.Newsletter {
	t.Helper()
	ctx := context.Background()
	src, err := db.NewSourceRepo(pool).GetOrCreate(ctx, db.SourceKindNewsletter, "news@example.com", "Example News")
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	subj, bod := subject, body
	nl, _, err := db.NewNewsletterRepo(pool).Insert(ctx, &db.Newsletter{
		SourceID:         src.ID,
		Subject:          &subj,
		BodyText:         &bod,
		ProcessingStatus: db.StatusPending,
	})
	if err != nil {
		t.Fatalf("seed newsletter: %v", err)
	}
	return *nl
}

func statusOf(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	nl, err := db.NewNewsletterRepo(pool).GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("get newsletter: %v", err)
	}
	return nl.ProcessingStatus
}

func TestProcessOneSegmentsScoresAndCompletes(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	nl := seedPending(t, pool, "Digest", "lots of content")

	scorer := &stubScorer{items: []ai.ScoredItem{
		{Title: "Real story", Snippet: "s", Kind: ai.KindStory, Summary: "A summary.",
			RelevanceScore: 88, PrimaryTopic: "AI engineering", Labels: []string{"llm"}},
		{Title: "Sponsor", Snippet: "buy", Kind: ai.KindAd, RelevanceScore: 0},
		{Title: "Quick hit", Snippet: "blip", Kind: ai.KindBlurb, RelevanceScore: 20},
	}}

	if err := quietWorker(pool, scorer).processOne(ctx, nl); err != nil {
		t.Fatalf("processOne: %v", err)
	}

	if got := statusOf(t, pool, nl.ID); got != db.StatusScored {
		t.Errorf("status = %q, want scored", got)
	}

	stories, err := db.NewStoryRepo(pool).ListByNewsletter(ctx, nl.ID)
	if err != nil {
		t.Fatalf("list stories: %v", err)
	}
	if len(stories) != 3 {
		t.Fatalf("got %d stories, want 3", len(stories))
	}
	// Position order preserved.
	for i, s := range stories {
		if s.Position != i {
			t.Errorf("story %d position = %d", i, s.Position)
		}
	}
	// Story-kind item fully scored.
	story := stories[0]
	if story.Kind != db.KindStory || story.RelevanceScore == nil || *story.RelevanceScore != 88 {
		t.Errorf("story[0] not scored: %+v", story)
	}
	if story.Model == nil || *story.Model != "stub-model" {
		t.Errorf("story[0] model = %v", story.Model)
	}
	if story.PromptVersion == nil || *story.PromptVersion != ai.PromptVersion {
		t.Errorf("story[0] prompt_version = %v", story.PromptVersion)
	}
	if story.Summary == nil || *story.Summary != "A summary." {
		t.Errorf("story[0] summary = %v", story.Summary)
	}
	if len(story.Labels) != 1 || story.Labels[0] != "llm" {
		t.Errorf("story[0] labels = %v", story.Labels)
	}
	// Ad and blurb persisted but unscored.
	if stories[1].Kind != db.KindAd || stories[1].RelevanceScore != nil {
		t.Errorf("ad should be unscored: %+v", stories[1])
	}
	if stories[2].Kind != db.KindBlurb || stories[2].RelevanceScore != nil {
		t.Errorf("blurb should be unscored: %+v", stories[2])
	}
}

func TestProcessOneReScoreIsIdempotentAndCarriesEngagement(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	nl := seedPending(t, pool, "Digest", "lots of content")

	const keepURL = "https://ex.com/keep"
	scorer := &stubScorer{items: []ai.ScoredItem{
		{Title: "Keep me", URL: keepURL, Kind: ai.KindStory, Summary: "s", RelevanceScore: 80, PrimaryTopic: "tech"},
		{Title: "Sponsor", Kind: ai.KindAd, RelevanceScore: 0},
	}}
	w := quietWorker(pool, scorer)
	repo := db.NewStoryRepo(pool)

	// First scoring.
	if err := w.processOne(ctx, nl); err != nil {
		t.Fatalf("processOne #1: %v", err)
	}
	first, _ := repo.ListByNewsletter(ctx, nl.ID)
	if len(first) != 2 {
		t.Fatalf("after first score: %d stories, want 2", len(first))
	}

	// Reader engages with the URL'd story.
	var keepID string
	for _, s := range first {
		if s.URL != nil && *s.URL == keepURL {
			keepID = s.ID
		}
	}
	if keepID == "" {
		t.Fatal("kept story not found after first score")
	}
	opened := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	if err := repo.SetBookmark(ctx, keepID, true); err != nil {
		t.Fatalf("bookmark: %v", err)
	}
	if err := repo.SetRating(ctx, keepID, 1); err != nil {
		t.Fatalf("rate: %v", err)
	}
	if err := repo.MarkOpened(ctx, keepID, opened); err != nil {
		t.Fatalf("open: %v", err)
	}

	// Re-fire: score the same newsletter again (the deployed re-fire path).
	if err := w.processOne(ctx, nl); err != nil {
		t.Fatalf("processOne #2: %v", err)
	}

	second, _ := repo.ListByNewsletter(ctx, nl.ID)
	if len(second) != 2 {
		t.Fatalf("after re-score: %d stories, want 2 (no duplicates)", len(second))
	}

	var keep *db.Story
	for i := range second {
		if second[i].URL != nil && *second[i].URL == keepURL {
			keep = &second[i]
		}
	}
	if keep == nil {
		t.Fatal("kept story missing after re-score")
	}
	if keep.ID == keepID {
		t.Error("expected a fresh story row (new id) after re-score, got the old id")
	}
	if !keep.Bookmarked {
		t.Error("bookmark not carried over")
	}
	if keep.UserRating == nil || *keep.UserRating != 1 {
		t.Errorf("rating not carried over: %v", keep.UserRating)
	}
	if keep.OpenedAt == nil || !keep.OpenedAt.Equal(opened) {
		t.Errorf("opened_at not carried over: got %v want %v", keep.OpenedAt, opened)
	}
}

func TestProcessOneTransientErrorRequeues(t *testing.T) {
	pool := setupDB(t)
	nl := seedPending(t, pool, "x", "body")

	scorer := &stubScorer{err: &ai.TransportError{StatusCode: 503, Err: errors.New("down")}}
	if err := quietWorker(pool, scorer).processOne(context.Background(), nl); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if got := statusOf(t, pool, nl.ID); got != db.StatusPending {
		t.Errorf("status = %q, want pending_scoring (requeued)", got)
	}
}

func TestProcessOneTerminalErrorFails(t *testing.T) {
	pool := setupDB(t)
	nl := seedPending(t, pool, "x", "body")

	scorer := &stubScorer{err: &ai.DecodeError{Err: errors.New("garbage")}}
	if err := quietWorker(pool, scorer).processOne(context.Background(), nl); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if got := statusOf(t, pool, nl.ID); got != db.StatusFailed {
		t.Errorf("status = %q, want failed", got)
	}
}

func TestProcessOneValidationErrorFails(t *testing.T) {
	pool := setupDB(t)
	nl := seedPending(t, pool, "x", "body")

	// A plain (non-transport) error stands in for a ParseItems validation failure.
	scorer := &stubScorer{err: errors.New("ai: response had items but none were usable")}
	if err := quietWorker(pool, scorer).processOne(context.Background(), nl); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if got := statusOf(t, pool, nl.ID); got != db.StatusFailed {
		t.Errorf("status = %q, want failed", got)
	}
}

func TestProcessOneEmptyItemsCompletesWithNoStories(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	nl := seedPending(t, pool, "x", "body")

	scorer := &stubScorer{items: nil}
	if err := quietWorker(pool, scorer).processOne(ctx, nl); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if got := statusOf(t, pool, nl.ID); got != db.StatusScored {
		t.Errorf("status = %q, want scored", got)
	}
	stories, _ := db.NewStoryRepo(pool).ListByNewsletter(ctx, nl.ID)
	if len(stories) != 0 {
		t.Errorf("got %d stories, want 0", len(stories))
	}
}

func TestProcessOneEmptyBodySkipsModel(t *testing.T) {
	pool := setupDB(t)
	nl := seedPending(t, pool, "", "")

	scorer := &stubScorer{items: []ai.ScoredItem{{Title: "should not be used"}}}
	if err := quietWorker(pool, scorer).processOne(context.Background(), nl); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if scorer.calls != 0 {
		t.Errorf("scorer called %d times, want 0 for empty body", scorer.calls)
	}
	if got := statusOf(t, pool, nl.ID); got != db.StatusScored {
		t.Errorf("status = %q, want scored", got)
	}
}

func TestClaimPendingIsAtomicAndRunOnceProcessesBatch(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	a := seedPending(t, pool, "a", "body a")
	b := seedPending(t, pool, "b", "body b")

	scorer := &stubScorer{items: []ai.ScoredItem{{Title: "t", Kind: ai.KindStory, RelevanceScore: 50}}}
	quietWorker(pool, scorer).runOnce(ctx)

	for _, nl := range []db.Newsletter{a, b} {
		if got := statusOf(t, pool, nl.ID); got != db.StatusScored {
			t.Errorf("newsletter %s status = %q, want scored", nl.ID, got)
		}
	}
	if scorer.calls != 2 {
		t.Errorf("scorer calls = %d, want 2", scorer.calls)
	}

	// Claiming again returns nothing — the batch is drained.
	claimed, err := db.NewNewsletterRepo(pool).ClaimPending(ctx, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("re-claim returned %d, want 0", len(claimed))
	}
}

func TestRequeueStaleResetsScoring(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	nl := seedPending(t, pool, "x", "body")
	// Simulate an interrupted run: stuck in scoring.
	if err := db.NewNewsletterRepo(pool).UpdateStatus(ctx, nl.ID, db.StatusScoring); err != nil {
		t.Fatalf("set scoring: %v", err)
	}

	n, err := quietWorker(pool, &stubScorer{}).requeueStale(ctx)
	if err != nil {
		t.Fatalf("requeueStale: %v", err)
	}
	if n != 1 {
		t.Errorf("requeued = %d, want 1", n)
	}
	if got := statusOf(t, pool, nl.ID); got != db.StatusPending {
		t.Errorf("status = %q, want pending_scoring", got)
	}
}
