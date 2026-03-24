package planner

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers/memory"
)

type stubLocaleRuntime struct {
	matches map[string]string
}

type capabilitySourceStub struct {
	caps types.CapabilitySet
	err  error
}

func (s capabilitySourceStub) Capabilities(context.Context) (types.CapabilitySet, error) {
	return s.caps, s.err
}

func (s stubLocaleRuntime) Normalize(value string) string {
	return locale.Normalize(value)
}

func (s stubLocaleRuntime) NormalizeMany(values []string) []string {
	return locale.NormalizeMany(values)
}

func (s stubLocaleRuntime) NormalizeAndSort(values []string) []string {
	return locale.NormalizeAndSort(values)
}

func (s stubLocaleRuntime) Match(value string) (string, bool) {
	matched, ok := s.matches[locale.Normalize(value)]
	return matched, ok
}

func (s stubLocaleRuntime) MatchAcceptLanguage(string) (string, bool) {
	return "", false
}

func (s stubLocaleRuntime) MatchAcceptLanguageWithOptions(string, locale.MatchOptions) (string, bool) {
	return "", false
}

func (s stubLocaleRuntime) Resolve(value string, _ locale.ResolveOptions) locale.Resolution {
	canonical := locale.Normalize(value)
	if matched, ok := s.matches[canonical]; ok {
		return locale.Resolution{
			Requested: value,
			Canonical: canonical,
			Matched:   matched,
			Chain:     []string{matched},
		}
	}
	return locale.Resolution{Requested: value, Canonical: canonical, Chain: []string{canonical}}
}

func (s stubLocaleRuntime) DecodeMetadata(string, any) error {
	return nil
}

func TestPlannerNormalizesAndValidates(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}, nil)
	p, err := New(Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	req := types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "  prayer ",
		Page:    0,
		PerPage: 0,
		Sort:    []types.Sort{{Field: "title", Direction: types.SortAsc}},
		Filters: types.AndExpr{Terms: []types.FilterExpr{types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: "archive"}}},
	}
	plan, err := p.BuildSearchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if plan.Request.Page != 1 || plan.Request.PerPage != 20 {
		t.Fatalf("expected default pagination, got page=%d perPage=%d", plan.Request.Page, plan.Request.PerPage)
	}
	if plan.Request.GroupBy != "parent_id" {
		t.Fatalf("expected default group by to be applied")
	}
}

func TestPlannerRejectsInvalidFilter(t *testing.T) {
	err := ValidateFilter(types.TermExpr{Field: "", Op: types.FilterOpEQ, Value: "x"})
	if err == nil {
		t.Fatalf("expected invalid filter error")
	}
}

func TestPlannerRejectsUnsupportedFilterOperator(t *testing.T) {
	err := ValidateFilter(types.TermExpr{Field: "topic", Op: types.FilterOp("wildcard"), Value: "archive"})
	if err == nil {
		t.Fatalf("expected invalid filter operator error")
	}
}

func TestPlannerRejectsInvalidFilterInPayload(t *testing.T) {
	err := ValidateFilter(types.TermExpr{Field: "topic", Op: types.FilterOpIn, Value: "archive"})
	if err == nil {
		t.Fatalf("expected invalid in payload error")
	}
}

func TestPlannerRejectsUnsupportedSearchMode(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	p, err := New(Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	err = p.ValidateSearchCapabilities(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Mode:    types.SearchModeSemantic,
		Semantic: &types.SemanticRequest{
			Field: "body",
		},
	}, memory.New(memory.Config{}))
	if err == nil {
		t.Fatalf("expected unsupported capability error")
	}
}

func TestPlannerRejectsUnsupportedHierarchicalAndDisjunctiveFacets(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	p, err := New(Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	source := capabilitySourceStub{caps: types.CapabilitySet{Facets: true}}
	err = p.ValidateSearchCapabilities(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Facets: []types.FacetRequest{
			{Field: "topic_hierarchy", Kind: types.FacetKindHierarchical},
			{Field: "topic", Disjunctive: true},
		},
	}, source)
	if err == nil {
		t.Fatalf("expected unsupported capability error")
	}
}

