package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SourceRepo persists and resolves publications/feeds.
type SourceRepo struct{ db DBTX }

// NewSourceRepo returns a SourceRepo over db (a pool or a transaction).
func NewSourceRepo(db DBTX) *SourceRepo { return &SourceRepo{db: db} }

const sourceCols = `id, name, kind, identity, created_at`

func scanSource(row pgx.Row) (*Source, error) {
	var s Source
	if err := row.Scan(&s.ID, &s.Name, &s.Kind, &s.Identity, &s.CreatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetOrCreate idempotently resolves a source by its (kind, identity). On first
// sight it inserts; on later calls it returns the existing row. name applies
// only on insert — it never clobbers an existing source's name.
func (r *SourceRepo) GetOrCreate(ctx context.Context, kind, identity, name string) (*Source, error) {
	const q = `
INSERT INTO sources (name, kind, identity)
VALUES ($1, $2, $3)
ON CONFLICT (kind, identity) DO UPDATE SET name = sources.name
RETURNING ` + sourceCols
	return scanSource(r.db.QueryRow(ctx, q, name, kind, identity))
}

// GetByID returns the source with the given id, or pgx.ErrNoRows.
func (r *SourceRepo) GetByID(ctx context.Context, id string) (*Source, error) {
	const q = `SELECT ` + sourceCols + ` FROM sources WHERE id = $1`
	return scanSource(r.db.QueryRow(ctx, q, id))
}
