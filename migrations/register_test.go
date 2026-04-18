package migrations

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
	searchpostgres "github.com/goliatone/go-search/providers/postgres"
	dispatchbunstore "github.com/goliatone/go-search/stores/dispatch/bun"
	editorialbunstore "github.com/goliatone/go-search/stores/editorial/bun"
	generationbunstore "github.com/goliatone/go-search/stores/generation/bun"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestOrderedSourcesProfilesAndToggles(t *testing.T) {
	t.Run("postgres-provider-default", func(t *testing.T) {
		sources, err := OrderedSources()
		if err != nil {
			t.Fatalf("OrderedSources: %v", err)
		}
		assertSourceNames(t, sources, []string{
			SourceNameProviderPostgres,
			SourceNameGeneration,
			SourceNameEditorial,
		})
	})

	t.Run("external-provider-default", func(t *testing.T) {
		sources, err := OrderedSources(WithProfile(ProfileExternalProvider))
		if err != nil {
			t.Fatalf("OrderedSources: %v", err)
		}
		assertSourceNames(t, sources, []string{
			SourceNameGeneration,
			SourceNameEditorial,
		})
	})

	t.Run("toggles-override-profile", func(t *testing.T) {
		sources, err := OrderedSources(
			WithProfile(ProfileExternalProvider),
			WithProviderEnabled(true),
			WithEditorialEnabled(false),
			WithDispatchEnabled(true),
		)
		if err != nil {
			t.Fatalf("OrderedSources: %v", err)
		}
		assertSourceNames(t, sources, []string{
			SourceNameProviderPostgres,
			SourceNameGeneration,
			SourceNameDispatch,
		})
	})
}

func TestOrderedSourcesRejectsUnknownProfile(t *testing.T) {
	if _, err := OrderedSources(WithProfile(Profile("unknown"))); err == nil {
		t.Fatalf("expected profile validation error")
	}
}

func TestComponentMigrationFSRootsValidateAsDialectSources(t *testing.T) {
	testCases := []struct {
		name      string
		resolveFS func() (fs.FS, error)
	}{
		{name: SourceNameProviderPostgres, resolveFS: searchpostgres.GetMigrationsFS},
		{name: SourceNameGeneration, resolveFS: generationbunstore.GetMigrationsFS},
		{name: SourceNameEditorial, resolveFS: editorialbunstore.GetMigrationsFS},
		{name: SourceNameDispatch, resolveFS: dispatchbunstore.GetMigrationsFS},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := tc.resolveFS()
			if err != nil {
				t.Fatalf("resolve fs: %v", err)
			}
			manager := persistence.NewMigrations()
			manager.RegisterDialectMigrations(
				root,
				persistence.WithDialectSourceLabel(tc.name),
				persistence.WithValidationTargets("postgres"),
			)
			if err := manager.ValidateDialects(context.Background(), bun.NewDB(nil, pgdialect.New())); err != nil {
				t.Fatalf("ValidateDialects: %v", err)
			}
		})
	}
}

func TestRegisterAndRegisterManagerProduceSamePlanPostgres(t *testing.T) {
	withPostgresDB(t, func(ctx context.Context, sqlDB *sql.DB, db *bun.DB) {
		manager := persistence.NewMigrations()
		if err := RegisterManager(manager, WithDispatchEnabled(true)); err != nil {
			t.Fatalf("RegisterManager: %v", err)
		}

		managerPlan, err := manager.Plan(ctx, db)
		if err != nil {
			t.Fatalf("manager.Plan: %v", err)
		}

		client := newPostgresClient(t, sqlDB)
		if err := Register(client, WithDispatchEnabled(true)); err != nil {
			t.Fatalf("Register: %v", err)
		}

		clientPlan, err := client.Plan(ctx)
		if err != nil {
			t.Fatalf("client.Plan: %v", err)
		}

		if !reflect.DeepEqual(planSnapshot(managerPlan), planSnapshot(clientPlan)) {
			t.Fatalf("plan mismatch:\nmanager=%#v\nclient=%#v", planSnapshot(managerPlan), planSnapshot(clientPlan))
		}
	})
}

func TestRegisterManagerMigrateCreatesComposedSchemaPostgres(t *testing.T) {
	withPostgresDB(t, func(ctx context.Context, _ *sql.DB, db *bun.DB) {
		manager := persistence.NewMigrations()
		if err := RegisterManager(manager, WithDispatchEnabled(true)); err != nil {
			t.Fatalf("RegisterManager: %v", err)
		}
		if err := manager.Migrate(ctx, db); err != nil {
			t.Fatalf("Migrate: %v", err)
		}

		assertTableExists(t, ctx, db, "search_documents", true)
		assertTableExists(t, ctx, db, "search_generations", true)
		assertTableExists(t, ctx, db, "search_editorial_rules", true)
		assertTableExists(t, ctx, db, "search_job_dispatches", true)
	})
}

