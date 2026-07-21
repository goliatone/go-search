package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/examples/search-shell/internal/config"
	"github.com/goliatone/go-search/examples/search-shell/internal/core"
	apphttp "github.com/goliatone/go-search/examples/search-shell/internal/http"
	"github.com/goliatone/go-search/examples/search-shell/internal/searchdemo"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
)

type SearchV1ValidationReport struct {
	Provider     string   `json:"provider"`
	CacheEnabled bool     `json:"cache_enabled"`
	Documents    int      `json:"documents"`
	Generation   int64    `json:"generation"`
	Checks       []string `json:"checks"`
}

type SearchV1RouteValidationReport struct {
	EditorialEnabled bool     `json:"editorial_enabled"`
	Checks           []string `json:"checks"`
}

const routeValidationRequestTimeoutMillis = 10_000

func RunSearchV1RuntimeValidationProfile(ctx context.Context, cfg searchdemo.Config) (SearchV1ValidationReport, error) {
	runtime, err := searchdemo.New(cfg)
	if err != nil {
		return SearchV1ValidationReport{}, err
	}
	defer func() {
		_ = runtime.Close()
	}()

	status, err := runtime.Status(ctx)
	if err != nil {
		return SearchV1ValidationReport{}, err
	}

	report := SearchV1ValidationReport{
		Provider:     status.Provider,
		CacheEnabled: status.CacheEnabled,
		Documents:    status.Documents,
		Generation:   status.Generation,
	}
	if status.Documents <= 45 {
		return report, fmt.Errorf("expected seeded document count above 45, got %d", status.Documents)
	}
	if len(status.SmokeFlows) < 4 {
		return report, fmt.Errorf("expected smoke flows in status, got %d", len(status.SmokeFlows))
	}
	report.Checks = append(report.Checks, "status")

	grouped, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:       "search",
		Surface:     searchdemo.SurfaceMediaGrouped,
		Group:       true,
		Locale:      "en",
		LandingSlug: "architecture",
		PerPage:     10,
	})
	if err != nil {
		return report, err
	}
	if len(grouped.Groups) == 0 || grouped.Groups[0].Parent == nil {
		return report, fmt.Errorf("expected grouped archive results with parent metadata")
	}
	if len(grouped.Groups[0].Hits) == 0 || grouped.Groups[0].Hits[0].Anchor == nil || strings.TrimSpace(grouped.Groups[0].Hits[0].Anchor.URL) == "" {
		return report, fmt.Errorf("expected grouped transcript hit with playback anchor")
	}
	if !hasHierarchicalFacet(grouped.Facets, media.FacetFieldTopicHierarchy) {
		return report, fmt.Errorf("expected hierarchical topic facet in grouped archive result")
	}
	report.Checks = append(report.Checks, "grouped_archive")

	flat, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:     "search",
		Surface:   searchdemo.SurfaceContentShared,
		Locale:    "en",
		SortField: "title",
		SortDir:   "asc",
		PerPage:   20,
	})
	if err != nil {
		return report, err
	}
	if !containsHitTypes(flat, types.DocumentTypeVideo, types.DocumentTypeDocument, types.DocumentTypeBlogArticle) {
		return report, fmt.Errorf("expected flat heterogeneous content hits, got %+v", flat.Hits)
	}
	if !hasSnippetAndBadge(flat.Hits) {
		return report, fmt.Errorf("expected highlighted snippet and badge metadata in flat search results")
	}
	report.Checks = append(report.Checks, "heterogeneous_flat")

	exactLocale, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:   "locale",
		Surface: searchdemo.SurfaceContentShared,
		Locale:  "es",
		PerPage: 10,
	})
	if err != nil {
		return report, err
	}
	if len(exactLocale.Hits) == 0 {
		return report, fmt.Errorf("expected exact-locale results for es query")
	}
	if exactLocale.Hits[0].Locale != "es" || !isExactLocaleOrigin(localeOrigin(exactLocale.Hits[0])) {
		return report, fmt.Errorf("expected exact locale result first, got locale=%q origin=%q", exactLocale.Hits[0].Locale, localeOrigin(exactLocale.Hits[0]))
	}

	fallbackLocale, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:   "transcript",
		Locale:  "es-MX",
		PerPage: 10,
	})
	if err != nil {
		return report, err
	}
	if len(fallbackLocale.Hits) == 0 || localeOrigin(fallbackLocale.Hits[0]) != "fallback" {
		return report, fmt.Errorf("expected fallback locale result for es-MX query, got %+v", fallbackLocale.Hits)
	}

	bound := runtime.BindSearchRequest(searchdemo.SearchRequest{
		Query:          "locale",
		AcceptLanguage: "fr-CA,fr;q=0.9,en;q=0.8",
	})
	if bound.Locale != "en" || bound.LocaleSource != "accept_language" || !bound.LocaleSupported {
		return report, fmt.Errorf("unexpected Accept-Language binding %+v", bound)
	}
	report.Checks = append(report.Checks, "locale_policy")

	supportSearch, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:       "support",
		Surface:     searchdemo.SurfaceUsers,
		ActorRole:   "support",
		ActorUserID: "00000000-0000-0000-0000-000000001002",
		TenantID:    "00000000-0000-0000-0000-000000000101",
		OrgID:       "00000000-0000-0000-0000-000000000201",
	})
	if err != nil {
		return report, err
	}
	if supportSearch.Total != 1 || len(supportSearch.Hits) != 1 || supportSearch.Hits[0].ID != "00000000-0000-0000-0000-000000001002" {
		return report, fmt.Errorf("expected support actor to see only self in user search, got %+v", supportSearch.Hits)
	}
	supportSuggest, err := runtime.Suggest(ctx, searchdemo.SuggestRequest{
		Query:       "support",
		Surface:     searchdemo.SurfaceUsers,
		ActorRole:   "support",
		ActorUserID: "00000000-0000-0000-0000-000000001002",
		TenantID:    "00000000-0000-0000-0000-000000000101",
		OrgID:       "00000000-0000-0000-0000-000000000201",
		Limit:       5,
	})
	if err != nil {
		return report, err
	}
	if len(supportSuggest.Items) != 1 || supportSuggest.Items[0].ID != "00000000-0000-0000-0000-000000001002" {
		return report, fmt.Errorf("expected support actor to see only self in suggest, got %+v", supportSuggest.Items)
	}
	report.Checks = append(report.Checks, "scope_and_permissions")

	baselinePinned, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:   "search",
		Surface: searchdemo.SurfaceMediaGrouped,
		Group:   false,
		Locale:  "en",
		PerPage: 20,
	})
	if err != nil {
		return report, err
	}
	baselinePinnedPos := hitPositionByParent(baselinePinned.Hits, "media-1")
	if baselinePinnedPos < 0 {
		return report, fmt.Errorf("expected media-1 in baseline ungrouped search results")
	}

	pinPosition := 0
	pinRuleID := fmt.Sprintf("release-pin-%d", time.Now().UnixNano())
	if err := runtime.UpsertEditorialRule(ctx, types.EditorialRankRule{
		ID:             pinRuleID,
		TargetType:     types.DocumentTypeTranscriptSegment,
		ParentTargetID: "media-1",
		Action:         types.EditorialActionPin,
		Position:       &pinPosition,
		Enabled:        true,
		Scope: types.EditorialScope{
			Indexes: []string{runtime.IndexNames()[0]},
			Query:   "search",
			Locale:  "en",
		},
		Reason: "release validation pin rule",
	}); err != nil {
		return report, err
	}
	defer func() {
		_ = runtime.DeleteEditorialRule(ctx, pinRuleID)
	}()
	pinRules, err := runtime.ListEditorialRules(ctx, nil)
	if err != nil {
		return report, err
	}
	if !containsRule(pinRules, pinRuleID, types.EditorialActionPin, "media-1") {
		return report, fmt.Errorf("expected pin rule to be stored in editorial registry")
	}

	pinned, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:   "search",
		Surface: searchdemo.SurfaceMediaGrouped,
		Group:   false,
		Locale:  "en",
		PerPage: 20,
	})
	if err != nil {
		return report, err
	}
	pinnedPos := hitPositionByParent(pinned.Hits, "media-1")
	if pinnedPos < 0 {
		return report, fmt.Errorf("expected pin rule result to keep media-1 in ungrouped results")
	}
	if pinnedPos > baselinePinnedPos {
		return report, fmt.Errorf("expected pin rule not to demote media-1: baseline=%d pinned=%d", baselinePinnedPos, pinnedPos)
	}
	if rankingRuleCount(pinned.Metadata) < 1 {
		return report, fmt.Errorf("expected pin query to report at least one applicable editorial rule")
	}

	hideRuleID := fmt.Sprintf("release-hide-%d", time.Now().UnixNano())
	if err := runtime.UpsertEditorialRule(ctx, types.EditorialRankRule{
		ID:             hideRuleID,
		TargetType:     types.DocumentTypeTranscriptSegment,
		ParentTargetID: "media-3",
		Action:         types.EditorialActionHide,
		Enabled:        true,
		Scope: types.EditorialScope{
			Indexes: []string{runtime.IndexNames()[0]},
			Query:   "editorial",
			Locale:  "en",
		},
		Reason: "release validation hide rule",
	}); err != nil {
		return report, err
	}
	defer func() {
		_ = runtime.DeleteEditorialRule(ctx, hideRuleID)
	}()

	hidden, err := runtime.Search(ctx, searchdemo.SearchRequest{
		Query:   "editorial",
		Surface: searchdemo.SurfaceMediaGrouped,
		Group:   true,
		Locale:  "en",
		PerPage: 20,
	})
	if err != nil {
		return report, err
	}
	if containsParent(hidden.Groups, "media-3") {
		return report, fmt.Errorf("expected hide rule to remove media-3 from grouped results")
	}
	hiddenSuggest, err := runtime.Suggest(ctx, searchdemo.SuggestRequest{
		Query:   "editorial",
		Surface: searchdemo.SurfaceMediaGrouped,
		Locale:  "en",
		Limit:   5,
	})
	if err != nil {
		return report, err
	}
	if containsSuggest(hiddenSuggest.Items, "media-3", "Editorial Ranking Rules") {
		return report, fmt.Errorf("expected hide rule to remove editorial suggestion")
	}
	report.Checks = append(report.Checks, "editorial")

	if cfg.CacheEnabled {
		before, err := runtime.Status(ctx)
		if err != nil {
			return report, err
		}
		cacheReq := searchdemo.SearchRequest{Query: "search", Surface: searchdemo.SurfaceContentShared, Locale: "en"}
		if _, err := runtime.Search(ctx, cacheReq); err != nil {
			return report, err
		}
		if _, err := runtime.Search(ctx, cacheReq); err != nil {
			return report, err
		}
		mid, err := runtime.Status(ctx)
		if err != nil {
			return report, err
		}
		if mid.Cache.Search.Hits <= before.Cache.Search.Hits {
			return report, fmt.Errorf("expected cached search hit count to increase, before=%+v after=%+v", before.Cache.Search, mid.Cache.Search)
		}
		if err := runtime.Reindex(ctx, 10); err != nil {
			return report, err
		}
		if _, err := runtime.Search(ctx, cacheReq); err != nil {
			return report, err
		}
		after, err := runtime.Status(ctx)
		if err != nil {
			return report, err
		}
		if after.Generation <= mid.Generation {
			return report, fmt.Errorf("expected generation bump after reindex, before=%d after=%d", mid.Generation, after.Generation)
		}
		if after.Cache.Search.Misses <= mid.Cache.Search.Misses {
			return report, fmt.Errorf("expected cache miss to increase after reindex, before=%+v after=%+v", mid.Cache.Search, after.Cache.Search)
		}
		report.Checks = append(report.Checks, "cache_and_reindex")
	}

	return report, nil
}

