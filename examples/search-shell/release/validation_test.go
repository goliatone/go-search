package release

import (
	"context"
	"slices"
	"testing"

	commandregistry "github.com/goliatone/go-command/registry"
	"github.com/goliatone/go-search/examples/search-shell/internal/config"
)

func TestRunSearchV1RuntimeValidationProfileMemory(t *testing.T) {
	cfg, _, err := SearchV1RuntimeConfigForProvider("memory", false)
	if err != nil {
		t.Fatalf("memory config: %v", err)
	}

	report, err := RunSearchV1RuntimeValidationProfile(context.Background(), cfg)
	if err != nil {
		t.Fatalf("run memory validation: %v", err)
	}
	for _, check := range []string{
		"status",
		"grouped_archive",
		"heterogeneous_flat",
		"locale_policy",
		"scope_and_permissions",
		"editorial",
		"cache_and_reindex",
	} {
		if !slices.Contains(report.Checks, check) {
			t.Fatalf("expected check %q in %+v", check, report.Checks)
		}
	}
}

func TestRunSearchV1RouteValidationProfileDefaultConfig(t *testing.T) {
	commandregistry.WithTestRegistry(func() {
		cfg := config.Defaults()
		cfg.Env = "test"
		cfg.Server.PrintRoutes = false
		cfg.SearchDemo.CacheEnabled = true

		report, err := RunSearchV1RouteValidationProfile(context.Background(), cfg)
		if err != nil {
			t.Fatalf("run route validation: %v", err)
		}
		for _, check := range []string{
			"demo_health",
			"demo_search_api",
			"site_search_page",
			"site_search_api",
			"site_suggest_api",
			"topic_landing",
			"editorial_route_toggle",
		} {
			if !slices.Contains(report.Checks, check) {
				t.Fatalf("expected route check %q in %+v", check, report.Checks)
			}
		}
		if !report.EditorialEnabled {
			t.Fatalf("expected editorial route validation to default to enabled")
		}
	})
}

func TestRunSearchV1RouteValidationProfileEditorialDisabled(t *testing.T) {
	commandregistry.WithTestRegistry(func() {
		cfg := config.Defaults()
		cfg.Env = "test"
		cfg.Server.PrintRoutes = false
		cfg.SearchDemo.EditorialEnabled = false

		report, err := RunSearchV1RouteValidationProfile(context.Background(), cfg)
		if err != nil {
			t.Fatalf("run route validation: %v", err)
		}
		if report.EditorialEnabled {
			t.Fatalf("expected editorial validation report to record disabled state")
		}
		if !slices.Contains(report.Checks, "editorial_route_toggle") {
			t.Fatalf("expected editorial toggle check in %+v", report.Checks)
		}
	})
}

func TestRunSearchV1RuntimeValidationProfileExternalProviders(t *testing.T) {
	requireExternal := SearchV1RequireExternalProviders()

	for _, provider := range []string{"typesense", "postgres"} {
		t.Run(provider, func(t *testing.T) {
			cfg, available, err := SearchV1RuntimeConfigForProvider(provider, requireExternal)
			if err != nil {
				t.Fatalf("provider config: %v", err)
			}
			if !available {
				t.Skipf("%s release validation is not configured", provider)
			}

			report, err := RunSearchV1RuntimeValidationProfile(context.Background(), cfg)
			if err != nil {
				t.Fatalf("run %s validation: %v", provider, err)
			}
			if report.Provider != provider {
				t.Fatalf("provider = %q, want %q", report.Provider, provider)
			}
			if !slices.Contains(report.Checks, "grouped_archive") || !slices.Contains(report.Checks, "cache_and_reindex") {
				t.Fatalf("expected grouped archive and cache checks in %+v", report.Checks)
			}
		})
	}
}
