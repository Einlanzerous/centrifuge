package httpapi

import (
	"context"
	"os"
	"testing"

	"github.com/Einlanzerous/centrifuge"
	"github.com/Einlanzerous/centrifuge/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool backs DB-gated handler tests. It is nil when DATABASE_URL_TEST is
// unset; those tests self-skip, while the auth/parse tests run regardless.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		os.Exit(m.Run())
	}

	ctx := context.Background()
	if err := db.Migrate(ctx, url, centrifuge.MigrationsFS, "migrations"); err != nil {
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
// and httpapi test packages (which all truncate the same database). See the db
// package's TestMain for the full rationale.
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

// dbPool skips the test when no test DB is configured, otherwise truncates and
// returns the shared pool.
func dbPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testPool == nil {
		t.Skip("DATABASE_URL_TEST not set; skipping DB-backed test")
	}
	if _, err := testPool.Exec(context.Background(), `TRUNCATE stories, newsletters, sources, user_sessions CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return testPool
}