func TestPlannerBuildsLocalePlanAndProviderRequest(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	p, err := New(Config{
		Registry:      registry,
		LocaleRuntime: stubLocaleRuntime{matches: map[string]string{"es-MX": "es"}},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	plan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  " ES_mx ",
		Locales: []string{"BO", "es"},
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if plan.Request.Locale != "es-MX" {
		t.Fatalf("normalized request locale = %q", plan.Request.Locale)
	}
	if !reflect.DeepEqual(plan.Request.Locales, []string{"bo", "es"}) {
		t.Fatalf("normalized request locales = %#v", plan.Request.Locales)
	}
	if plan.Locale.Canonical != "es-MX" || plan.Locale.Matched != "es" || plan.Locale.Primary != "es" {
		t.Fatalf("unexpected locale plan = %+v", plan.Locale)
	}
	if !reflect.DeepEqual(plan.Locale.Fallbacks, []string{"bo"}) {
		t.Fatalf("fallbacks = %#v", plan.Locale.Fallbacks)
	}
	if !reflect.DeepEqual(plan.Locale.Chain, []string{"es", "bo"}) {
		t.Fatalf("chain = %#v", plan.Locale.Chain)
	}
	providerReq := plan.ProviderRequest()
	if providerReq.Locale != "es" {
		t.Fatalf("provider locale = %q", providerReq.Locale)
	}
	if !reflect.DeepEqual(providerReq.Locales, []string{"bo"}) {
		t.Fatalf("provider locales = %#v", providerReq.Locales)
	}
}

func TestPlannerTracksExplicitLocalesWithoutPrimary(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	p, err := New(Config{
		Registry:      registry,
		LocaleRuntime: stubLocaleRuntime{},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	plan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locales: []string{"BO", "en_US", "bo"},
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if plan.Locale.Primary != "" {
		t.Fatalf("expected no primary locale, got %q", plan.Locale.Primary)
	}
	if !reflect.DeepEqual(plan.Locale.RequestedLocales, []string{"bo", "en-US"}) {
		t.Fatalf("requested locales = %#v", plan.Locale.RequestedLocales)
	}
	if !reflect.DeepEqual(plan.Locale.Chain, []string{"bo", "en-US"}) {
		t.Fatalf("chain = %#v", plan.Locale.Chain)
	}
	providerReq := plan.ProviderRequest()
	if providerReq.Locale != "" {
		t.Fatalf("provider locale = %q", providerReq.Locale)
	}
	if !reflect.DeepEqual(providerReq.Locales, []string{"bo", "en-US"}) {
		t.Fatalf("provider locales = %#v", providerReq.Locales)
	}
}

func TestPlannerUsesConfiguredDefaults(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}, nil)
	p, err := New(Config{
		Registry: registry,
		Defaults: Defaults{
			SearchPage:                 2,
			SearchPerPage:              15,
			SuggestLimit:               7,
			DefaultSearchMode:          types.SearchModeHybrid,
			DisableIndexGroupByDefault: true,
		},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	searchPlan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if searchPlan.Request.Page != 2 || searchPlan.Request.PerPage != 15 {
		t.Fatalf("expected configured pagination defaults, got %+v", searchPlan.Request)
	}
	if searchPlan.Request.Mode != types.SearchModeHybrid {
		t.Fatalf("expected configured default mode, got %s", searchPlan.Request.Mode)
	}
	if searchPlan.Request.GroupBy != "" {
		t.Fatalf("expected index group default to be disabled, got %q", searchPlan.Request.GroupBy)
	}
	suggestPlan, err := p.BuildSuggestPlan(context.Background(), types.SuggestRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
	})
	if err != nil {
		t.Fatalf("build suggest plan: %v", err)
	}
	if suggestPlan.Request.Limit != 7 {
		t.Fatalf("expected configured suggest limit, got %d", suggestPlan.Request.Limit)
	}
}

func TestPlannerLeavesMixedIndexesFlatByDefault(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}, nil)
	_ = registry.Register(types.IndexDefinition{Name: "documents"}, nil)
	p, err := New(Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	plan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media", "documents"},
		Query:   "architecture",
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if plan.Request.GroupBy != "" {
		t.Fatalf("expected mixed-index request to stay flat, got %q", plan.Request.GroupBy)
	}
}

func TestPlannerRejectsGroupedSearchAcrossIncompatibleIndexes(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}, nil)
	_ = registry.Register(types.IndexDefinition{Name: "documents"}, nil)
	p, err := New(Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	if _, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media", "documents"},
		Query:   "architecture",
		GroupBy: "parent_id",
	}); err == nil {
		t.Fatalf("expected grouped mixed-index request to fail")
	}
}

