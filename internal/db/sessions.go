package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// SessionRepo reads and updates per-session "last looked" state.
type SessionRepo struct{ db DBTX }

// NewSessionRepo returns a SessionRepo over db (a pool or a transaction).
func NewSessionRepo(db DBTX) *SessionRepo { return &SessionRepo{db: db} }

const sessionCols = `id, label, last_viewed_at, created_at, updated_at`

func scanSession(row pgx.Row) (*Session, error) {
	var s Session
	if err := row.Scan(&s.ID, &s.Label, &s.LastViewedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetOrCreate idempotently resolves a session by its label, inserting on first
// sight. The single-user build calls this with DefaultSessionLabel.
func (r *SessionRepo) GetOrCreate(ctx context.Context, label string) (*Session, error) {
	const q = `
INSERT INTO user_sessions (label)
VALUES ($1)
ON CONFLICT (label) DO UPDATE SET label = user_sessions.label
RETURNING ` + sessionCols
	return scanSession(r.db.QueryRow(ctx, q, label))
}

// GetOrCreateDefault resolves the single implicit user's session.
func (r *SessionRepo) GetOrCreateDefault(ctx context.Context) (*Session, error) {
	return r.GetOrCreate(ctx, DefaultSessionLabel)
}

// TouchLastViewed stamps a session's last_viewed_at to at and returns the
// updated row — this is the "I've now seen the Today feed" mutation. Returns
// pgx.ErrNoRows if the label is unknown.
func (r *SessionRepo) TouchLastViewed(ctx context.Context, label string, at time.Time) (*Session, error) {
	const q = `
UPDATE user_sessions SET last_viewed_at = $2, updated_at = now()
WHERE label = $1
RETURNING ` + sessionCols
	return scanSession(r.db.QueryRow(ctx, q, label, at))
}
