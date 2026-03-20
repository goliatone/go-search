package main

import (
	"os"
	"strings"

	"github.com/goliatone/go-search/examples/search-shell/internal/config"
	"github.com/goliatone/go-search/examples/search-shell/internal/core"
	apphttp "github.com/goliatone/go-search/examples/search-shell/internal/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	appCore, err := core.New(nil, &cfg)
	if err != nil {
		panic(err)
	}

	if err := apphttp.Register(appCore); err != nil {
		panic(err)
	}

	appCore.Logger.Info("search shell ready",
		"address", cfg.Server.Address,
		"home", joinURL(normalizeAddress(cfg.Server.Address), "/"),
		"admin", joinURL(normalizeAddress(cfg.Server.Address), cfg.Admin.BasePath),
		"demo", joinURL(normalizeAddress(cfg.Server.Address), "/demo/search"),
		"config", cfg.ConfigPath,
		"provider", appCore.Search.ProviderName(),
	)
	for _, credential := range appCore.DemoCredentials {
		appCore.Logger.Info("demo auth credential",
			"username", credential.Username,
			"email", credential.Email,
			"password", credential.Password,
			"role", credential.Role,
		)
	}
	if token := strings.TrimSpace(appCore.DemoToken); token != "" {
		appCore.Logger.Info("demo bearer token", "token", token)
	}

	if err := appCore.Serve(); err != nil {
		appCore.Logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}
	if strings.HasPrefix(address, ":") {
		return "http://localhost" + address
	}
	if address == "" {
		return "http://localhost:8484"
	}
	return "http://" + address
}

func joinURL(base, suffix string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return base
	}
	if strings.HasPrefix(suffix, "/") {
		return base + suffix
	}
	return base + "/" + suffix
}
