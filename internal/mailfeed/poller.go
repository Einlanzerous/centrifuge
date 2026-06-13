// Package mailfeed runs the live email auto-feed (CTFG-24): a background poller
// that dials Gmail outbound on an interval, pulls each new message as raw
// RFC822, and feeds it through the same ingestion core as the /ingest webhook.
//
// Polling (rather than a push webhook) is deliberate: centrifuge runs on the
// homelab behind NAT with no public ingress, so an outbound poll needs nothing
// exposed to the internet. The poller mirrors the scoring worker's shape — a
// New(...) constructor with functional options and a blocking Run(ctx) started
// in a goroutine — so its lifecycle and failure handling read the same.
package mailfeed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/ingest"
)

// Defaults applied when an option is not supplied.
const (
	DefaultInterval = 120 * time.Second
	DefaultBatch    = 25
)

// MailClient is the Gmail seam the poller depends on. *gmailClient satisfies it;
// tests supply a stub so they need no network.
type MailClient interface {
	// ListUnprocessed returns up to max message IDs that still need ingesting
	// (those not yet carrying the processed label).
	ListUnprocessed(ctx context.Context, max int) ([]string, error)
	// GetRaw fetches one message as raw RFC822 bytes.
	GetRaw(ctx context.Context, id string) ([]byte, error)
	// MarkProcessed labels a message so subsequent polls skip it.
	MarkProcessed(ctx context.Context, id string) error
}

// Ingester is the ingestion seam. *ingest.Ingestor satisfies it; tests stub it.
type Ingester interface {
	Ingest(ctx context.Context, msg ingest.InboundMessage) (*ingest.Result, error)
}

// Poller pulls new Gmail messages and feeds them through ingestion.
type Poller struct {
	client   MailClient
	ingester Ingester
	interval time.Duration
	batch    int
	logger   *slog.Logger
}

// Option configures a Poller.
type Option func(*Poller)

// WithInterval sets the poll interval between ticks.
func WithInterval(d time.Duration) Option {
	return func(p *Poller) {
		if d > 0 {
			p.interval = d
		}
	}
}

// WithBatch sets how many messages are processed per tick.
func WithBatch(n int) Option {
	return func(p *Poller) {
		if n > 0 {
			p.batch = n
		}
	}
}

// WithLogger sets the poller's logger.
func WithLogger(l *slog.Logger) Option {
	return func(p *Poller) {
		if l != nil {
			p.logger = l
		}
	}
}

// New builds a Poller over the ingestion core and a Gmail client.
func New(ingester Ingester, client MailClient, opts ...Option) *Poller {
	p := &Poller{
		client:   client,
		ingester: ingester,
		interval: DefaultInterval,
		batch:    DefaultBatch,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Run polls until ctx is cancelled. It processes a batch immediately and once
// per interval thereafter. Run blocks; call it in a goroutine.
func (p *Poller) Run(ctx context.Context) {
	p.logger.Info("mail feed poller started", "interval", p.interval.String(), "batch", p.batch)
	t := time.NewTicker(p.interval)
	defer t.Stop()

	p.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			p.logger.Info("mail feed poller stopped")
			return
		case <-t.C:
			p.runOnce(ctx)
		}
	}
}

// runOnce lists and processes one batch of unprocessed messages, logging
// outcomes. It never returns an error: a transient failure should not kill the
// loop — the next tick retries anything left unlabeled.
func (p *Poller) runOnce(ctx context.Context) {
	ids, err := p.client.ListUnprocessed(ctx, p.batch)
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Error("list unprocessed mail", "error", err)
		}
		return
	}
	if len(ids) == 0 {
		return
	}
	p.logger.Debug("found unprocessed messages", "count", len(ids))

	var ingested, duplicate, failed int
	for _, id := range ids {
		if ctx.Err() != nil {
			return // shutting down; unlabeled messages recover on next start.
		}
		dup, err := p.processOne(ctx, id)
		switch {
		case err != nil:
			failed++
			p.logger.Error("process message", "message", id, "error", err)
		case dup:
			duplicate++
		default:
			ingested++
		}
	}
	p.logger.Info("mail feed poll complete", "ingested", ingested, "duplicate", duplicate, "failed", failed)
}

// processOne fetches, parses, ingests, and labels one message, returning whether
// the message deduplicated against an existing newsletter.
//
// Labeling is what advances the cursor: an ingested message is labeled so the
// next poll's query skips it. The label is best-effort — ingestion already
// dedupes by Message-ID, so a message re-fetched after a failed label is a
// harmless no-op (Created=false). The one case that does get labeled on failure
// is an unparseable message: it will never parse, so labeling it stops a poison
// message from occupying a batch slot every tick.
func (p *Poller) processOne(ctx context.Context, id string) (duplicate bool, err error) {
	raw, err := p.client.GetRaw(ctx, id)
	if err != nil {
		return false, fmt.Errorf("fetch: %w", err)
	}

	msg, err := ingest.ParseRFC822(raw)
	if err != nil {
		if lerr := p.client.MarkProcessed(ctx, id); lerr != nil {
			p.logger.Warn("label unparseable message", "message", id, "error", lerr)
		}
		return false, fmt.Errorf("parse: %w", err)
	}

	res, err := p.ingester.Ingest(ctx, *msg)
	if err != nil {
		// Leave it unlabeled so a later poll retries once ingestion recovers.
		return false, fmt.Errorf("ingest: %w", err)
	}

	if err := p.client.MarkProcessed(ctx, id); err != nil {
		p.logger.Warn("label ingested message", "message", id, "error", err)
	}
	return !res.Created, nil
}