func RunSearchV1RouteValidationProfile(ctx context.Context, cfg config.AppConfig) (SearchV1RouteValidationReport, error) {
	appCore, err := core.New(ctx, &cfg)
	if err != nil {
		return SearchV1RouteValidationReport{}, err
	}
	defer func() {
		_ = appCore.Shutdown(context.Background())
	}()
	if err := apphttp.Register(appCore); err != nil {
		return SearchV1RouteValidationReport{}, err
	}

	report := SearchV1RouteValidationReport{
		EditorialEnabled: cfg.SearchDemo.EditorialEnabled,
	}

	healthResp, err := appCore.Fiber.Test(httptest.NewRequest(http.MethodGet, "/api/demo/health", nil), routeValidationRequestTimeoutMillis)
	if err != nil {
		return report, err
	}
	if healthResp.StatusCode != http.StatusOK {
		return report, fmt.Errorf("GET /api/demo/health returned %d", healthResp.StatusCode)
	}
	report.Checks = append(report.Checks, "demo_health")

	searchResp, err := appCore.Fiber.Test(httptest.NewRequest(http.MethodGet, "/api/demo/search?surface=content_shared&locale=en&q=search", nil), routeValidationRequestTimeoutMillis)
	if err != nil {
		return report, err
	}
	if searchResp.StatusCode != http.StatusOK {
		return report, fmt.Errorf("GET /api/demo/search returned %d", searchResp.StatusCode)
	}
	var searchPayload struct {
		Result types.SearchResultPage `json:"result"`
	}
	if err := json.NewDecoder(searchResp.Body).Decode(&searchPayload); err != nil {
		return report, err
	}
	if searchPayload.Result.Total == 0 {
		return report, fmt.Errorf("expected demo API to return results")
	}
	report.Checks = append(report.Checks, "demo_search_api")

	sitePageResp, err := appCore.Fiber.Test(httptest.NewRequest(http.MethodGet, "/site/search?q=search&locale=en", nil), routeValidationRequestTimeoutMillis)
	if err != nil {
		return report, err
	}
	if sitePageResp.StatusCode != http.StatusOK {
		return report, fmt.Errorf("GET /site/search returned %d", sitePageResp.StatusCode)
	}
	report.Checks = append(report.Checks, "site_search_page")

	siteSearchResp, err := appCore.Fiber.Test(httptest.NewRequest(http.MethodGet, "/api/v1/site/search?q=search&locale=en", nil), routeValidationRequestTimeoutMillis)
	if err != nil {
		return report, err
	}
	if siteSearchResp.StatusCode != http.StatusOK {
		return report, fmt.Errorf("GET /api/v1/site/search returned %d", siteSearchResp.StatusCode)
	}
	report.Checks = append(report.Checks, "site_search_api")

	siteSuggestResp, err := appCore.Fiber.Test(httptest.NewRequest(http.MethodGet, "/api/v1/site/search/suggest?q=sea&locale=en", nil), routeValidationRequestTimeoutMillis)
	if err != nil {
		return report, err
	}
	if siteSuggestResp.StatusCode != http.StatusOK {
		return report, fmt.Errorf("GET /api/v1/site/search/suggest returned %d", siteSuggestResp.StatusCode)
	}
	report.Checks = append(report.Checks, "site_suggest_api")

	topicResp, err := appCore.Fiber.Test(httptest.NewRequest(http.MethodGet, "/demo/topics/architecture", nil), routeValidationRequestTimeoutMillis)
	if err != nil {
		return report, err
	}
	if topicResp.StatusCode != http.StatusOK {
		return report, fmt.Errorf("GET /demo/topics/architecture returned %d", topicResp.StatusCode)
	}
	report.Checks = append(report.Checks, "topic_landing")

	editorialReq := httptest.NewRequest(http.MethodGet, "/api/demo/editorial", nil)
	if cfg.SearchDemo.EditorialEnabled {
		token, tokenErr := appCore.Auther.TokenService().Generate(appCore.DemoIdentity, nil)
		if tokenErr != nil {
			return report, tokenErr
		}
		editorialReq.Header.Set("Authorization", "Bearer "+token)
	}
	editorialResp, err := appCore.Fiber.Test(editorialReq, routeValidationRequestTimeoutMillis)
	if err != nil {
		return report, err
	}
	if cfg.SearchDemo.EditorialEnabled && editorialResp.StatusCode != http.StatusOK {
		return report, fmt.Errorf("expected editorial API to be enabled, got %d", editorialResp.StatusCode)
	}
	if !cfg.SearchDemo.EditorialEnabled && editorialResp.StatusCode != http.StatusNotFound {
		return report, fmt.Errorf("expected editorial API to be disabled with 404, got %d", editorialResp.StatusCode)
	}
	report.Checks = append(report.Checks, "editorial_route_toggle")

	return report, nil
}

