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
	sources      *db.SourceRepo
	newsletters  *db.NewsletterRepo
	maxBodyChars int
}

// Option configures an Ingestor.
type Option func(*Ingestor)

// WithMaxBodyChars caps the cleaned body text derived for each newsletter. A
// value <= 0 disables truncation.
func WithMaxBodyChars(n int) Option {
	return func(in *Ingestor) { in.maxBodyChars = n }
}

// NewIngestor builds an Ingestor over database (a pool or a transaction).
func NewIngestor(database db.DBTX, opts ...Option) *Ingestor {
	in := &Ingestor{
		sources:      db.NewSourceRepo(database),
		newsletters:  db.NewNewsletterRepo(database),
		maxBodyChars: DefaultMaxBodyChars,
	}
	for _, opt := range opts {
		opt(in)
	}
	return in
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
		name = fallbackSourceName(identity)
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

	// Derive the cleaned, capped body the scorer will read, and dedupe on it.
	bodyText := in.prepareBody(msg)
	if bodyText != "" {
		nl.BodyText = &bodyText
	}
	if h := hashBody(bodyText); h != "" {
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

// fallbackSourceName derives a display name when the sender carries no display
// name. The local part is friendlier than the full address ("itbrew" rather
// than "itbrew@morningbrew.com"). Proper brand names come from the From display
// name (the live RFC822 feed carries it) or manual curation (source management
// is a follow-up); this is just a sane default for the bare-address case.
func fallbackSourceName(identity string) string {
	if i := strings.IndexByte(identity, '@'); i > 0 {
		return identity[:i]
	}
	return identity
}

// prepareBody produces the cleaned, capped body text persisted with the
// newsletter and later fed to the scorer. HTML is the richest source, so when
// present it is sanitized to text; otherwise a pre-extracted plaintext body is
// whitespace-normalized and truncated. raw_html is still stored verbatim.
func (in *Ingestor) prepareBody(msg InboundMessage) string {
	if msg.RawHTML != "" {
		return CleanText(msg.RawHTML, in.maxBodyChars)
	}
	return truncateChars(collapseSpaces(msg.BodyText), in.maxBodyChars)
}

// hashBody computes the fallback dedupe key from the prepared body text. It
// lowercases before hashing so case differences don't defeat the match. An
// empty body yields no hash.
func hashBody(body string) string {
	if body == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(body)))
	return hex.EncodeToString(sum[:])
}
