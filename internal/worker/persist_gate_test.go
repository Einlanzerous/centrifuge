package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func gateFindingsOf(t *testing.T, pool *pgxpool.Pool, id string) []string {
	t.Helper()
	var got []string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(gate_findings, '{}') FROM newsletters WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("read gate_findings: %v", err)
	}
	return got
}

func TestPersistRecordsAndClearsGateFindings(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	// A big non-essay body whose scoring yields a single summaryless story:
	// trips silent_collapse and empty_summary.
	nl := seedPending(t, pool, "Digest", strings.Repeat("word ", 2000))

	scorer := &stubScorer{items: []ai.ScoredItem{
		{Title: "Dump", Snippet: "word word", Kind: ai.KindStory, RelevanceScore: 5},
	}}
	if err := quietWorker(pool, scorer).processOne(ctx, nl); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if got := statusOf(t, pool, nl.ID); got != db.StatusScored {
		t.Errorf("status = %q, want scored (gate flags, it does not fail)", got)
	}
	findings := gateFindingsOf(t, pool, nl.ID)
	if len(findings) != 2 {
		t.Fatalf("gate_findings = %v, want silent_collapse + empty_summary", findings)
	}

	// A clean re-score must clear the flag.
	scorer.items = []ai.ScoredItem{
		{Title: "A", Snippet: "word word", Kind: ai.KindStory, Summary: "Fine.", RelevanceScore: 50},
		{Title: "B", Snippet: "word word word", Kind: ai.KindStory, Summary: "Also fine.", RelevanceScore: 60},
	}
	if err := db.NewNewsletterRepo(pool).Requeue(ctx, nl.ID); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	claimed, err := db.NewNewsletterRepo(pool).ClaimPending(ctx, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v (%d)", err, len(claimed))
	}
	if err := quietWorker(pool, scorer).processOne(ctx, claimed[0]); err != nil {
		t.Fatalf("re-score: %v", err)
	}
	if findings := gateFindingsOf(t, pool, nl.ID); len(findings) != 0 {
		t.Errorf("gate_findings after clean re-score = %v, want cleared", findings)
	}
}
