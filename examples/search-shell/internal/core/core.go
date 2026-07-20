package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/goliatone/go-search/examples/search-shell/internal/config"
	"github.com/goliatone/go-search/examples/search-shell/internal/searchdemo"

	"github.com/goliatone/go-admin/pkg/admin"
	"github.com/goliatone/go-admin/pkg/client"
	"github.com/goliatone/go-admin/quickstart"
	auth "github.com/goliatone/go-auth"
	"github.com/goliatone/go-featuregate/adapters/configadapter"
	fggate "github.com/goliatone/go-featuregate/gate"
	"github.com/goliatone/go-featuregate/resolver"
	"github.com/goliatone/go-router"
)

type FeatureStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type Core struct {
	Config      *config.AppConfig   `json:"config"`
	Logger      *slog.Logger        `json:"logger"`
	StartedAt   time.Time           `json:"started_at"`
	Search      *searchdemo.Runtime `json:"search"`
	AdminConfig admin.Config
	Server      router.Server[*fiber.App]
	Router      router.Router[*fiber.App]
	Fiber       *fiber.App

	Admin              *admin.Admin
	SiteSearchProvider admin.SearchProvider
	SearchOperations   *admin.GoSearchOperations
	Authenticator      *admin.GoAuthAuthenticator
	AuthCookieName     string
	Auther             *auth.Auther
	RouteAuthenticator *auth.RouteAuthenticator
	DemoIdentity       DemoIdentity
	routeAuthConfig    authRuntimeConfig
}

