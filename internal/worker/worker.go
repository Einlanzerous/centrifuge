// Package worker runs the decoupled scoring loop: it polls newsletters that
// ingestion left in pending_scoring, segments and scores each via the model,
// persists the resulting stories, and advances the newsletter to scored (or
// failed). Keeping scoring out of the /ingest path is what makes ingestion fast
// and resilient — a slow or down model never blocks or drops an email.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Einlanzerous/centrifuge/internal/ai"
	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Defaults applied when an option is not supplied.
const (
	DefaultInterval  = 30 * time.Second
	DefaultBatchSize = 5
)

// Scorer is the model seam the worker depends on. *ai.Scorer satisfies it; tests
// supply a stub so they need no live model.
type Scorer interface {
	Score(ctx context.Context, in ai.ScoreInput) ([]ai.ScoredItem, error)
	Model() string
}

// Worker polls and scores pending newsletters.
type Worker struct {
	pool      *pgxpool.Pool
	scorer    Scorer
	interval  time.Duration
	batchSize int
	logger    *slog.Logger
}

// Option configures a Worker.
type Option func(*Worker)

// WithInterval sets the poll interval between batches.
func WithInterval(d time.Duration) Option {
	return func(w *Worker) {
		if d > 0 {
			w.interval = d
		}
	}
}

// WithBatchSize sets how many newsletters are claimed per poll.
func WithBatchSize(n int) Option {
	return func(w *Worker) {
		if n > 0 {
			w.batchSize = n
		}
	}
}

// WithLogger sets the worker's logger.
func WithLogger(l *slog.Logger) Option {
	return func(w *Worker) {
		if l != nil {
			w.logger = l
		}
	}
}

