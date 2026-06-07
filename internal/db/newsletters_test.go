package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestNewsletterInsertDedup(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	src, err := NewSourceRepo(pool).GetOrCreate(ctx, SourceKindNewsletter, "a@example.com", "A")
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	repo := NewNewsletterRepo(pool)

	// First insert with a message_id.
	n1, inserted, err := repo.Insert(ctx, &Newsletter{SourceID: src.ID, MessageID: ptr("<m1@x>")})
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v", inserted, err)
	}
	if n1.ProcessingStatus != StatusPending {
		t.Fatalf("expected default status %q, got %q", StatusPending, n1.ProcessingStatus)
	}

	// Same message_id dedups to the existing row.
	n2, inserted, err := repo.Insert(ctx, &Newsletter{SourceID: src.ID, MessageID: ptr("<m1@x>")})
	if err != nil {
		t.Fatalf("dup insert: %v", err)
	}
	if inserted {
		t.Fatal("expected dedup (inserted=false) on duplicate message_id")
	}
	if n2.ID != n1.ID {
		t.Fatalf("dedup returned different row: %s vs %s", n2.ID, n1.ID)
	}

	// Soft dedup by content hash when there's no message_id.
	h1, inserted, err := repo.Insert(ctx, &Newsletter{SourceID: src.ID, DedupeHash: ptr("hash-abc")})
	if err != nil || !inserted {
		t.Fatalf("hash insert: inserted=%v err=%v", inserted, err)
	}
	h2, inserted, err := repo.Insert(ctx, &Newsletter{SourceID: src.ID, DedupeHash: ptr("hash-abc")})
	if err != nil {
		t.Fatalf("hash dup insert: %v", err)
	}
	if inserted || h2.ID != h1.ID {
		t.Fatalf("expected soft dedup by hash: inserted=%v id=%s vs %s", inserted, h2.ID, h1.ID)
	}

	// No identity at all always inserts a fresh row.
	b1, _, err := repo.Insert(ctx, &Newsletter{SourceID: src.ID})
	if err != nil {
		t.Fatalf("bare insert 1: %v", err)
	}
	b2, inserted, err := repo.Insert(ctx, &Newsletter{SourceID: src.ID})
	if err != nil || !inserted {
		t.Fatalf("bare insert 2: inserted=%v err=%v", inserted, err)
	}
	if b1.ID == b2.ID {
		t.Fatal("bare inserts should be distinct rows")
	}
}

func TestNewsletterFetchAndTransition(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	src, err := NewSourceRepo(pool).GetOrCreate(ctx, SourceKindNewsletter, "b@example.com", "B")
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	repo := NewNewsletterRepo(pool)

	for _, mid := range []string{"<p1@x>", "<p2@x>", "<p3@x>"} {
		if _, _, err := repo.Insert(ctx, &Newsletter{SourceID: src.ID, MessageID: ptr(mid)}); err != nil {
			t.Fatalf("insert %s: %v", mid, err)
		}
	}

	pending, err := repo.FetchByStatus(ctx, StatusPending, 10)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending, got %d", len(pending))
	}

	// limit is honored.
	limited, err := repo.FetchByStatus(ctx, StatusPending, 2)
	if err != nil {
		t.Fatalf("fetch limited: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 with limit, got %d", len(limited))
	}

	// Transition one to scored; it leaves the pending set.
	if err := repo.UpdateStatus(ctx, pending[0].ID, StatusScored); err != nil {
		t.Fatalf("transition: %v", err)
	}
	after, err := repo.FetchByStatus(ctx, StatusPending, 10)
	if err != nil {
		t.Fatalf("fetch after: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("expected 2 pending after transition, got %d", len(after))
	}

	// Unknown id is reported, not silently ignored.
	if err := repo.UpdateStatus(ctx, "00000000-0000-0000-0000-000000000000", StatusFailed); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows for unknown id, got %v", err)
	}
}
