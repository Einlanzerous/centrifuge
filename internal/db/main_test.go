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

	unlock := lockTestDB(pool)
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

// testDBLockKey is a shared pg_advisory_lock key. The db, ingest, and httpapi
// test packages all truncate the same centrifuge_test database, so `go test
// ./...` (which runs packages in parallel) would let one package's TRUNCATE wipe
// another's rows mid-test. Holding this lock for the duration of each package's
// run serializes their access regardless of -p.
const testDBLockKey = 918273645

// lockTestDB takes the cross-package serialization lock and returns a release
// func. The lock lives on a dedicated connection held until tests finish.
func lockTestDB(pool *pgxpool.Pool) func() {
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		panic("acquire lock conn: " + err.Error())
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", testDBLockKey); err != nil {
		panic("advisory lock: " + err.Error())
	}
	return func() {
		_, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", testDBLockKey)
		conn.Release()
	}
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
