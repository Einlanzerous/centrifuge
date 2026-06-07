package db

import (
	"context"
	"testing"
)

func TestSourceGetOrCreate(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	repo := NewSourceRepo(pool)

	first, err := repo.GetOrCreate(ctx, SourceKindNewsletter, "brew@morningbrew.com", "Morning Brew")
	if err != nil {
		t.Fatalf("first GetOrCreate: %v", err)
	}
	if first.ID == "" {
		t.Fatal("expected a generated id")
	}
	if first.Kind != SourceKindNewsletter || first.Identity != "brew@morningbrew.com" {
		t.Fatalf("unexpected source: %+v", first)
	}

	// Same (kind, identity) resolves to the same row and does not clobber name.
	again, err := repo.GetOrCreate(ctx, SourceKindNewsletter, "brew@morningbrew.com", "Different Name")
	if err != nil {
		t.Fatalf("second GetOrCreate: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("expected same id, got %s vs %s", again.ID, first.ID)
	}
	if again.Name != "Morning Brew" {
		t.Fatalf("name was clobbered: %q", again.Name)
	}

	// Same identity under a different kind is a distinct source.
	rss, err := repo.GetOrCreate(ctx, SourceKindRSS, "brew@morningbrew.com", "Brew RSS")
	if err != nil {
		t.Fatalf("rss GetOrCreate: %v", err)
	}
	if rss.ID == first.ID {
		t.Fatal("expected distinct source for different kind")
	}

	got, err := repo.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != first.ID {
		t.Fatalf("GetByID returned wrong row: %+v", got)
	}
}
