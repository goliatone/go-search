package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
	"github.com/uptrace/bun"
)

type stubBunDB struct{}

func (stubBunDB) NewInsert() *bun.InsertQuery {
	return nil
}

func (stubBunDB) NewSelect() *bun.SelectQuery {
	return nil
}

func (stubBunDB) NewDelete() *bun.DeleteQuery {
	return nil
}

func (stubBunDB) NewRaw(string, ...any) *bun.RawQuery {
	return nil
}

func (stubBunDB) PingContext(context.Context) error {
	return nil
}

func (stubBunDB) RunInTx(ctx context.Context, opts *sql.TxOptions, fn func(context.Context, bun.Tx) error) error {
	return nil
}

func TestPostgresProviderExternalSchemaManagementSkipsInternalMigrations(t *testing.T) {
	provider, err := New(Config{
		DB:               stubBunDB{},
		SchemaManagement: SchemaManagementExternal,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := provider.EnsureIndex(context.Background(), types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	if !provider.schemaReady {
		t.Fatalf("expected schemaReady after external schema bootstrap")
	}
}

func TestPostgresProviderAutoSchemaManagementStillRequiresBunDBForMigrations(t *testing.T) {
	provider, err := New(Config{DB: stubBunDB{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = provider.EnsureIndex(context.Background(), types.IndexDefinition{Name: "media"})
	if err == nil {
		t.Fatalf("expected EnsureIndex to fail without *bun.DB in auto mode")
	}
	if !strings.Contains(err.Error(), "requires *bun.DB for migrations") {
		t.Fatalf("unexpected EnsureIndex error: %v", err)
	}
}

func TestSchemaManagementNormalizationDefaultsToAuto(t *testing.T) {
	cfg, err := normalizeConfig(Config{DB: stubBunDB{}})
	if err != nil {
		t.Fatalf("normalizeConfig: %v", err)
	}
	if cfg.SchemaManagement != SchemaManagementAuto {
		t.Fatalf("expected default schema management to normalize to auto, got %q", cfg.SchemaManagement)
	}
}

func TestPostgresProviderRejectsInvalidSchemaManagementMode(t *testing.T) {
	_, err := New(Config{
		DB:               stubBunDB{},
		SchemaManagement: SchemaManagementMode("invalid"),
	})
	if err == nil {
		t.Fatalf("expected invalid schema management mode to fail")
	}
	if !strings.Contains(err.Error(), "invalid schema management mode") {
		t.Fatalf("unexpected invalid schema management error: %v", err)
	}
}