func SearchV1RuntimeConfigForProvider(provider string, requireExternal bool) (searchdemo.Config, bool, error) {
	cfg := searchdemo.Config{
		Provider:         provider,
		CacheEnabled:     true,
		SeedOnStart:      true,
		DefaultLocale:    "en",
		ReindexBatchSize: 10,
	}

	switch strings.TrimSpace(strings.ToLower(provider)) {
	case "", "memory":
		cfg.Provider = "memory"
		cfg.IndexName = fmt.Sprintf("media_transcripts_release_memory_%d", time.Now().UnixNano())
		return cfg, true, nil
	case "postgres":
		dsn := firstNonEmpty(
			strings.TrimSpace(os.Getenv("APP_SEARCH_DEMO__POSTGRES_DSN")),
			strings.TrimSpace(testkit.Integration.Postgres.DSN),
		)
		if dsn == "" {
			if requireExternal {
				return searchdemo.Config{}, false, fmt.Errorf("postgres release validation requires APP_SEARCH_DEMO__POSTGRES_DSN or testkit.Integration.Postgres.DSN")
			}
			return searchdemo.Config{}, false, nil
		}
		cfg.PostgresDSN = dsn
		cfg.IndexName = fmt.Sprintf("media_transcripts_release_pg_%d", time.Now().UnixNano())
		return cfg, true, nil
	case "typesense":
		serverURL := firstNonEmpty(
			strings.TrimSpace(os.Getenv("APP_SEARCH_DEMO__TYPESENSE_SERVER_URL")),
			strings.TrimSpace(os.Getenv("GO_SEARCH_TEST_TYPESENSE_URL")),
			strings.TrimSpace(testkit.Integration.Typesense.ServerURL),
		)
		apiKey := firstNonEmpty(
			strings.TrimSpace(os.Getenv("APP_SEARCH_DEMO__TYPESENSE_API_KEY")),
			strings.TrimSpace(os.Getenv("GO_SEARCH_TEST_TYPESENSE_API_KEY")),
			strings.TrimSpace(testkit.Integration.Typesense.APIKey),
		)
		if serverURL == "" || apiKey == "" {
			if requireExternal {
				return searchdemo.Config{}, false, fmt.Errorf("typesense release validation requires APP_SEARCH_DEMO__TYPESENSE_SERVER_URL and APP_SEARCH_DEMO__TYPESENSE_API_KEY")
			}
			return searchdemo.Config{}, false, nil
		}
		cfg.TypesenseServerURL = serverURL
		cfg.TypesenseAPIKey = apiKey
		cfg.IndexName = fmt.Sprintf("media_transcripts_release_ts_%d", time.Now().UnixNano())
		cfg.TypesenseCollectionPrefix = fmt.Sprintf("search_shell_release_%d_", time.Now().UnixNano())
		return cfg, true, nil
	default:
		return searchdemo.Config{}, false, fmt.Errorf("unsupported provider %q", provider)
	}
}

