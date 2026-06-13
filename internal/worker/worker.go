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
	// DefaultMaxScoringAttempts bounds how many times a newsletter whose model
	// output came back truncated is re-scored before the worker gives up and
	// keeps whatever was salvaged (or marks it failed). Transport failures are
	// not bounded by this — the model recovering should always be retried.
	DefaultMaxScoringAttempts = 3
)

// Scorer is the model seam the worker depends on. *ai.Scorer satisfies it; tests
// supply a stub so they need no live model.
type Scorer interface {
	Score(ctx context.Context, in ai.ScoreInput) ([]ai.ScoredItem, error)
	Model() string
	// Deterministic reports whether scoring samples greedily (temperature 0), so
	// retrying a truncated response would reproduce the identical output. When
	// true the worker salvages immediately instead of burning retries (CTFG-45).
	Deterministic() bool
}

// Worker polls and scores pending newsletters.
type Worker struct {
	pool        *pgxpool.Pool
	scorer      Scorer
	interval    time.Duration
	batchSize   int
	maxAttempts int
	logger      *slog.Logger
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

// WithMaxScoringAttempts bounds re-scoring of a newsletter whose model output
// keeps coming back truncated. Values < 1 are ignored.
func WithMaxScoringAttempts(n int) Option {
	return func(w *Worker) {
		if n >= 1 {
			w.maxAttempts = n
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
		pool:        pool,
		scorer:      scorer,
		interval:    DefaultInterval,
		batchSize:   DefaultBatchSize,
		maxAttempts: DefaultMaxScoringAttempts,
		logger:      slog.Default(),
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
		return w.handleScoreError(ctx, nl, items, err)
	}
	return w.persist(ctx, nl, items)
}

// handleScoreError decides what to do when scoring returns an error, so a
// newsletter is never silently lost:
//
//   - Transport failure (model down/slow): requeue unconditionally — the model
//     will recover, and these don't count against the truncation budget.
//   - Truncated output (CTFG-33): retry within the attempt budget; once exhausted,
//     persist whatever items were salvaged, or mark failed if none survived.
//   - Anything else (structural/validation error): terminal, mark failed.
//
// items carries any partial results the scorer salvaged from a truncated
// response.
func (w *Worker) handleScoreError(ctx context.Context, nl db.Newsletter, items []ai.ScoredItem, scoreErr error) error {
	repo := db.NewNewsletterRepo(w.pool)

	var te *ai.TransportError
	if errors.As(scoreErr, &te) {
		w.logger.Warn("scoring transient transport failure; requeuing", "newsletter", nl.ID, "error", scoreErr)
		return repo.Requeue(ctx, nl.ID)
	}

	var tr *ai.TruncatedError
	if errors.As(scoreErr, &tr) {
		// A retry is only worthwhile when sampling is stochastic — at temperature
		// 0 the model is deterministic, so a re-run reproduces the identical
		// truncation and the retries are pure wasted GPU time (CTFG-45). Salvage
		// immediately in that case.
		if !w.scorer.Deterministic() && nl.ScoringAttempts < w.maxAttempts {
			w.logger.Warn("scoring output truncated; requeuing for retry",
				"newsletter", nl.ID, "attempt", nl.ScoringAttempts, "max", w.maxAttempts, "recovered", tr.Recovered)
			return repo.Requeue(ctx, nl.ID)
		}
		if len(items) > 0 {
			w.logger.Warn("scoring truncated; persisting salvaged items",
				"newsletter", nl.ID, "attempts", nl.ScoringAttempts, "recovered", len(items), "deterministic", w.scorer.Deterministic())
			return w.persist(ctx, nl, items)
		}
		w.logger.Error("scoring truncated with nothing salvageable; marking failed",
			"newsletter", nl.ID, "attempts", nl.ScoringAttempts, "deterministic", w.scorer.Deterministic())
		return repo.MarkFailed(ctx, nl.ID, scoreErr.Error())
	}

	w.logger.Error("scoring terminal failure; marking failed", "newsletter", nl.ID, "error", scoreErr)
	return repo.MarkFailed(ctx, nl.ID, scoreErr.Error())
}

// persist writes the segmented stories and their scores, then flips the
// newsletter to scored — all in one transaction so a crash never leaves partial
// stories behind a still-scoring newsletter. Only story-kind items get their
// scoring fields written; ads/blurbs/promos are persisted unscored.
//
// (Re)scoring is idempotent (CTFG-40): any existing stories for the newsletter
// are deleted first so a re-score REPLACES them rather than appending
// duplicates. Reader engagement (bookmark / rating / opened) is carried over
// best-effort, matched by URL — stories without a URL can't be matched and
// reset, since re-segmentation gives them no stable identity.
func (w *Worker) persist(ctx context.Context, nl db.Newsletter, items []ai.ScoredItem) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stories := db.NewStoryRepo(tx)
	newsletters := db.NewNewsletterRepo(tx)

	// Snapshot engagement on the prior stories (if this is a re-score), then
	// replace them. First scoring finds none and deletes nothing.
	prior, err := stories.ListByNewsletter(ctx, nl.ID)
	if err != nil {
		return err
	}
	priorEng := engagementByURL(prior)
	if _, err := stories.DeleteByNewsletter(ctx, nl.ID); err != nil {
		return err
	}

	if len(items) == 0 {
		// The model found nothing worth segmenting; still a clean completion.
		if err := newsletters.UpdateStatus(ctx, nl.ID, db.StatusScored); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

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
		if inserted[i].Kind == db.KindStory {
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
		if err := reapplyEngagement(ctx, stories, inserted[i], priorEng); err != nil {
			return err
		}
	}

	if err := newsletters.UpdateStatus(ctx, nl.ID, db.StatusScored); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// engagement is the reader signal carried across a re-score.
type engagement struct {
	bookmarked bool
	rating     *int
	openedAt   *time.Time
}

// engagementByURL snapshots reader engagement from prior stories, keyed by URL.
// Stories with no URL, or with nothing to carry, are skipped.
func engagementByURL(stories []db.Story) map[string]engagement {
	out := make(map[string]engagement)
	for _, s := range stories {
		if s.URL == nil || *s.URL == "" {
			continue
		}
		if !s.Bookmarked && s.UserRating == nil && s.OpenedAt == nil {
			continue
		}
		out[*s.URL] = engagement{bookmarked: s.Bookmarked, rating: s.UserRating, openedAt: s.OpenedAt}
	}
	return out
}

// reapplyEngagement restores carried-over engagement onto a freshly inserted
// story when its URL matches one seen before the re-score.
func reapplyEngagement(ctx context.Context, stories *db.StoryRepo, s db.Story, prior map[string]engagement) error {
	if len(prior) == 0 || s.URL == nil || *s.URL == "" {
		return nil
	}
	e, ok := prior[*s.URL]
	if !ok {
		return nil
	}
	if e.bookmarked {
		if err := stories.SetBookmark(ctx, s.ID, true); err != nil {
			return err
		}
	}
	if e.rating != nil {
		if err := stories.SetRating(ctx, s.ID, *e.rating); err != nil {
			return err
		}
	}
	if e.openedAt != nil {
		if err := stories.MarkOpened(ctx, s.ID, *e.openedAt); err != nil {
			return err
		}
	}
	return nil
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
