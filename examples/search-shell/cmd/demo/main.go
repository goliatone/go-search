package main

import (
	"context"
	"os"
	"path"
	"strings"

	"github.com/goliatone/go-search/examples/search-shell/internal/config"
	"github.com/goliatone/go-search/examples/search-shell/internal/core"
	apphttp "github.com/goliatone/go-search/examples/search-shell/internal/http"
	"github.com/goliatone/go-search/pkg/types"
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
	defer func() {
		_ = appCore.Shutdown(context.Background())
	}()

	if err := apphttp.Register(appCore); err != nil {
		panic(err)
	}

	baseURL := normalizeAddress(cfg.Server.Address)
	status, err := appCore.Search.Status(context.Background())
	if err != nil {
		panic(err)
	}

	appCore.Logger.Info("search shell ready",
		"address", cfg.Server.Address,
		"base_url", baseURL,
		"home", joinURL(baseURL, "/"),
		"admin", joinURL(baseURL, cfg.Admin.BasePath),
		"demo", joinURL(baseURL, "/demo/search"),
		"config", cfg.ConfigPath,
		"provider", appCore.Search.ProviderName(),
		"default_locale", status.DefaultLocale,
		"documents", status.Documents,
		"generation", status.Generation,
		"editorial_rules", status.EditorialRules,
		"indexes", appCore.Search.IndexNames(),
		"surfaces", []string{"content_shared", "content_split", "media_grouped", "users"},
		"capabilities", enabledCapabilities(status.Capabilities),
	)
	if limitations := capabilityLimitations(status.Capabilities); len(limitations) > 0 {
		appCore.Logger.Info("provider capability limitations",
			"provider", appCore.Search.ProviderName(),
			"limitations", limitations,
		)
	}

	appCore.Logger.Info("demo pages",
		"routes", []string{
			"GET " + joinURL(baseURL, "/"),
			"GET " + joinURL(baseURL, "/demo/search?surface=content_shared&q=search"),
			"GET " + joinURL(baseURL, "/demo/search?surface=content_split&q=search"),
			"GET " + joinURL(baseURL, "/demo/search?surface=media_grouped&group=true&q=transcript"),
			"GET " + joinURL(baseURL, "/demo/search?surface=users&q=admin"),
			"GET " + joinURL(baseURL, "/demo/topics/architecture"),
			"GET " + joinURL(baseURL, "/demo/ops"),
		},
	)
	appCore.Logger.Info("demo read APIs",
		"routes", append([]string{
			"GET " + joinURL(baseURL, "/healthz"),
			"GET " + joinURL(baseURL, "/readyz"),
			"GET " + joinURL(baseURL, "/api/demo/health"),
			"GET " + joinURL(baseURL, "/api/demo/stats"),
			"GET " + joinURL(baseURL, "/api/demo/search?surface=content_shared&q=search"),
			"GET " + joinURL(baseURL, "/api/demo/suggest?surface=content_shared&q=sea&limit=5"),
		}, editorialReadRoutes(cfg, baseURL)...),
	)
	appCore.Logger.Info("demo mutation APIs",
		"routes", append([]string{
			"POST " + joinURL(baseURL, "/api/demo/ensure"),
			"POST " + joinURL(baseURL, "/api/demo/reindex"),
			"POST " + joinURL(baseURL, "/api/demo/users/create"),
			"POST " + joinURL(baseURL, "/api/demo/users/update"),
			"POST " + joinURL(baseURL, "/api/demo/users/lifecycle"),
			"POST " + joinURL(baseURL, "/api/demo/users/profile"),
			"POST " + joinURL(baseURL, "/api/demo/users/role"),
		}, editorialMutationRoutes(cfg, baseURL)...),
	)
	appCore.Logger.Info("site and admin integrations",
		"routes", []string{
			"GET " + joinURL(baseURL, path.Join(cfg.Admin.BasePath, "login")),
			"GET " + joinURL(baseURL, cfg.Admin.BasePath),
			"GET " + joinURL(baseURL, path.Join(cfg.Admin.BasePath, "api/search")) + "?query=search&limit=5",
			"GET " + joinURL(baseURL, "/site/search?q=search&locale=en"),
			"GET " + joinURL(baseURL, "/api/v1/site/search?q=search&locale=en"),
			"GET " + joinURL(baseURL, "/api/v1/site/search/suggest?q=sea&locale=en"),
		},
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

func enabledCapabilities(set types.CapabilitySet) []string {
	out := make([]string, 0, 16)
	if set.Facets {
		out = append(out, "facets")
	}
	if set.HierarchicalFacets {
		out = append(out, "hierarchical_facets")
	}
	if set.RangeFacets {
		out = append(out, "range_facets")
	}
	if set.DisjunctiveFacets {
		out = append(out, "disjunctive_facets")
	}
	if set.PrefixSearch {
		out = append(out, "prefix_search")
	}
	if set.TypoTolerance {
		out = append(out, "typo_tolerance")
	}
	if set.Highlighting {
		out = append(out, "highlighting")
	}
	if set.Snippets {
		out = append(out, "snippets")
	}
	if set.Grouping {
		out = append(out, "grouping")
	}
	if set.SemanticSearch {
		out = append(out, "semantic_search")
	}
	if set.HybridSearch {
		out = append(out, "hybrid_search")
	}
	if set.AutoEmbedding {
		out = append(out, "auto_embedding")
	}
	if set.ExternalEmbeddings {
		out = append(out, "external_embeddings")
	}
	if set.DistanceThreshold {
		out = append(out, "distance_threshold")
	}
	if set.MultilingualEmbeds {
		out = append(out, "multilingual_embeds")
	}
	for _, mode := range set.SupportedSearchModes {
		if value := strings.TrimSpace(string(mode)); value != "" {
			out = append(out, "mode:"+value)
		}
	}
	return out
}

func capabilityLimitations(set types.CapabilitySet) []string {
	if len(set.Limitations) == 0 {
		return nil
	}
	out := make([]string, 0, len(set.Limitations))
	for _, limitation := range set.Limitations {
		capability := strings.TrimSpace(limitation.Capability)
		message := strings.TrimSpace(limitation.Message)
		switch {
		case capability == "":
			out = append(out, message)
		case message == "":
			out = append(out, capability)
		default:
			out = append(out, capability+": "+message)
		}
	}
	return out
}

func editorialReadRoutes(cfg config.AppConfig, baseURL string) []string {
	if !cfg.SearchDemo.EditorialEnabled {
		return nil
	}
	return []string{
		"GET " + joinURL(baseURL, "/api/demo/editorial"),
	}
}

func editorialMutationRoutes(cfg config.AppConfig, baseURL string) []string {
	if !cfg.SearchDemo.EditorialEnabled {
		return nil
	}
	return []string{
		"POST " + joinURL(baseURL, "/api/demo/editorial/upsert"),
		"POST " + joinURL(baseURL, "/api/demo/editorial/enable"),
		"POST " + joinURL(baseURL, "/api/demo/editorial/disable"),
		"POST " + joinURL(baseURL, "/api/demo/editorial/delete"),
	}
}
