package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/db"
)

// Ingestor is the source-agnostic ingestion core. Every entrypoint — the raw
// RFC822 webhook, the JSON drop, a future live feed — normalizes to an
// InboundMessage and calls Ingest. It persists raw deliveries durably and never
// scores inline; scoring is the decoupled Phase 3 worker's job.
type Ingestor struct {
	sources     *db.SourceRepo
	newsletters *db.NewsletterRepo
}

// NewIngestor builds an Ingestor over database (a pool or a transaction).
func NewIngestor(database db.DBTX) *Ingestor {
	return &Ingestor{
		sources:     db.NewSourceRepo(database),
		newsletters: db.NewNewsletterRepo(database),
	}
}

// Result reports the outcome of an Ingest call.
type Result struct {
	Newsletter *db.Newsletter
	SourceID   string
	// Created is false when the message deduplicated against an existing row.
	Created bool
}

// Ingest normalizes and durably persists one inbound message. It resolves (or
// creates) the source from the sender, computes a content dedupe hash, and
// inserts a newsletter in processing_status=pending_scoring. Re-ingesting the
// same message — matched by Message-ID, else by content hash — is a no-op that
// returns the existing row with Created=false.
func (in *Ingestor) Ingest(ctx context.Context, msg InboundMessage) (*Result, error) {
	identity := normalizeAddr(msg.FromAddr)
	name := msg.FromName
	if name == "" {
		name = identity
	}
	src, err := in.sources.GetOrCreate(ctx, db.SourceKindNewsletter, identity, name)
	if err != nil {
		return nil, fmt.Errorf("resolve source: %w", err)
	}

	receivedAt := msg.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	nl := &db.Newsletter{SourceID: src.ID, ReceivedAt: &receivedAt}
	if msg.MessageID != "" {
		nl.MessageID = &msg.MessageID
	}
	if msg.Subject != "" {
		nl.Subject = &msg.Subject
	}
	if msg.RawHTML != "" {
		nl.RawHTML = &msg.RawHTML
	}
	if msg.BodyText != "" {
		nl.BodyText = &msg.BodyText
	}
	if h := dedupeHash(msg); h != "" {
		nl.DedupeHash = &h
	}

	inserted, created, err := in.newsletters.Insert(ctx, nl)
	if err != nil {
		return nil, fmt.Errorf("persist newsletter: %w", err)
	}
	return &Result{Newsletter: inserted, SourceID: src.ID, Created: created}, nil
}

// normalizeAddr lowercases and trims a sender address for use as a stable source
// identity. An empty address collapses to a shared "unknown" source rather than
// minting a row per blank sender.
func normalizeAddr(addr string) string {
	a := strings.ToLower(strings.TrimSpace(addr))
	if a == "" {
		return "unknown"
	}
	return a
}

// dedupeHash computes a stable content fingerprint, used as the fallback dedupe
// key when a sender omits or reuses its Message-ID. It hashes the normalized
// body — whitespace-collapsed and lowercased so trivial reformatting doesn't
// defeat it — preferring the cleaned text and falling back to raw HTML.
func dedupeHash(msg InboundMessage) string {
	body := msg.BodyText
	if body == "" {
		body = msg.RawHTML
	}
	body = normalizeBody(body)
	if body == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// normalizeBody collapses all runs of whitespace to single spaces and lowercases
// the result, so cosmetic differences don't produce distinct hashes.
func normalizeBody(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
