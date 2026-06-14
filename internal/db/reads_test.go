package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedStory inserts a single scored story under a named source, returning its
// id. received controls the parent newsletter's received_at (the timeline ts).
func seedStory(t *testing.T, pool *pgxpool.Pool, sourceName, identity, title, topic string, score int, received time.Time) string {
	t.Helper()
	ctx := context.Background()
	src, err := NewSourceRepo(pool).GetOrCreate(ctx, SourceKindNewsletter, identity, sourceName)
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	rec := received
	nl, _, err := NewNewsletterRepo(pool).Insert(ctx, &Newsletter{
		SourceID:   src.ID,
		RawHTML:    ptr("<p>" + title + " body</p>"),
		ReceivedAt: &rec,
	})
	if err != nil {
		t.Fatalf("seed newsletter: %v", err)
	}
	st := NewStoryRepo(pool)
	out, err := st.InsertMany(ctx, nl.ID, src.ID, []Story{{Position: 0, Kind: KindStory, Title: ptr(title)}})
	if err != nil {
		t.Fatalf("seed story: %v", err)
	}
	if err := st.ScoreUpdate(ctx, out[0].ID, Score{
		Summary: title + " summary", RelevanceScore: score, PrimaryTopic: topic,
		Labels: []string{"x"}, Model: "m", PromptVersion: "v1",
	}); err != nil {
		t.Fatalf("score story: %v", err)
	}
	return out[0].ID
}

func TestArchiveFilters(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	repo := NewStoryRepo(pool)

	now := time.Now().UTC()
	seedStory(t, pool, "Brew", "brew@x", "Transit funding bill", "transit", 80, now.Add(-1*time.Hour))
	seedStory(t, pool, "Brew", "brew@x", "New LLM agents", "AI engineering", 90, now.Add(-2*time.Hour))
	seedStory(t, pool, "Observer", "obs@x", "Nuclear loan terms", "nuclear", 70, now.Add(-48*time.Hour))

	// No filter: all three, newest first.
	all, err := repo.Archive(ctx, ArchiveFilter{})
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d, want 3", len(all))
	}
	if all[0].Title == nil || *all[0].Title != "Transit funding bill" {
		t.Fatalf("not newest-first: %v", all[0].Title)
	}
	if all[0].SourceName != "Brew" {
		t.Fatalf("source name not joined: %q", all[0].SourceName)
	}

	// Topic filter.
	byTopic, _ := repo.Archive(ctx, ArchiveFilter{Topic: "nuclear"})
	if len(byTopic) != 1 || *byTopic[0].Title != "Nuclear loan terms" {
		t.Fatalf("topic filter: %+v", titles(byTopic))
	}

	// Search filter (case-insensitive).
	bySearch, _ := repo.Archive(ctx, ArchiveFilter{Query: "llm"})
	if len(bySearch) != 1 || *bySearch[0].Title != "New LLM agents" {
		t.Fatalf("search filter: %+v", titles(bySearch))
	}

	// Date-range filter excludes the 48h-old story.
	cutoff := now.Add(-24 * time.Hour)
	byDate, _ := repo.Archive(ctx, ArchiveFilter{From: &cutoff})
	if len(byDate) != 2 {
		t.Fatalf("date filter: %+v", titles(byDate))
	}

	// Source filter.
	bySrc, _ := repo.Archive(ctx, ArchiveFilter{SourceID: all[0].SourceID})
	if len(bySrc) != 2 {
		t.Fatalf("source filter: %+v", titles(bySrc))
	}
}

func TestListScoredSinceAndBrief(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	repo := NewStoryRepo(pool)

	now := time.Now().UTC()
	seedStory(t, pool, "Brew", "brew@x", "A", "AI engineering", 90, now)
	seedStory(t, pool, "Brew", "brew@x", "B", "transit", 60, now)

	// nil since: all scored stories, best first.
	since, err := repo.ListScoredSince(ctx, nil, 10)
	if err != nil {
		t.Fatalf("ListScoredSince: %v", err)
	}
	if len(since) != 2 || *since[0].Title != "A" {
		t.Fatalf("since nil: %+v", titles(since))
	}

	// A future cutoff yields nothing (everything was scored before it).
	future := now.Add(time.Hour)
	none, _ := repo.ListScoredSince(ctx, &future, 10)
	if len(none) != 0 {
		t.Fatalf("future since should be empty, got %d", len(none))
	}

	// Brief returns unopened stories; after opening one it drops out.
	brief, _ := repo.ListBrief(ctx, 10)
	if len(brief) != 2 {
		t.Fatalf("brief: %d", len(brief))
	}
	if err := repo.MarkOpened(ctx, brief[0].ID, now); err != nil {
		t.Fatalf("MarkOpened: %v", err)
	}
	brief2, _ := repo.ListBrief(ctx, 10)
	if len(brief2) != 1 {
		t.Fatalf("brief after open: %d", len(brief2))
	}
}