func SearchV1RequireExternalProviders() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("GO_SEARCH_RELEASE_REQUIRE_EXTERNAL")), "true")
}

func containsHitTypes(result types.SearchResultPage, want ...string) bool {
	if len(want) == 0 {
		return true
	}
	have := map[string]struct{}{}
	for _, hit := range result.Hits {
		have[strings.TrimSpace(hit.Type)] = struct{}{}
	}
	for _, typ := range want {
		if _, ok := have[strings.TrimSpace(typ)]; !ok {
			return false
		}
	}
	return true
}

func hasSnippetAndBadge(hits []types.SearchHit) bool {
	for _, hit := range hits {
		if hit.Snippet != nil && strings.TrimSpace(hit.Snippet.Highlighted) != "" && hit.Fields != nil && strings.TrimSpace(fmt.Sprint(hit.Fields[media.FieldResultBadge])) != "" {
			return true
		}
	}
	return false
}

func hasHierarchicalFacet(facets []types.SearchFacet, field string) bool {
	for _, facet := range facets {
		if facet.Field == field && facet.Kind == "hierarchical" && facet.Disjunctive && len(facet.Values) > 0 {
			return true
		}
	}
	return false
}

func localeOrigin(hit types.SearchHit) string {
	if hit.Retrieval == nil || hit.Retrieval.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(hit.Retrieval.Metadata["locale_origin"]))
}

func isExactLocaleOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	return origin == "exact" || origin == "matched"
}

func hitPositionByParent(hits []types.SearchHit, parentID string) int {
	for i, hit := range hits {
		if hit.Parent != nil && hit.Parent.ID == parentID {
			return i
		}
	}
	return -1
}

func containsRule(rules []types.EditorialRankRule, id, action, parentID string) bool {
	for _, rule := range rules {
		if strings.EqualFold(strings.TrimSpace(rule.ID), strings.TrimSpace(id)) &&
			strings.EqualFold(strings.TrimSpace(rule.Action), strings.TrimSpace(action)) &&
			strings.EqualFold(strings.TrimSpace(rule.ParentTargetID), strings.TrimSpace(parentID)) {
			return true
		}
	}
	return false
}

func rankingRuleCount(metadata map[string]any) int {
	if metadata == nil {
		return 0
	}
	ranking, ok := metadata["ranking"].(map[string]any)
	if !ok {
		return 0
	}
	switch value := ranking["rule_count"].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func containsParent(groups []types.SearchGroup, parentID string) bool {
	for _, group := range groups {
		if group.Parent != nil && group.Parent.ID == parentID {
			return true
		}
	}
	return false
}

func containsSuggest(items []types.SuggestHit, id, title string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.ID), strings.TrimSpace(id)) {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(item.Title), strings.TrimSpace(title)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