func TestRegisterManagerMigrateExternalProviderProfilePostgres(t *testing.T) {
	withPostgresDB(t, func(ctx context.Context, _ *sql.DB, db *bun.DB) {
		manager := persistence.NewMigrations()
		if err := RegisterManager(
			manager,
			WithProfile(ProfileExternalProvider),
			WithDispatchEnabled(true),
		); err != nil {
			t.Fatalf("RegisterManager: %v", err)
		}
		if err := manager.Migrate(ctx, db); err != nil {
			t.Fatalf("Migrate: %v", err)
		}

		assertTableExists(t, ctx, db, "search_documents", false)
		assertTableExists(t, ctx, db, "search_generations", true)
		assertTableExists(t, ctx, db, "search_editorial_rules", true)
		assertTableExists(t, ctx, db, "search_job_dispatches", true)
	})
}

func TestPlanSourcesSupportsCanonicalSourceNamesPostgres(t *testing.T) {
	withPostgresDB(t, func(ctx context.Context, _ *sql.DB, db *bun.DB) {
		manager := persistence.NewMigrations()
		if err := RegisterManager(manager, WithDispatchEnabled(true)); err != nil {
			t.Fatalf("RegisterManager: %v", err)
		}

		plan, err := manager.PlanSources(ctx, db, SourceNameGeneration, SourceNameEditorial)
		if err != nil {
			t.Fatalf("PlanSources: %v", err)
		}

		expectedSelected := []string{SourceNameGeneration, SourceNameEditorial}
		if !reflect.DeepEqual(plan.SelectedSources, expectedSelected) {
			t.Fatalf("selected sources mismatch: got=%v want=%v", plan.SelectedSources, expectedSelected)
		}
		if len(plan.Entries) != 2 {
			t.Fatalf("expected 2 plan entries, got %d", len(plan.Entries))
		}
		for _, entry := range plan.Entries {
			if entry.SourceName != SourceNameGeneration && entry.SourceName != SourceNameEditorial {
				t.Fatalf("unexpected source in subset plan: %+v", entry)
			}
		}
	})
}

func TestRegisterManagerFailsLoudlyOnWrongDialect(t *testing.T) {
	withSQLiteClient(t, func(ctx context.Context, client *persistence.Client) {
		if err := Register(client); err != nil {
			t.Fatalf("Register: %v", err)
		}
		err := client.Migrate(ctx)
		if err == nil {
			t.Fatalf("expected migrate to fail on sqlite for postgres-only sources")
		}
	})
}

func TestComponentMigrationManagersFailLoudlyOnWrongDialect(t *testing.T) {
	withSQLiteClient(t, func(ctx context.Context, client *persistence.Client) {
		err := generationbunstore.Migrations().Migrate(ctx, client.DB())
		if err == nil {
			t.Fatalf("expected generation store migrations to fail on sqlite")
		}
	})
}

func TestLegacyPlainManagersRemainUnsafePostgres(t *testing.T) {
	withPostgresDB(t, func(ctx context.Context, _ *sql.DB, db *bun.DB) {
		if err := generationbunstore.Migrations().Migrate(ctx, db); err != nil {
			t.Fatalf("generation migrate: %v", err)
		}
		if err := editorialbunstore.Migrations().Migrate(ctx, db); err != nil {
			t.Fatalf("editorial migrate: %v", err)
		}

		assertTableExists(t, ctx, db, "search_generations", true)
		assertTableExists(t, ctx, db, "search_editorial_rules", false)

		var count int
		if err := db.NewRaw("SELECT COUNT(1) FROM bun_migrations WHERE name = '001'").Scan(ctx, &count); err != nil {
			t.Fatalf("bun_migrations count: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected one raw 001 migration row, got %d", count)
		}
	})
}

func TestOrderedProviderGenerationCompositionSupportsExternalProviderPostgres(t *testing.T) {
	withPostgresDB(t, func(ctx context.Context, _ *sql.DB, db *bun.DB) {
		manager := persistence.NewMigrations()
		if err := RegisterManager(
			manager,
			WithProfile(ProfilePostgresProvider),
			WithEditorialEnabled(false),
			WithDispatchEnabled(false),
		); err != nil {
			t.Fatalf("RegisterManager: %v", err)
		}
		if err := manager.Migrate(ctx, db); err != nil {
			t.Fatalf("Migrate: %v", err)
		}

		provider, err := searchpostgres.New(searchpostgres.Config{
			DB:               db,
			SchemaManagement: searchpostgres.SchemaManagementExternal,
		})
		if err != nil {
			t.Fatalf("provider.New: %v", err)
		}
		if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
			t.Fatalf("EnsureIndex: %v", err)
		}

		store := generationbunstore.New(generationbunstore.Config{DB: db})
		generation, err := store.Bump(ctx, "media")
		if err != nil {
			t.Fatalf("generation bump: %v", err)
		}
		if generation != 1 {
			t.Fatalf("expected generation 1, got %d", generation)
		}

		assertTableExists(t, ctx, db, "search_documents", true)
		assertTableExists(t, ctx, db, "search_generations", true)
		assertTableExists(t, ctx, db, "search_editorial_rules", false)
	})
}

