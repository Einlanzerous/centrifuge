package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool is the shared pool for DB-backed tests. It is nil when
// DATABASE_URL_TEST is unset, in which case every test self-skips.
var testPool *pgxpool.Pool

// TestMain wires up the test database. It reads DATABASE_URL_TEST directly
// rather than config.Load(), which hard-requires DATABASE_URL, and applies the
// repo's migrations against it before any test runs.
func TestMain(m *testing.M) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		// No test DB configured — run anyway so each test reports as skipped.
		os.Exit(m.Run())
	}

	ctx := context.Background()
	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		panic("resolve migrations dir: " + err.Error())
	}
	if err := Migrate(ctx, url, dir); err != nil {
		panic("migrate test db: " + err.Error())
	}

	pool, err := NewPool(ctx, url)
	if err != nil {
		panic("open test pool: " + err.Error())
	}
	testPool = pool

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// setupDB skips when no test DB is configured, otherwise truncates all tables
// for a clean slate and returns the shared pool.
func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testPool == nil {
		t.Skip("DATABASE_URL_TEST not set; skipping DB-backed test")
	}
	if _, err := testPool.Exec(context.Background(), `TRUNCATE stories, newsletters, sources CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return testPool
}

// ptr returns a pointer to v — convenience for the many nullable columns.
func ptr[T any](v T) *T { return &v }

// seedNewsletter creates a source and one newsletter, returning both ids. It is
// the common starting point for story tests.
func seedNewsletter(t *testing.T, pool *pgxpool.Pool) (sourceID, newsletterID string) {
	t.Helper()
	ctx := context.Background()
	src, err := NewSourceRepo(pool).GetOrCreate(ctx, SourceKindNewsletter, "news@example.com", "Example News")
	if err != nil {
		t.Fatalf("seed source: %v", err)
	}
	nl, _, err := NewNewsletterRepo(pool).Insert(ctx, &Newsletter{
		SourceID:  src.ID,
		MessageID: ptr("<seed@example.com>"),
		Subject:   ptr("Seed issue"),
	})
	if err != nil {
		t.Fatalf("seed newsletter: %v", err)
	}
	return src.ID, nl.ID
}
