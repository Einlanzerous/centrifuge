package ingest

import (
	"context"
	"testing"

	"github.com/Einlanzerous/centrifuge/internal/db"
)

func TestIngestCreatesSourceAndNewsletter(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	ing := NewIngestor(pool)

	res, err := ing.Ingest(ctx, InboundMessage{
		FromAddr:  "News@Example.com",
		FromName:  "Example News",
		Subject:   "Issue 1",
		MessageID: "abc@example.com",
		RawHTML:   "<p>hello</p>",
		BodyText:  "hello",
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if !res.Created {
		t.Fatal("expected Created=true on first ingest")
	}
	if res.Newsletter.ProcessingStatus != db.StatusPending {
		t.Errorf("status = %q, want %q", res.Newsletter.ProcessingStatus, db.StatusPending)
	}

	// The source is resolved by the normalized (lowercased) sender address.
	src, err := db.NewSourceRepo(pool).GetByID(ctx, res.SourceID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if src.Identity != "news@example.com" {
		t.Errorf("identity = %q, want normalized lowercase", src.Identity)
	}
	if src.Name != "Example News" {
		t.Errorf("name = %q, want display name", src.Name)
	}
}

func TestIngestDedupeByMessageID(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	ing := NewIngestor(pool)
	msg := InboundMessage{FromAddr: "a@b.com", MessageID: "dup@b.com", BodyText: "one"}

	first, err := ing.Ingest(ctx, msg)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !first.Created {
		t.Fatal("first ingest should create")
	}

	// Same Message-ID, different body — still a duplicate (Message-ID wins).
	msg.BodyText = "two"
	second, err := ing.Ingest(ctx, msg)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Created {
		t.Fatal("re-ingesting the same Message-ID should dedupe")
	}
	if second.Newsletter.ID != first.Newsletter.ID {
		t.Errorf("ids differ: %s vs %s", first.Newsletter.ID, second.Newsletter.ID)
	}
}

func TestIngestDedupeByContentHash(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	ing := NewIngestor(pool)

	// No Message-ID: dedupe falls back to the content hash, which must survive
	// whitespace and case differences.
	a := InboundMessage{FromAddr: "a@b.com", BodyText: "Breaking   News\nToday"}
	b := InboundMessage{FromAddr: "a@b.com", BodyText: "breaking news today"}

	first, err := ing.Ingest(ctx, a)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !first.Created {
		t.Fatal("first ingest should create")
	}

	second, err := ing.Ingest(ctx, b)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Created {
		t.Fatal("matching content hash should dedupe")
	}
	if second.Newsletter.ID != first.Newsletter.ID {
		t.Errorf("ids differ: %s vs %s", first.Newsletter.ID, second.Newsletter.ID)
	}
}

func TestIngestDistinctMessages(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	ing := NewIngestor(pool)

	first, err := ing.Ingest(ctx, InboundMessage{FromAddr: "a@b.com", BodyText: "alpha"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := ing.Ingest(ctx, InboundMessage{FromAddr: "a@b.com", BodyText: "beta"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.Created {
		t.Fatal("distinct bodies should both insert")
	}
	if first.Newsletter.ID == second.Newsletter.ID {
		t.Fatal("distinct messages should get distinct ids")
	}
}