func New(_ context.Context, cfg *config.AppConfig) (*Core, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	adminCfg := quickstart.NewAdminConfig(
		cfg.Admin.BasePath,
		cfg.Admin.Title,
		cfg.Admin.DefaultLocale,
	)

	adm, _, err := quickstart.NewAdmin(
		adminCfg,
		quickstart.AdapterHooks{},
		quickstart.WithAdminDependencies(admin.Dependencies{
			FeatureGate: buildFeatureGate(cfg.FeatureDefaults()),
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("build admin: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	auther, routeAuth, authn, demoIdentity, authCookieName, routeAuthConfig, err := setupAuth(adm, cfg)
	if err != nil {
		return nil, fmt.Errorf("setup auth: %w", err)
	}

	isDev := strings.EqualFold(strings.TrimSpace(cfg.Env), "development") ||
		strings.EqualFold(strings.TrimSpace(cfg.Env), "dev") ||
		strings.EqualFold(strings.TrimSpace(cfg.Env), "local")

	viewEngine, err := quickstart.NewViewEngine(
		client.FS(),
		quickstart.WithViewTemplateFuncs(quickstart.DefaultTemplateFuncs(
			quickstart.WithTemplateURLResolver(adm.URLs()),
			quickstart.WithTemplateBasePath(adminCfg.BasePath),
			quickstart.WithTemplateFeatureGate(adm.FeatureGate()),
		)),
		quickstart.WithViewDebug(isDev),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize view engine: %w", err)
	}

	server, r := quickstart.NewFiberServer(
		viewEngine,
		adminCfg,
		adm,
		isDev,
		quickstart.WithFiberConfig(func(fcfg *fiber.Config) {
			if fcfg != nil {
				fcfg.EnablePrintRoutes = cfg.Server.PrintRoutes
			}
		}),
	)

	if err := adm.Initialize(r); err != nil {
		return nil, fmt.Errorf("initialize admin routes: %w", err)
	}
	quickstart.NewStaticAssets(r, adminCfg, client.Assets())

	if err := quickstart.RegisterAuthUIRoutes(
		r,
		adminCfg,
		routeAuth,
		quickstart.WithAuthUIFeatureGate(adm.FeatureGate()),
	); err != nil {
		return nil, fmt.Errorf("register auth UI routes: %w", err)
	}

	if err := quickstart.RegisterAdminUIRoutes(r, adminCfg, adm, authn); err != nil {
		return nil, fmt.Errorf("register admin UI routes: %w", err)
	}

	searchRuntime, err := searchdemo.New(searchdemo.Config{
		Provider:                  cfg.SearchDemo.Provider,
		CacheEnabled:              cfg.SearchDemo.CacheEnabled,
		SeedOnStart:               cfg.SearchDemo.SeedOnStart,
		IndexName:                 cfg.SearchDemo.IndexName,
		DefaultLocale:             cfg.SearchDemo.DefaultLocale,
		CultureDataPath:           cfg.SearchDemo.CultureDataPath,
		PostgresDSN:               cfg.SearchDemo.PostgresDSN,
		TypesenseServerURL:        cfg.SearchDemo.TypesenseServerURL,
		TypesenseAPIKey:           cfg.SearchDemo.TypesenseAPIKey,
		TypesenseCollectionPrefix: cfg.SearchDemo.TypesenseCollectionPrefix,
		ReindexBatchSize:          cfg.SearchDemo.ReindexBatchSize,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize search demo runtime: %w", err)
	}

	if searchService := adm.SearchService(); searchService != nil {
		sharedIndexes := searchRuntime.SurfaceIndexes(searchdemo.SurfaceContentShared)
		searchService.SetPrimary(admin.NewGoSearchGlobalAdapter(admin.GoSearchGlobalAdapterConfig{
			Search:       searchRuntime.SearchQuery(),
			Indexes:      sharedIndexes,
			FallbackType: "content",
		}))
	}

	sharedIndexes := searchRuntime.SurfaceIndexes(searchdemo.SurfaceContentShared)
	siteSearchProvider := admin.NewGoSearchSiteProvider(admin.GoSearchSiteProviderConfig{
		Search:  searchRuntime.SearchQuery(),
		Suggest: searchRuntime.SuggestQuery(),
		Indexes: sharedIndexes,
	})

	searchOperations := &admin.GoSearchOperations{
		Health:      searchRuntime.HealthQuery(),
		Stats:       searchRuntime.StatsQuery(),
		EnsureIndex: searchRuntime.EnsureCommand(),
		Reindex:     searchRuntime.ReindexCommand(),
		Indexes:     searchRuntime.IndexNames(),
	}

	return &Core{
		Config:             cfg,
		Logger:             logger,
		StartedAt:          time.Now().UTC(),
		Search:             searchRuntime,
		AdminConfig:        adminCfg,
		Server:             server,
		Router:             r,
		Fiber:              server.WrappedRouter(),
		Admin:              adm,
		SiteSearchProvider: siteSearchProvider,
		SearchOperations:   searchOperations,
		Authenticator:      authn,
		AuthCookieName:     authCookieName,
		Auther:             auther,
		RouteAuthenticator: routeAuth,
		DemoIdentity:       demoIdentity,
		routeAuthConfig:    routeAuthConfig,
	}, nil
}

func buildFeatureGate(defaults map[string]bool) fggate.FeatureGate {
	return resolver.New(
		resolver.WithDefaults(configadapter.NewDefaultsFromBools(defaults)),
	)
}

func (c *Core) Serve() error {
	if c == nil || c.Server == nil || c.Config == nil {
		return fmt.Errorf("core server is not configured")
	}
	return c.Server.Serve(c.Config.Server.Address)
}

func (c *Core) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var shutdownErr error
	if c != nil && c.Server != nil {
		shutdownErr = c.Server.Shutdown(ctx)
	}
	if c != nil && c.Search != nil {
		if err := c.Search.Close(); err != nil && shutdownErr == nil {
			shutdownErr = err
		}
	}
	return shutdownErr
}

func (c *Core) Features() []FeatureStatus {
	if c == nil || c.Config == nil {
		return nil
	}
	defaults := c.Config.FeatureDefaults()
	out := make([]FeatureStatus, 0, len(defaults))
	for key, value := range defaults {
		out = append(out, FeatureStatus{Name: key, Enabled: value})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
