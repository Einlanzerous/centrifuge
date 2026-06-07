package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool is the shared pool for DB-backed tests; nil when DATABASE_URL_TEST is
// unset, in which case every test self-skips (CI has no live database).
var testPool *pgxpool.Pool

// TestMain wires up the test database: it reads DATABASE_URL_TEST directly and
// applies the repo's migrations before any test runs.
func TestMain(m *testing.M) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		os.Exit(m.Run())
	}

	ctx := context.Background()
	dir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		panic("resolve migrations dir: " + err.Error())
	}
	if err := db.Migrate(ctx, url, dir); err != nil {
		panic("migrate test db: " + err.Error())
	}

	pool, err := db.NewPool(ctx, url)
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

// testDBLockKey serializes shared centrifuge_test access across the db, ingest,
// httpapi, and worker test packages (which all truncate the same database). See
// the db package's TestMain for the full rationale.
const testDBLockKey = 918273645

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
// and returns the shared pool.
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