// New builds a Worker over pool and scorer.
func New(pool *pgxpool.Pool, scorer Scorer, opts ...Option) *Worker {
	w := &Worker{
		pool:      pool,
		scorer:    scorer,
		interval:  DefaultInterval,
		batchSize: DefaultBatchSize,
		logger:    slog.Default(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Run polls until ctx is cancelled. It first requeues any newsletter left in
// scoring by a previous run (an interrupted shutdown), then processes a batch
// immediately and once per interval thereafter. Run blocks; call it in a
// goroutine.
func (w *Worker) Run(ctx context.Context) {
	if n, err := w.requeueStale(ctx); err != nil {
		w.logger.Error("requeue stale scoring newsletters", "error", err)
	} else if n > 0 {
		w.logger.Info("requeued interrupted newsletters", "count", n)
	}

	w.logger.Info("scoring worker started", "interval", w.interval.String(), "batch", w.batchSize, "model", w.scorer.Model())
	t := time.NewTicker(w.interval)
	defer t.Stop()

	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("scoring worker stopped")
			return
		case <-t.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce claims and processes one batch, logging outcomes. It never returns an
// error: a transient failure should not kill the loop.
func (w *Worker) runOnce(ctx context.Context) {
	claimed, err := db.NewNewsletterRepo(w.pool).ClaimPending(ctx, w.batchSize)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("claim pending newsletters", "error", err)
		}
		return
	}
	if len(claimed) == 0 {
		return
	}
	w.logger.Debug("claimed newsletters for scoring", "count", len(claimed))

	var scored, failed int
	for i := range claimed {
		if ctx.Err() != nil {
			return // shutting down; leftover claims recover on next start.
		}
		if err := w.processOne(ctx, claimed[i]); err != nil {
			failed++
			w.logger.Error("process newsletter", "newsletter", claimed[i].ID, "error", err)
			continue
		}
		scored++
	}
	w.logger.Info("scoring batch complete", "scored", scored, "failed", failed)
}

// processOne segments, scores, and persists a single newsletter, transitioning
// its status. A nil return means the newsletter reached a terminal state
// (scored/failed/requeued) cleanly; a non-nil error means even the status
// update failed and the row is left as-is for recovery.
func (w *Worker) processOne(ctx context.Context, nl db.Newsletter) error {
	in := ai.ScoreInput{
		Subject: deref(nl.Subject),
		Body:    deref(nl.BodyText),
	}
	// Nothing to feed the model — mark scored with zero stories rather than
	// burning a generate call on an empty body.
	if strings.TrimSpace(in.Subject) == "" && strings.TrimSpace(in.Body) == "" {
		return db.NewNewsletterRepo(w.pool).UpdateStatus(ctx, nl.ID, db.StatusScored)
	}

	items, err := w.scorer.Score(ctx, in)
	if err != nil {
		return w.handleScoreError(ctx, nl, err)
	}
	return w.persist(ctx, nl, items)
}

// handleScoreError requeues on transient transport failures (the model may
// recover) and marks failed on terminal ones (a decode/validation error won't
// improve on retry), so a newsletter is never silently lost.
func (w *Worker) handleScoreError(ctx context.Context, nl db.Newsletter, scoreErr error) error {
	repo := db.NewNewsletterRepo(w.pool)

	var te *ai.TransportError
	if errors.As(scoreErr, &te) {
		w.logger.Warn("scoring transient failure; requeuing", "newsletter", nl.ID, "error", scoreErr)
		return repo.UpdateStatus(ctx, nl.ID, db.StatusPending)
	}

	w.logger.Error("scoring terminal failure; marking failed", "newsletter", nl.ID, "error", scoreErr)
	return repo.UpdateStatus(ctx, nl.ID, db.StatusFailed)
}

// persist writes the segmented stories and their scores, then flips the
// newsletter to scored — all in one transaction so a crash never leaves partial
// stories behind a still-scoring newsletter. Only story-kind items get their
// scoring fields written; ads/blurbs/promos are persisted unscored.
func (w *Worker) persist(ctx context.Context, nl db.Newsletter, items []ai.ScoredItem) error {
	if len(items) == 0 {
		// The model found nothing worth segmenting; still a clean completion.
		return db.NewNewsletterRepo(w.pool).UpdateStatus(ctx, nl.ID, db.StatusScored)
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	stories := db.NewStoryRepo(tx)
	newsletters := db.NewNewsletterRepo(tx)

	rows := make([]db.Story, len(items))
	for i, it := range items {
		rows[i] = db.Story{
			Position: i,
			Kind:     it.Kind,
			Section:  ptrOrNil(it.Section),
			Title:    ptrOrNil(it.Title),
			URL:      ptrOrNil(it.URL),
			Snippet:  ptrOrNil(it.Snippet),
		}
	}

	inserted, err := stories.InsertMany(ctx, nl.ID, nl.SourceID, rows)
	if err != nil {
		return err
	}

	model := w.scorer.Model()
	for i := range inserted {
		if inserted[i].Kind != db.KindStory {
			continue
		}
		it := items[i]
		if err := stories.ScoreUpdate(ctx, inserted[i].ID, db.Score{
			Summary:        it.Summary,
			RelevanceScore: it.RelevanceScore,
			PrimaryTopic:   it.PrimaryTopic,
			Labels:         it.Labels,
			Model:          model,
			PromptVersion:  ai.PromptVersion,
		}); err != nil {
			return err
		}
	}

	if err := newsletters.UpdateStatus(ctx, nl.ID, db.StatusScored); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// requeueStale flips any newsletter stuck in scoring (from an interrupted run)
// back to pending_scoring so it is retried. Safe for a single worker; under
// multiple workers a row genuinely in flight elsewhere would also be requeued,
// but the claim's idempotent re-scoring tolerates that.
func (w *Worker) requeueStale(ctx context.Context) (int64, error) {
	ct, err := w.pool.Exec(ctx,
		`UPDATE newsletters SET processing_status = $2 WHERE processing_status = $1`,
		db.StatusScoring, db.StatusPending)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
