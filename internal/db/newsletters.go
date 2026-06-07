package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// NewsletterRepo persists raw deliveries and drives their processing_status.
type NewsletterRepo struct{ db DBTX }

// NewNewsletterRepo returns a NewsletterRepo over db (a pool or a transaction).
func NewNewsletterRepo(db DBTX) *NewsletterRepo { return &NewsletterRepo{db: db} }

const newsletterCols = `id, source_id, message_id, subject, raw_html, body_text, ` +
	`received_at, dedupe_hash, ingested_at, processing_status`

func scanNewsletter(row pgx.Row) (*Newsletter, error) {
	var n Newsletter
	err := row.Scan(&n.ID, &n.SourceID, &n.MessageID, &n.Subject, &n.RawHTML,
		&n.BodyText, &n.ReceivedAt, &n.DedupeHash, &n.IngestedAt, &n.ProcessingStatus)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// Insert stores a newsletter, deduplicating against an existing delivery when
// possible. The returned bool reports whether a new row was inserted (false
// means an existing duplicate was returned untouched).
//
// Dedup precedence: a present message_id is enforced by the partial unique
// index; otherwise a present dedupe_hash is matched best-effort (the hash index
// is non-unique, so this is a soft check). Deliveries with neither always
// insert.
func (r *NewsletterRepo) Insert(ctx context.Context, n *Newsletter) (*Newsletter, bool, error) {
	if n.ProcessingStatus == "" {
		n.ProcessingStatus = StatusPending
	}

	// Soft dedup by content hash only when there's no message_id to enforce on.
	if n.MessageID == nil && n.DedupeHash != nil {
		existing, err := r.getByDedupeHash(ctx, *n.DedupeHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
		if existing != nil {
			return existing, false, nil
		}
	}

	const insert = `
INSERT INTO newsletters
	(source_id, message_id, subject, raw_html, body_text, received_at, dedupe_hash, processing_status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	args := []any{n.SourceID, n.MessageID, n.Subject, n.RawHTML, n.BodyText,
		n.ReceivedAt, n.DedupeHash, n.ProcessingStatus}

	if n.MessageID != nil {
		q := insert + ` ON CONFLICT (message_id) WHERE message_id IS NOT NULL DO NOTHING RETURNING ` + newsletterCols
		nl, err := scanNewsletter(r.db.QueryRow(ctx, q, args...))
		if errors.Is(err, pgx.ErrNoRows) {
			// Lost the race / duplicate message_id: return the existing row.
			existing, gerr := r.getByMessageID(ctx, *n.MessageID)
			if gerr != nil {
				return nil, false, gerr
			}
			return existing, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		return nl, true, nil
	}

	nl, err := scanNewsletter(r.db.QueryRow(ctx, insert+` RETURNING `+newsletterCols, args...))
	if err != nil {
		return nil, false, err
	}
	return nl, true, nil
}

func (r *NewsletterRepo) getByMessageID(ctx context.Context, messageID string) (*Newsletter, error) {
	const q = `SELECT ` + newsletterCols + ` FROM newsletters WHERE message_id = $1`
	return scanNewsletter(r.db.QueryRow(ctx, q, messageID))
}

func (r *NewsletterRepo) getByDedupeHash(ctx context.Context, hash string) (*Newsletter, error) {
	const q = `SELECT ` + newsletterCols + ` FROM newsletters WHERE dedupe_hash = $1 ORDER BY ingested_at DESC LIMIT 1`
	return scanNewsletter(r.db.QueryRow(ctx, q, hash))
}

// GetByID returns the newsletter with the given id, or pgx.ErrNoRows.
func (r *NewsletterRepo) GetByID(ctx context.Context, id string) (*Newsletter, error) {
	const q = `SELECT ` + newsletterCols + ` FROM newsletters WHERE id = $1`
	return scanNewsletter(r.db.QueryRow(ctx, q, id))
}

// FetchByStatus returns up to limit newsletters in the given processing_status,
// oldest first — the polling shape the scoring worker uses for pending_scoring.
func (r *NewsletterRepo) FetchByStatus(ctx context.Context, status string, limit int) ([]Newsletter, error) {
	const q = `SELECT ` + newsletterCols + ` FROM newsletters WHERE processing_status = $1 ORDER BY ingested_at LIMIT $2`
	rows, err := r.db.Query(ctx, q, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Newsletter
	for rows.Next() {
		n, err := scanNewsletter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// UpdateStatus transitions a newsletter's processing_status. It returns
// pgx.ErrNoRows if no newsletter has the given id.
func (r *NewsletterRepo) UpdateStatus(ctx context.Context, id, status string) error {
	ct, err := r.db.Exec(ctx, `UPDATE newsletters SET processing_status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
