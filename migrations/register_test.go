package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
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
		assertStableSourceIdentity(t, sources, []expectedSourceIdentity{
			{name: SourceNameProviderPostgres, key: SourceKeyProviderPostgres, order: SourceOrderProviderPostgres},
			{name: SourceNameGeneration, key: SourceKeyGeneration, order: SourceOrderGeneration},
			{name: SourceNameEditorial, key: SourceKeyEditorial, order: SourceOrderEditorial},
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
		assertStableSourceIdentity(t, sources, []expectedSourceIdentity{
			{name: SourceNameGeneration, key: SourceKeyGeneration, order: SourceOrderGeneration},
			{name: SourceNameEditorial, key: SourceKeyEditorial, order: SourceOrderEditorial},
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
		assertStableSourceIdentity(t, sources, []expectedSourceIdentity{
			{name: SourceNameProviderPostgres, key: SourceKeyProviderPostgres, order: SourceOrderProviderPostgres},
			{name: SourceNameGeneration, key: SourceKeyGeneration, order: SourceOrderGeneration},
			{name: SourceNameDispatch, key: SourceKeyDispatch, order: SourceOrderDispatch},
		})
	})
}

func TestOrderedSourcesUseStableIdentities(t *testing.T) {
	sources, err := OrderedSources(WithDispatchEnabled(true))
	if err != nil {
		t.Fatalf("OrderedSources: %v", err)
	}
	assertSourceNames(t, sources, []string{
		SourceNameProviderPostgres,
		SourceNameGeneration,
		SourceNameEditorial,
		SourceNameDispatch,
	})
	assertStableSourceIdentity(t, sources, []expectedSourceIdentity{
		{name: SourceNameProviderPostgres, key: SourceKeyProviderPostgres, order: SourceOrderProviderPostgres},
		{name: SourceNameGeneration, key: SourceKeyGeneration, order: SourceOrderGeneration},
		{name: SourceNameEditorial, key: SourceKeyEditorial, order: SourceOrderEditorial},
		{name: SourceNameDispatch, key: SourceKeyDispatch, order: SourceOrderDispatch},
	})
}

func TestLegacyOrderedSourcesUsePositionalIdentity(t *testing.T) {
	sources, err := LegacyOrderedSources(
		WithProfile(ProfileExternalProvider),
		WithProviderEnabled(true),
		WithEditorialEnabled(false),
		WithDispatchEnabled(true),
	)
	if err != nil {
		t.Fatalf("LegacyOrderedSources: %v", err)
	}
	assertSourceNames(t, sources, []string{
		SourceNameProviderPostgres,
		SourceNameGeneration,
		SourceNameDispatch,
	})
	assertPositionalSourceIdentity(t, sources)
}

func TestOrderedSourcesRejectsUnknownProfile(t *testing.T) {
	if _, err := OrderedSources(WithProfile(Profile("unknown"))); err == nil {
		t.Fatalf("expected profile validation error")
	}
}

func TestRegisterManagerComposesAfterStableHostSource(t *testing.T) {
	manager := persistence.NewMigrations()
	hostFS := fstest.MapFS{
		"0001_host.up.sql":   {Data: []byte("SELECT 1;")},
		"0001_host.down.sql": {Data: []byte("SELECT 1;")},
	}
	err := manager.RegisterOrderedMigrationSources(
		persistence.NewStableOrderedMigrationSource("host-runtime", hostFS, "host-runtime", 100),
	)
	if err != nil {
		t.Fatalf("register host source: %v", err)
	}

	if err := RegisterManager(manager, WithDispatchEnabled(true)); err != nil {
		t.Fatalf("RegisterManager after stable host source: %v", err)
	}

	plan, err := manager.Plan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Plan after stable host source: %v", err)
	}
	assertSourceStablePlan(t, plan, []expectedPlanSource{
		{name: "host-runtime", key: "host_runtime", order: 100, position: 1},
		{name: SourceNameProviderPostgres, key: "go_search_provider_postgres", order: SourceOrderProviderPostgres, position: 2},
		{name: SourceNameGeneration, key: "go_search_generation", order: SourceOrderGeneration, position: 3},
		{name: SourceNameEditorial, key: "go_search_editorial", order: SourceOrderEditorial, position: 4},
		{name: SourceNameDispatch, key: "go_search_dispatch", order: SourceOrderDispatch, position: 5},
	})
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
		plan, err := manager.Plan(ctx, db)
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if err := manager.Migrate(ctx, db); err != nil {
			t.Fatalf("Migrate: %v", err)
		}

		assertTableExists(t, ctx, db, "search_documents", true)
		assertTableExists(t, ctx, db, "search_generations", true)
		assertTableExists(t, ctx, db, "search_editorial_rules", true)
		assertTableExists(t, ctx, db, "search_job_dispatches", true)
		assertStableMigrationMarkers(t, ctx, db, plan)
		assertStableSourceMetadata(t, ctx, db, []expectedSourceMetadata{
			{name: SourceNameProviderPostgres, key: "go_search_provider_postgres", order: SourceOrderProviderPostgres, position: 1},
			{name: SourceNameGeneration, key: "go_search_generation", order: SourceOrderGeneration, position: 2},
			{name: SourceNameEditorial, key: "go_search_editorial", order: SourceOrderEditorial, position: 3},
			{name: SourceNameDispatch, key: "go_search_dispatch", order: SourceOrderDispatch, position: 4},
		})
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

func TestStableBackfillForLegacySearchMarkersPostgres(t *testing.T) {
	withPostgresDB(t, func(ctx context.Context, _ *sql.DB, db *bun.DB) {
		legacySources, err := LegacyOrderedSources(WithDispatchEnabled(true))
		if err != nil {
			t.Fatalf("LegacyOrderedSources: %v", err)
		}

		legacy := persistence.NewMigrations()
		if err := legacy.RegisterOrderedMigrationSources(legacySources...); err != nil {
			t.Fatalf("register legacy sources: %v", err)
		}
		if err := legacy.Migrate(ctx, db); err != nil {
			t.Fatalf("legacy migrate: %v", err)
		}

		stable := persistence.NewMigrations()
		if err := RegisterManager(stable, WithDispatchEnabled(true)); err != nil {
			t.Fatalf("RegisterManager stable: %v", err)
		}
		if err := stable.BackfillStableOrderedMigrationMarkers(ctx, db, legacySources); err != nil {
			t.Fatalf("BackfillStableOrderedMigrationMarkers: %v", err)
		}

		plan, err := stable.Plan(ctx, db)
		if err != nil {
			t.Fatalf("stable Plan: %v", err)
		}
		if len(plan.Entries) == 0 {
			t.Fatalf("expected stable plan entries")
		}
		assertBackfilledAliases(t, ctx, db, legacySources, plan)
		for _, entry := range plan.Entries {
			if !entry.Applied {
				t.Fatalf("expected backfilled entry to be applied: %+v", entry)
			}
			if !strings.HasPrefix(entry.SyntheticName, "ordsrc_") {
				t.Fatalf("expected source-stable synthetic name after backfill, got %q", entry.SyntheticName)
			}
		}
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
			if entry.IdentityMode != persistence.OrderedMigrationIdentitySourceStable {
				t.Fatalf("unexpected identity mode in subset plan: got=%s want=%s", entry.IdentityMode, persistence.OrderedMigrationIdentitySourceStable)
			}
			if entry.SourceOrder != SourceOrderGeneration && entry.SourceOrder != SourceOrderEditorial {
				t.Fatalf("unexpected source order in subset plan: %+v", entry)
			}
			if entry.SourceKey != "go_search_generation" && entry.SourceKey != "go_search_editorial" {
				t.Fatalf("unexpected normalized source key in subset plan: %+v", entry)
			}
			if !strings.HasPrefix(entry.SyntheticName, "ordsrc_") {
				t.Fatalf("expected source-stable synthetic name, got %q", entry.SyntheticName)
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

type expectedSourceIdentity struct {
	name  string
	key   string
	order int
}

type expectedPlanSource struct {
	name     string
	key      string
	order    int
	position int
}

func assertSourceStablePlan(t *testing.T, plan *persistence.MigrationPlan, expected []expectedPlanSource) {
	t.Helper()
	if plan == nil {
		t.Fatalf("expected migration plan")
	}
	expectedNames := make([]string, 0, len(expected))
	expectedByName := make(map[string]expectedPlanSource, len(expected))
	for _, source := range expected {
		expectedNames = append(expectedNames, source.name)
		expectedByName[source.name] = source
	}
	if !reflect.DeepEqual(plan.SelectedSources, expectedNames) {
		t.Fatalf("selected sources mismatch: got=%v want=%v", plan.SelectedSources, expectedNames)
	}

	seen := make(map[string]struct{}, len(expected))
	for _, entry := range plan.Entries {
		want, ok := expectedByName[entry.SourceName]
		if !ok {
			t.Fatalf("unexpected source in plan: %+v", entry)
		}
		seen[entry.SourceName] = struct{}{}
		if entry.IdentityMode != persistence.OrderedMigrationIdentitySourceStable {
			t.Fatalf("entry %q identity mode mismatch: got=%s want=%s", entry.SyntheticName, entry.IdentityMode, persistence.OrderedMigrationIdentitySourceStable)
		}
		if entry.SourceKey != want.key ||
			entry.SourceOrder != want.order ||
			entry.ResolvedPosition != want.position {
			t.Fatalf("entry %q metadata mismatch: got key=%q order=%d position=%d want=%+v", entry.SyntheticName, entry.SourceKey, entry.SourceOrder, entry.ResolvedPosition, want)
		}
		if !strings.HasPrefix(entry.SyntheticName, "ordsrc_") {
			t.Fatalf("expected source-stable synthetic name, got %q", entry.SyntheticName)
		}
	}
	for _, source := range expected {
		if _, ok := seen[source.name]; !ok {
			t.Fatalf("missing plan entries for source %q", source.name)
		}
	}
}

func assertStableSourceIdentity(t *testing.T, sources []persistence.OrderedMigrationSource, expected []expectedSourceIdentity) {
	t.Helper()
	if len(sources) != len(expected) {
		t.Fatalf("source count mismatch: got=%d want=%d", len(sources), len(expected))
	}
	for i, source := range sources {
		want := expected[i]
		if source.Name != want.name {
			t.Fatalf("source %d name mismatch: got=%q want=%q", i, source.Name, want.name)
		}
		if source.IdentityMode != persistence.OrderedMigrationIdentitySourceStable {
			t.Fatalf("source %q identity mode mismatch: got=%s want=%s", source.Name, source.IdentityMode, persistence.OrderedMigrationIdentitySourceStable)
		}
		if source.SourceKey != want.key {
			t.Fatalf("source %q key mismatch: got=%q want=%q", source.Name, source.SourceKey, want.key)
		}
		if source.Order != want.order {
			t.Fatalf("source %q order mismatch: got=%d want=%d", source.Name, source.Order, want.order)
		}
		if source.SourceKey == "" {
			t.Fatalf("source %q has empty source key", source.Name)
		}
		if source.Order <= 0 {
			t.Fatalf("source %q has non-positive source order %d", source.Name, source.Order)
		}
	}
}

func assertPositionalSourceIdentity(t *testing.T, sources []persistence.OrderedMigrationSource) {
	t.Helper()
	for _, source := range sources {
		if source.IdentityMode != persistence.OrderedMigrationIdentityPositional {
			t.Fatalf("source %q identity mode mismatch: got=%s want=%s", source.Name, source.IdentityMode, persistence.OrderedMigrationIdentityPositional)
		}
		if source.SourceKey != "" {
			t.Fatalf("source %q legacy source key mismatch: got=%q want empty", source.Name, source.SourceKey)
		}
		if source.Order != 0 {
			t.Fatalf("source %q legacy order mismatch: got=%d want 0", source.Name, source.Order)
		}
		if len(source.DependsOn) != 0 {
			t.Fatalf("source %q legacy dependencies mismatch: got=%v want empty", source.Name, source.DependsOn)
		}
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
		t.Skipf("testkit.Integration.Postgres.DSN is not set; set %s to run Postgres integration tests", testkit.EnvPostgresDSN)
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
		"DROP TABLE IF EXISTS bun_ordered_migration_aliases",
		"DROP TABLE IF EXISTS bun_ordered_migration_sources",
		"DROP TABLE IF EXISTS bun_migrations",
		"DROP TABLE IF EXISTS search_job_dispatches",
		"DROP TABLE IF EXISTS search_editorial_rules",
		"DROP TABLE IF EXISTS search_generations",
		"DROP TABLE IF EXISTS search_documents",
		"DROP FUNCTION IF EXISTS search_documents_tsvector(TEXT, TEXT, TEXT, TEXT)",
		"DROP FUNCTION IF EXISTS search_documents_text(TEXT, TEXT, TEXT)",
	}
	for _, statement := range statements {
		if _, err := db.NewRaw(statement).Exec(ctx); err != nil {
			t.Fatalf("reset statement %q: %v", statement, err)
		}
	}
}

func assertStableMigrationMarkers(t *testing.T, ctx context.Context, db *bun.DB, plan *persistence.MigrationPlan) {
	t.Helper()
	if plan == nil {
		t.Fatalf("expected migration plan")
	}
	for _, entry := range plan.Entries {
		if entry.IdentityMode != persistence.OrderedMigrationIdentitySourceStable {
			t.Fatalf("entry %q identity mode mismatch: got=%s want=%s", entry.SyntheticName, entry.IdentityMode, persistence.OrderedMigrationIdentitySourceStable)
		}
		if !strings.HasPrefix(entry.SyntheticName, "ordsrc_") {
			t.Fatalf("entry %q does not use source-stable name", entry.SyntheticName)
		}
		var count int
		if err := db.NewRaw("SELECT COUNT(1) FROM bun_migrations WHERE name = ?", entry.SyntheticName).Scan(ctx, &count); err != nil {
			t.Fatalf("bun_migrations lookup %q: %v", entry.SyntheticName, err)
		}
		if count != 1 {
			t.Fatalf("expected one applied marker for %q, got %d", entry.SyntheticName, count)
		}
	}
}

type expectedSourceMetadata struct {
	name     string
	key      string
	order    int
	position int
}

func assertStableSourceMetadata(t *testing.T, ctx context.Context, db *bun.DB, expected []expectedSourceMetadata) {
	t.Helper()
	type sourceRow struct {
		SourceKey        string `bun:"source_key"`
		SourceName       string `bun:"source_name"`
		SourceOrder      int    `bun:"source_order"`
		ResolvedPosition int    `bun:"resolved_position"`
		IdentityMode     string `bun:"identity_mode"`
	}
	var rows []sourceRow
	if err := db.NewRaw(`
SELECT source_key, source_name, source_order, resolved_position, identity_mode
FROM bun_ordered_migration_sources
ORDER BY source_order
`).Scan(ctx, &rows); err != nil {
		t.Fatalf("bun_ordered_migration_sources query: %v", err)
	}
	if len(rows) != len(expected) {
		t.Fatalf("metadata row count mismatch: got=%d want=%d rows=%+v", len(rows), len(expected), rows)
	}
	for i, row := range rows {
		want := expected[i]
		if row.SourceKey != want.key ||
			row.SourceName != want.name ||
			row.SourceOrder != want.order ||
			row.ResolvedPosition != want.position ||
			row.IdentityMode != persistence.OrderedMigrationIdentitySourceStable.String() {
			t.Fatalf("metadata row %d mismatch: got=%+v want=%+v identity=%q", i, row, want, persistence.OrderedMigrationIdentitySourceStable.String())
		}
	}
}

func assertBackfilledAliases(
	t *testing.T,
	ctx context.Context,
	db *bun.DB,
	legacySources []persistence.OrderedMigrationSource,
	plan *persistence.MigrationPlan,
) {
	t.Helper()
	sourcePositions := make(map[string]int, len(legacySources))
	for i, source := range legacySources {
		sourcePositions[source.Name] = i + 1
	}
	sourceMigrationPositions := make(map[string]int, len(legacySources))
	for _, entry := range plan.Entries {
		sourcePosition, ok := sourcePositions[entry.SourceName]
		if !ok {
			t.Fatalf("missing legacy source position for %q", entry.SourceName)
		}
		sourceMigrationPositions[entry.SourceName]++
		legacyName := fmt.Sprintf("ord_%06d_%06d", sourcePosition, sourceMigrationPositions[entry.SourceName])

		var count int
		if err := db.NewRaw(`
SELECT COUNT(1)
FROM bun_ordered_migration_aliases
WHERE legacy_name = ? AND stable_name = ? AND source_key = ?
`, legacyName, entry.SyntheticName, entry.SourceKey).Scan(ctx, &count); err != nil {
			t.Fatalf("alias lookup %q -> %q: %v", legacyName, entry.SyntheticName, err)
		}
		if count != 1 {
			t.Fatalf("expected one alias %q -> %q for %q, got %d", legacyName, entry.SyntheticName, entry.SourceKey, count)
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
