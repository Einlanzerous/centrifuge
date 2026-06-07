package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestStoryInsertManyAndScore(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	sourceID, newsletterID := seedNewsletter(t, pool)
	repo := NewStoryRepo(pool)

	in := []Story{
		{Position: 0, Kind: KindStory, Section: ptr("One Big Headline"), Title: ptr("First"), URL: ptr("https://a")},
		{Position: 1, Kind: KindStory, Title: ptr("Second")},
		{Position: 2, Kind: KindAd, Title: ptr("Sponsored")}, // persisted-but-unscored
	}
	out, err := repo.InsertMany(ctx, newsletterID, sourceID, in)
	if err != nil {
		t.Fatalf("InsertMany: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 inserted, got %d", len(out))
	}
	for i, s := range out {
		if s.ID == "" {
			t.Fatalf("story %d missing id", i)
		}
		if s.SourceID != sourceID || s.NewsletterID != newsletterID {
			t.Fatalf("story %d has wrong fks: %+v", i, s)
		}
		if s.RelevanceScore != nil {
			t.Fatalf("story %d should be unscored at insert", i)
		}
	}

	// Empty kind defaults to story (driven by the column default).
	defaulted, err := repo.InsertMany(ctx, newsletterID, sourceID, []Story{{Position: 9}})
	if err != nil {
		t.Fatalf("InsertMany default kind: %v", err)
	}
	if defaulted[0].Kind != KindStory {
		t.Fatalf("expected default kind story, got %q", defaulted[0].Kind)
	}

	// Empty input is a no-op.
	none, err := repo.InsertMany(ctx, newsletterID, sourceID, nil)
	if err != nil || none != nil {
		t.Fatalf("empty InsertMany: got %v err %v", none, err)
	}

	// Score the first story in place, with labels round-tripping through jsonb.
	target := out[0].ID
	if err := repo.ScoreUpdate(ctx, target, Score{
		Summary:        "A concise summary.",
		RelevanceScore: 87,
		PrimaryTopic:   "AI engineering",
		Labels:         []string{"llm", "agents"},
		Model:          "gemma4:31b",
		PromptVersion:  "v1",
	}); err != nil {
		t.Fatalf("ScoreUpdate: %v", err)
	}

	got, err := repo.ListByNewsletter(ctx, newsletterID)
	if err != nil {
		t.Fatalf("ListByNewsletter: %v", err)
	}
	// 3 + 1 defaulted = 4, ordered by position (0,1,2,9).
	if len(got) != 4 {
		t.Fatalf("expected 4 stories, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Position > got[i].Position {
			t.Fatalf("not ordered by position: %+v", got)
		}
	}

	scored := got[0]
	if scored.RelevanceScore == nil || *scored.RelevanceScore != 87 {
		t.Fatalf("relevance not persisted: %+v", scored.RelevanceScore)
	}
	if scored.ScoredAt == nil {
		t.Fatal("scored_at should be set")
	}
	if len(scored.Labels) != 2 || scored.Labels[0] != "llm" || scored.Labels[1] != "agents" {
		t.Fatalf("labels did not round-trip: %+v", scored.Labels)
	}

	// Unknown id reports ErrNoRows.
	if err := repo.ScoreUpdate(ctx, "00000000-0000-0000-0000-000000000000", Score{}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestStoryEngagement(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	sourceID, newsletterID := seedNewsletter(t, pool)
	repo := NewStoryRepo(pool)

	out, err := repo.InsertMany(ctx, newsletterID, sourceID, []Story{{Position: 0, Kind: KindStory, Title: ptr("X")}})
	if err != nil {
		t.Fatalf("InsertMany: %v", err)
	}
	id := out[0].ID

	when := time.Now().UTC().Truncate(time.Millisecond)
	if err := repo.SetBookmark(ctx, id, true); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}
	if err := repo.SetRating(ctx, id, 1); err != nil {
		t.Fatalf("SetRating: %v", err)
	}
	if err := repo.MarkOpened(ctx, id, when); err != nil {
		t.Fatalf("MarkOpened: %v", err)
	}
	// Reader "mark as ad" override.
	if err := repo.SetKind(ctx, id, KindAd); err != nil {
		t.Fatalf("SetKind: %v", err)
	}

	got, err := repo.ListByNewsletter(ctx, newsletterID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	s := got[0]
	if !s.Bookmarked {
		t.Fatal("bookmark not set")
	}
	if s.UserRating == nil || *s.UserRating != 1 {
		t.Fatalf("rating not set: %+v", s.UserRating)
	}
	if s.OpenedAt == nil || !s.OpenedAt.Equal(when) {
		t.Fatalf("opened_at mismatch: got %v want %v", s.OpenedAt, when)
	}
	if s.Kind != KindAd {
		t.Fatalf("kind override not applied: %q", s.Kind)
	}

	// Engagement on an unknown id is reported.
	if err := repo.SetBookmark(ctx, "00000000-0000-0000-0000-000000000000", true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestSourceAggregate(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	srcRepo := NewSourceRepo(pool)
	nlRepo := NewNewsletterRepo(pool)
	stRepo := NewStoryRepo(pool)

	// Source A: two scored stories (avg 80), one bookmark.
	a, _ := srcRepo.GetOrCreate(ctx, SourceKindNewsletter, "a@x", "A")
	anl, _, _ := nlRepo.Insert(ctx, &Newsletter{SourceID: a.ID, MessageID: ptr("<a1>")})
	as, err := stRepo.InsertMany(ctx, anl.ID, a.ID, []Story{{Position: 0, Kind: KindStory}, {Position: 1, Kind: KindStory}})
	if err != nil {
		t.Fatalf("seed A stories: %v", err)
	}
	mustScore(t, stRepo, as[0].ID, 70)
	mustScore(t, stRepo, as[1].ID, 90)
	if err := stRepo.SetBookmark(ctx, as[0].ID, true); err != nil {
		t.Fatalf("bookmark: %v", err)
	}

	// Source B: one scored story (avg 50).
	b, _ := srcRepo.GetOrCreate(ctx, SourceKindNewsletter, "b@x", "B")
	bnl, _, _ := nlRepo.Insert(ctx, &Newsletter{SourceID: b.ID, MessageID: ptr("<b1>")})
	bs, _ := stRepo.InsertMany(ctx, bnl.ID, b.ID, []Story{{Position: 0, Kind: KindStory}})
	mustScore(t, stRepo, bs[0].ID, 50)

	// Source C: no stories at all.
	if _, err := srcRepo.GetOrCreate(ctx, SourceKindNewsletter, "c@x", "C"); err != nil {
		t.Fatalf("seed C: %v", err)
	}

	stats, err := stRepo.SourceAggregate(ctx)
	if err != nil {
		t.Fatalf("SourceAggregate: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(stats))
	}

	byName := map[string]SourceStats{}
	for _, s := range stats {
		byName[s.SourceName] = s
	}

	if got := byName["A"]; got.AvgRelevance == nil || *got.AvgRelevance != 80 {
		t.Fatalf("A avg: %+v", got.AvgRelevance)
	} else if got.StoryCount != 2 || got.ScoredCount != 2 || got.BookmarkCount != 1 {
		t.Fatalf("A counts: %+v", got)
	}
	if got := byName["C"]; got.AvgRelevance != nil || got.StoryCount != 0 {
		t.Fatalf("C should be empty: %+v", got)
	}

	// Best average first; C (null) sorts last.
	if stats[0].SourceName != "A" || stats[len(stats)-1].SourceName != "C" {
		t.Fatalf("unexpected ordering: %v", names(stats))
	}
}

func mustScore(t *testing.T, repo *StoryRepo, id string, score int) {
	t.Helper()
	if err := repo.ScoreUpdate(context.Background(), id, Score{
		RelevanceScore: score,
		Model:          "gemma4:31b",
		PromptVersion:  "v1",
	}); err != nil {
		t.Fatalf("score %s: %v", id, err)
	}
}

func names(stats []SourceStats) []string {
	out := make([]string, len(stats))
	for i, s := range stats {
		out[i] = s.SourceName
	}
	return out
}
