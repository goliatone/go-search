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
	Config    *config.AppConfig   `json:"config"`
	Logger    *slog.Logger        `json:"logger"`
	StartedAt time.Time           `json:"started_at"`
	Search    *searchdemo.Runtime `json:"search"`
	Server    router.Server[*fiber.App]
	Router    router.Router[*fiber.App]
	Fiber     *fiber.App

	Admin              *admin.Admin
	Authenticator      *admin.GoAuthAuthenticator
	AuthCookieName     string
	Auther             *auth.Auther
	RouteAuthenticator *auth.RouteAuthenticator
	DemoCredentials    []DemoCredential
	DemoIdentity       DemoIdentity
	DemoToken          string
}

func New(ctx context.Context, cfg *config.AppConfig) (*Core, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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

	auther, routeAuth, authn, demoCredentials, demoIdentity, demoToken, authCookieName, err := setupAuth(adm, cfg, logger)
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
		auther,
		authCookieName,
		quickstart.WithAuthUIFeatureGate(adm.FeatureGate()),
	); err != nil {
		return nil, fmt.Errorf("register auth UI routes: %w", err)
	}

	if err := quickstart.RegisterAdminUIRoutes(r, adminCfg, adm, authn); err != nil {
		return nil, fmt.Errorf("register admin UI routes: %w", err)
	}

	searchRuntime, err := searchdemo.New(searchdemo.Config{
		Provider:        cfg.SearchDemo.Provider,
		SeedOnStart:     cfg.SearchDemo.SeedOnStart,
		IndexName:       cfg.SearchDemo.IndexName,
		DefaultLocale:   cfg.SearchDemo.DefaultLocale,
		CultureDataPath: cfg.SearchDemo.CultureDataPath,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize search demo runtime: %w", err)
	}

	return &Core{
		Config:             cfg,
		Logger:             logger,
		StartedAt:          time.Now().UTC(),
		Search:             searchRuntime,
		Server:             server,
		Router:             r,
		Fiber:              server.WrappedRouter(),
		Admin:              adm,
		Authenticator:      authn,
		AuthCookieName:     authCookieName,
		Auther:             auther,
		RouteAuthenticator: routeAuth,
		DemoCredentials:    demoCredentials,
		DemoIdentity:       demoIdentity,
		DemoToken:          demoToken,
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
	if c == nil || c.Server == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.Server.Shutdown(ctx)
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