func TestPlannerProducesCanonicalLocalesForProvidersAndCacheKeys(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	p, err := New(Config{
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	plan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  " EN_us ",
		Locales: []string{" ES_mx ", "bo", "es_mx"},
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}

	providerReq := plan.ProviderRequest()
	if providerReq.Locale != "en-US" {
		t.Fatalf("provider locale = %q", providerReq.Locale)
	}
	if !reflect.DeepEqual(providerReq.Locales, []string{"es-MX", "bo"}) {
		t.Fatalf("provider locales = %#v", providerReq.Locales)
	}

	cacheKeyLocales := locale.NormalizeAndSort(append([]string{providerReq.Locale}, providerReq.Locales...))
	if !reflect.DeepEqual(cacheKeyLocales, []string{"bo", "en-US", "es-MX"}) {
		t.Fatalf("cache key locales = %#v", cacheKeyLocales)
	}
}

func TestPlannerResolvesLocaleChainAndAppliesTypedMetadata(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	runtime := mustBuildI18nRuntime(t)
	p, err := New(Config{
		Registry:      registry,
		LocaleRuntime: runtime,
		LocalePolicy: LocalePolicy{
			MatchStrategy:   locale.MatchExactOrParent,
			Scope:           locale.ScopeActiveOnly,
			ExpandParents:   true,
			ExpandFallbacks: true,
			IncludeDefault:  true,
		},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	plan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "es-MX",
		Semantic: &types.SemanticRequest{
			Field: "body",
		},
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}

	if plan.Locale.Primary != "es-419" {
		t.Fatalf("primary locale = %q", plan.Locale.Primary)
	}
	if !reflect.DeepEqual(plan.Locale.Fallbacks, []string{"es", "en"}) {
		t.Fatalf("fallbacks = %#v", plan.Locale.Fallbacks)
	}
	if !reflect.DeepEqual(plan.Locale.Chain, []string{"es-419", "es", "en"}) {
		t.Fatalf("chain = %#v", plan.Locale.Chain)
	}
	if !reflect.DeepEqual(plan.Locale.Origins, map[string]string{
		"es-419": "matched",
		"es":     "parent",
		"en":     "fallback",
	}) {
		t.Fatalf("origins = %#v", plan.Locale.Origins)
	}
	if plan.Locale.Metadata.Analyzer != "spanish" {
		t.Fatalf("analyzer = %q", plan.Locale.Metadata.Analyzer)
	}
	if plan.Request.Metadata["locale_analyzer"] != "spanish" {
		t.Fatalf("request locale analyzer = %#v", plan.Request.Metadata["locale_analyzer"])
	}
	if plan.Request.Metadata["locale_semantic_model"] != "semantic-es-419" {
		t.Fatalf("request locale semantic model = %#v", plan.Request.Metadata["locale_semantic_model"])
	}
	if plan.Request.Semantic == nil || plan.Request.Semantic.Model != "semantic-es-419" {
		t.Fatalf("semantic request = %+v", plan.Request.Semantic)
	}

	providerReq := plan.ProviderRequest()
	if providerReq.Locale != "es-419" {
		t.Fatalf("provider locale = %q", providerReq.Locale)
	}
	if !reflect.DeepEqual(providerReq.Locales, []string{"es", "en"}) {
		t.Fatalf("provider locales = %#v", providerReq.Locales)
	}
}

func TestPlannerPrefersEnabledFallbackWhenMatchedLocaleIsDisabled(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	runtime := mustBuildI18nRuntime(t)
	p, err := New(Config{
		Registry:      registry,
		LocaleRuntime: runtime,
		LocalePolicy: LocalePolicy{
			MatchStrategy:   locale.MatchExactOrParent,
			Scope:           locale.ScopeAll,
			ExpandFallbacks: true,
			IncludeDefault:  true,
		},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}

	plan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "fr-CA",
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}

	if plan.Locale.Matched != "fr" {
		t.Fatalf("matched locale = %q", plan.Locale.Matched)
	}
	if plan.Locale.Primary != "en" {
		t.Fatalf("primary locale = %q", plan.Locale.Primary)
	}
	if len(plan.Locale.Fallbacks) != 0 {
		t.Fatalf("fallbacks = %#v", plan.Locale.Fallbacks)
	}
	if plan.Locale.Metadata.Analyzer != "english" {
		t.Fatalf("analyzer = %q", plan.Locale.Metadata.Analyzer)
	}
	if plan.Request.Metadata["locale_search_enabled"] != true {
		t.Fatalf("locale_search_enabled = %#v", plan.Request.Metadata["locale_search_enabled"])
	}
	if origins := plan.Request.Metadata["locale_origins"]; !reflect.DeepEqual(origins, map[string]any{
		"fr": "matched",
		"en": "fallback",
	}) {
		t.Fatalf("locale_origins = %#v", origins)
	}
}

func mustBuildI18nRuntime(t *testing.T) *locale.I18nRuntime {
	t.Helper()

	runtime, err := locale.NewI18nRuntimeFromCultureData(filepath.Join("..", "testdata", "locale_search_culture.json"), "en")
	if err != nil {
		t.Fatalf("NewI18nRuntimeFromCultureData: %v", err)
	}
	return runtime
}
