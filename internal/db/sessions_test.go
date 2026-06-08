package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSessionGetOrCreateAndTouch(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE user_sessions`); err != nil {
		t.Fatalf("truncate sessions: %v", err)
	}
	repo := NewSessionRepo(pool)

	s, err := repo.GetOrCreateDefault(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateDefault: %v", err)
	}
	if s.Label != DefaultSessionLabel {
		t.Fatalf("label = %q, want %q", s.Label, DefaultSessionLabel)
	}
	if s.LastViewedAt != nil {
		t.Fatalf("a fresh session should never have been viewed: %v", s.LastViewedAt)
	}

	// Idempotent: same row, same id.
	again, err := repo.GetOrCreateDefault(ctx)
	if err != nil {
		t.Fatalf("GetOrCreateDefault again: %v", err)
	}
	if again.ID != s.ID {
		t.Fatalf("GetOrCreate not idempotent: %s vs %s", again.ID, s.ID)
	}

	when := time.Now().UTC().Truncate(time.Millisecond)
	touched, err := repo.TouchLastViewed(ctx, DefaultSessionLabel, when)
	if err != nil {
		t.Fatalf("TouchLastViewed: %v", err)
	}
	if touched.LastViewedAt == nil || !touched.LastViewedAt.Equal(when) {
		t.Fatalf("last_viewed_at = %v, want %v", touched.LastViewedAt, when)
	}

	// Unknown label reports ErrNoRows.
	if _, err := repo.TouchLastViewed(ctx, "nope", when); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}