func assertSourceNames(t *testing.T, sources []persistence.OrderedMigrationSource, expected []string) {
	t.Helper()
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Name)
	}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("source order mismatch: got=%v want=%v", names, expected)
	}
}

type postgresTestPersistenceConfig struct {
	driver string
	server string
}

func (c postgresTestPersistenceConfig) GetDebug() bool {
	return false
}

func (c postgresTestPersistenceConfig) GetDriver() string {
	if c.driver != "" {
		return c.driver
	}
	return "postgres"
}

func (c postgresTestPersistenceConfig) GetServer() string {
	return c.server
}

func (c postgresTestPersistenceConfig) GetPingTimeout() time.Duration {
	return time.Second
}

func (c postgresTestPersistenceConfig) GetOtelIdentifier() string {
	return ""
}

func newPostgresClient(t *testing.T, sqlDB *sql.DB) *persistence.Client {
	t.Helper()
	client, err := persistence.New(
		postgresTestPersistenceConfig{server: testkit.Integration.Postgres.DSN},
		sqlDB,
		pgdialect.New(),
	)
	if err != nil {
		t.Fatalf("persistence.New: %v", err)
	}
	return client
}

func withPostgresDB(t *testing.T, fn func(context.Context, *sql.DB, *bun.DB)) {
	t.Helper()
	dsn := testkit.Integration.Postgres.DSN
	if dsn == "" {
		t.Skip("testkit.Integration.Postgres.DSN is not set")
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db := bun.NewDB(sqlDB, pgdialect.New())
	ctx := context.Background()
	resetSearchSchema(t, ctx, db)
	fn(ctx, sqlDB, db)
	resetSearchSchema(t, ctx, db)
}

func withSQLiteClient(t *testing.T, fn func(context.Context, *persistence.Client)) {
	t.Helper()
	client := newSQLiteClient(t)
	fn(context.Background(), client)
}

func newSQLiteClient(t *testing.T) *persistence.Client {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "migrations.db") + "?cache=shared&_fk=1"
	sqlDB, err := sql.Open(sqliteshim.ShimName, dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	client, err := persistence.New(
		postgresTestPersistenceConfig{driver: sqliteshim.ShimName, server: dsn},
		sqlDB,
		sqlitedialect.New(),
	)
	if err != nil {
		t.Fatalf("persistence.New sqlite: %v", err)
	}
	return client
}

func resetSearchSchema(t *testing.T, ctx context.Context, db *bun.DB) {
	t.Helper()
	statements := []string{
		"DROP TABLE IF EXISTS bun_migration_locks",
		"DROP TABLE IF EXISTS bun_migrations",
		"DROP TABLE IF EXISTS search_job_dispatches",
		"DROP TABLE IF EXISTS search_editorial_rules",
		"DROP TABLE IF EXISTS search_generations",
		"DROP TABLE IF EXISTS search_documents",
	}
	for _, statement := range statements {
		if _, err := db.NewRaw(statement).Exec(ctx); err != nil {
			t.Fatalf("reset statement %q: %v", statement, err)
		}
	}
}

func assertTableExists(t *testing.T, ctx context.Context, db *bun.DB, table string, expected bool) {
	t.Helper()
	var resolved *string
	if err := db.NewRaw("SELECT to_regclass(?)", "public."+table).Scan(ctx, &resolved); err != nil {
		t.Fatalf("table lookup %q: %v", table, err)
	}
	if expected && resolved == nil {
		t.Fatalf("expected table %q to exist", table)
	}
	if !expected && resolved != nil {
		t.Fatalf("expected table %q to be absent, got %q", table, *resolved)
	}
}

func planSnapshot(plan *persistence.MigrationPlan) []string {
	if plan == nil {
		return nil
	}
	snapshot := make([]string, 0, len(plan.SelectedSources)+len(plan.Entries))
	snapshot = append(snapshot, plan.SelectedSources...)
	for _, entry := range plan.Entries {
		snapshot = append(snapshot,
			entry.SourceName,
			entry.SourceLabel,
			entry.SyntheticName,
			entry.OriginalVersion,
			entry.OriginalComment,
			entry.UpPath,
			entry.DownPath,
			string(entry.SourceKind),
		)
	}
	return snapshot
}
