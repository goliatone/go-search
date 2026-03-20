package bunstore

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func TestEditorialStoreIntegration(t *testing.T) {
	dsn := testkit.Integration.Postgres.DSN
	if dsn == "" {
		t.Skip("testkit.Integration.Postgres.DSN is not set")
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer sqlDB.Close()
	db := bun.NewDB(sqlDB, pgdialect.New())
	ctx := context.Background()
	if err := Migrations().Migrate(ctx, db); err != nil {
		t.Fatalf("migrate editorial store: %v", err)
	}
	store := New(db)
	now := time.Now()
	rule := types.EditorialRankRule{
		ID:         "rule-1",
		TargetType: "video",
		TargetID:   "video-1",
		Action:     types.EditorialActionPin,
		Weight:     10,
		Enabled:    true,
		StartsAt:   &now,
		Scope:      types.EditorialScope{Indexes: []string{"media"}, Locale: "en"},
	}
	if err := store.Upsert(ctx, rule); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	rules, err := store.ListApplicable(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Locale:  "en",
	})
	if err != nil {
		t.Fatalf("list applicable: %v", err)
	}
	if len(rules) == 0 {
		t.Fatalf("expected editorial rules")
	}
	enabled := true
	rules, err = store.List(ctx, types.EditorialRuleListRequest{
		Indexes: []string{"media"},
		Locale:  "en",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one listed rule, got %d", len(rules))
	}
	if err := store.SetEnabled(ctx, "rule-1", false); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	rules, err = store.ListApplicable(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Locale:  "en",
	})
	if err != nil {
		t.Fatalf("list applicable after disable: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected no applicable rules after disable, got %d", len(rules))
	}
	if err := store.Delete(ctx, "rule-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
