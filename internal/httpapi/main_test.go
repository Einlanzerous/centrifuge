package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// dbPool skips the test when no test DB is configured, otherwise truncates and
// returns the shared pool.
func dbPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testPool == nil {
		t.Skip("DATABASE_URL_TEST not set; skipping DB-backed test")
	}
	if _, err := testPool.Exec(context.Background(), `TRUNCATE stories, newsletters, sources CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return testPool
}