func TestGetEnrichedAndTopicRegistry(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	repo := NewStoryRepo(pool)

	now := time.Now().UTC()
	id := seedStory(t, pool, "Brew", "brew@x", "Headline", "AI engineering", 88, now)
	seedStory(t, pool, "Brew", "brew@x", "Other", "AI engineering", 50, now)
	seedStory(t, pool, "Obs", "obs@x", "Third", "nuclear", 70, now)

	v, err := repo.GetEnriched(ctx, id)
	if err != nil {
		t.Fatalf("GetEnriched: %v", err)
	}
	if v.SourceName != "Brew" || v.Body == nil || *v.Body == "" {
		t.Fatalf("enriched missing source/body: %+v", v)
	}
	if v.RelevanceScore == nil || *v.RelevanceScore != 88 {
		t.Fatalf("enriched score: %+v", v.RelevanceScore)
	}

	reg, err := repo.TopicRegistry(ctx)
	if err != nil {
		t.Fatalf("TopicRegistry: %v", err)
	}
	if len(reg) != 2 {
		t.Fatalf("topics: %+v", reg)
	}
	// AI engineering (2) before nuclear (1).
	if reg[0].Topic != "AI engineering" || reg[0].Count != 2 {
		t.Fatalf("topic order/count: %+v", reg)
	}
}

// seedDigest inserts one newsletter with the given raw HTML and items (each
// carrying its own kind/title/snippet), returning the inserted stories in
// position order. Unlike seedStory it leaves items unscored — these tests only
// exercise GetEnriched's structural fields.
func seedDigest(t *testing.T, pool *pgxpool.Pool, sourceName, identity, rawHTML string, items []Story) []Story {
	t.Helper()
	ctx := context.Background()
	src, err := NewSourceRepo(pool).GetOrCreate(ctx, SourceKindNewsletter, identity, sourceName)
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	now := time.Now().UTC()
	nl, _, err := NewNewsletterRepo(pool).Insert(ctx, &Newsletter{
		SourceID:   src.ID,
		RawHTML:    ptr(rawHTML),
		ReceivedAt: &now,
	})
	if err != nil {
		t.Fatalf("seed newsletter: %v", err)
	}
	out, err := NewStoryRepo(pool).InsertMany(ctx, nl.ID, src.ID, items)
	if err != nil {
		t.Fatalf("seed digest: %v", err)
	}
	return out
}

// TestGetEnrichedSegmentedSpansAllKinds guards CTFG-36 defect A: when
// segmentation demotes a digest's genuine sibling stories to blurb/ad, a lone
// surviving story remains. The "segmented" flag must still report true (so the
// Reader slices that story's own segment) rather than treating it as a single
// essay and dumping the entire newsletter.
func TestGetEnrichedSegmentedSpansAllKinds(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	repo := NewStoryRepo(pool)

	const lead = "Researchers at the institute discovered that migrating birds use quantum effects to navigate across whole continents during long seasonal journeys"
	const adSnip = "This issue is brought to you by AcmeVPN the fast secure tunnel trusted by millions of remote workers everywhere today"
	rawHTML := "<p>" + lead + ".</p><p>" + adSnip + ".</p>"

	items := seedDigest(t, pool, "Daily", "daily@x", rawHTML, []Story{
		{Position: 0, Kind: KindStory, Title: ptr("Quantum Birds"), Snippet: ptr(lead)},
		{Position: 1, Kind: KindAd, Title: ptr("AcmeVPN"), Snippet: ptr(adSnip)},
	})

	v, err := repo.GetEnriched(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("GetEnriched: %v", err)
	}
	if !v.Segmented {
		t.Fatal("lone story with an ad sibling must be segmented (a digest), got Segmented=false")
	}
	if v.SegmentText == nil {
		t.Fatal("segmented story should have sliced segment text, got nil")
	}
	if !strings.Contains(*v.SegmentText, "migrating birds") {
		t.Fatalf("segment missing the lead story: %q", *v.SegmentText)
	}
	if strings.Contains(*v.SegmentText, "AcmeVPN") {
		t.Fatalf("segment leaked the sibling ad (whole-newsletter dump): %q", *v.SegmentText)
	}

	// A true single essay (no siblings) stays non-segmented: the whole body IS
	// the story, rendered inline by the Reader.
	solo := seedDigest(t, pool, "Essayist", "essay@x", "<p>"+lead+".</p>", []Story{
		{Position: 0, Kind: KindStory, Title: ptr("Solo"), Snippet: ptr(lead)},
	})
	sv, err := repo.GetEnriched(ctx, solo[0].ID)
	if err != nil {
		t.Fatalf("GetEnriched solo: %v", err)
	}
	if sv.Segmented {
		t.Fatal("a single-item newsletter must not be segmented")
	}
}

func titles(vs []StoryView) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		if v.Title != nil {
			out[i] = *v.Title
		}
	}
	return out
}
