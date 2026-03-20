package bunstore

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func TestGenerationStoreIntegration(t *testing.T) {
	dsn := os.Getenv("GO_SEARCH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GO_SEARCH_TEST_POSTGRES_DSN is not set")
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	ctx := context.Background()
	if err := Migrations().Migrate(ctx, db); err != nil {
		t.Fatalf("migrate generation store: %v", err)
	}
	store := New(db)
	generation, err := store.Bump(ctx, "media")
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	if generation != 1 {
		t.Fatalf("expected generation 1, got %d", generation)
	}
	generation, err = store.Get(ctx, "media")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if generation != 1 {
		t.Fatalf("expected stored generation 1, got %d", generation)
	}
}
