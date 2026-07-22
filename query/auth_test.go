package query

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers/memory"
)

type allowListScopeGuard struct {
	allowed map[string]struct{}
}

func (g allowListScopeGuard) AllowSearch(context.Context, types.ActorRef, types.SearchRequest) bool {
	return true
}

func (g allowListScopeGuard) AllowSuggest(context.Context, types.ActorRef, types.SuggestRequest) bool {
	return true
}

func (g allowListScopeGuard) AllowDocument(_ context.Context, _ types.ActorRef, doc types.Document) bool {
	_, ok := g.allowed[doc.ID]
	return ok
}

type providerEnforcedPublicScopeGuard struct{}

var _ types.ProviderEnforcedSearchScopeGuard = providerEnforcedPublicScopeGuard{}

func (providerEnforcedPublicScopeGuard) AllowSearch(context.Context, types.ActorRef, types.SearchRequest) bool {
	return true
}

func (providerEnforcedPublicScopeGuard) AllowSuggest(context.Context, types.ActorRef, types.SuggestRequest) bool {
	return true
}

func (providerEnforcedPublicScopeGuard) AllowDocument(_ context.Context, _ types.ActorRef, doc types.Document) bool {
	return doc.Visibility.Public
}

func (providerEnforcedPublicScopeGuard) SearchAuthorizationEnforcedByProvider(_ context.Context, _ types.ActorRef, req types.SearchRequest) bool {
	return requiresPublicVisibility(req.Filters)
}

func requiresPublicVisibility(expr types.FilterExpr) bool {
	switch value := expr.(type) {
	case types.TermExpr:
		public, ok := value.Value.(bool)
		return value.Field == "visibility_public" && value.Op == types.FilterOpEQ && ok && public
	case types.AndExpr:
		if slices.ContainsFunc(value.Terms, requiresPublicVisibility) {
			return true
		}
	}
	return false
}

type capturingSuggestProvider struct {
	lastSuggest types.SuggestRequest
}

type capturingSearchProvider struct {
	lastSearch types.SearchRequest
	page       types.SearchResultPage
}

type capturingEvidenceProvider struct {
	capturingSearchProvider
	lastGuard types.ScopeGuard
}

func (p *capturingEvidenceProvider) AggregateEvidence(_ context.Context, req types.EvidenceRequest) (map[string]*types.MatchEvidenceSummary, error) {
	p.lastGuard = req.Guard
	out := make(map[string]*types.MatchEvidenceSummary, len(req.ResultIDs))
	for _, id := range req.ResultIDs {
		out[id] = &types.MatchEvidenceSummary{Exact: true, Status: types.EvidenceStatusComplete}
	}
	return out, nil
}

func (p *capturingSuggestProvider) Name() string { return "capture" }

func (p *capturingSuggestProvider) Capabilities(context.Context) (types.CapabilitySet, error) {
	return types.CapabilitySet{}, nil
}

func (p *capturingSuggestProvider) EnsureIndex(context.Context, types.IndexDefinition) error {
	return nil
}

func (p *capturingSuggestProvider) Search(context.Context, types.SearchRequest) (types.SearchResultPage, error) {
	return types.SearchResultPage{}, nil
}

func (p *capturingSuggestProvider) Suggest(_ context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	p.lastSuggest = req
	return types.SuggestResult{}, nil
}

func (p *capturingSuggestProvider) UpsertDocuments(context.Context, string, []types.Document) error {
	return nil
}

func (p *capturingSuggestProvider) ReplaceDocuments(context.Context, string, string, []string, []types.Document) error {
	return nil
}

func (p *capturingSuggestProvider) DeleteDocuments(context.Context, string, []string) error {
	return nil
}

func (p *capturingSuggestProvider) DeleteBySource(context.Context, string, string, []string) error {
	return nil
}

func (p *capturingSuggestProvider) Health(context.Context, types.HealthRequest) (types.HealthStatus, error) {
	return types.HealthStatus{}, nil
}

func (p *capturingSearchProvider) Name() string { return "capture-search" }

func (p *capturingSearchProvider) Capabilities(context.Context) (types.CapabilitySet, error) {
	return types.CapabilitySet{SupportedSearchModes: []types.SearchMode{types.SearchModeLexical}}, nil
}

func (p *capturingSearchProvider) EnsureIndex(context.Context, types.IndexDefinition) error {
	return nil
}

func (p *capturingSearchProvider) Search(_ context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	p.lastSearch = req
	return p.page, nil
}

func (p *capturingSearchProvider) Suggest(context.Context, types.SuggestRequest) (types.SuggestResult, error) {
	return types.SuggestResult{}, nil
}

func (p *capturingSearchProvider) UpsertDocuments(context.Context, string, []types.Document) error {
	return nil
}

func (p *capturingSearchProvider) ReplaceDocuments(context.Context, string, string, []string, []types.Document) error {
	return nil
}

func (p *capturingSearchProvider) DeleteDocuments(context.Context, string, []string) error {
	return nil
}

func (p *capturingSearchProvider) DeleteBySource(context.Context, string, string, []string) error {
	return nil
}

func (p *capturingSearchProvider) Health(context.Context, types.HealthRequest) (types.HealthStatus, error) {
	return types.HealthStatus{}, nil
}

type queryLocaleRuntime struct{}

func (queryLocaleRuntime) Normalize(value string) string { return locale.Normalize(value) }
func (queryLocaleRuntime) NormalizeMany(values []string) []string {
	return locale.NormalizeMany(values)
}
func (queryLocaleRuntime) NormalizeAndSort(values []string) []string {
	return locale.NormalizeAndSort(values)
}
func (queryLocaleRuntime) Match(value string) (string, bool) {
	if locale.Normalize(value) == "es-MX" {
		return "es", true
	}
	return "", false
}
func (queryLocaleRuntime) MatchAcceptLanguage(string) (string, bool) { return "", false }
func (queryLocaleRuntime) MatchAcceptLanguageWithOptions(string, locale.MatchOptions) (string, bool) {
	return "", false
}
func (queryLocaleRuntime) Resolve(value string, _ locale.ResolveOptions) locale.Resolution {
	canonical := locale.Normalize(value)
	if canonical == "es-MX" {
		return locale.Resolution{
			Requested: value,
			Canonical: canonical,
			Matched:   "es",
			Chain:     []string{"es"},
		}
	}
	return locale.Resolution{Requested: value, Canonical: canonical, Chain: []string{canonical}}
}
func (queryLocaleRuntime) DecodeMetadata(string, any) error { return nil }

func TestSearchFiltersUnauthorizedHitsGroupsAndFacets(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id", FacetFields: []string{"topic"}}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:       "segment-allow",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Ocean Wind",
			Body:     "prayer by the sea",
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{"topic": {"archive"}},
		},
		{
			ID:       "segment-deny",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Hidden Prayer",
			Body:     "prayer behind the wall",
			Fields: map[string]any{
				"parent_title": "Hidden Prayer",
				"parent_url":   "https://example.org/video-2",
			},
			Facets: map[string][]string{"topic": {"hidden"}},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{
		Registry:   registry,
		ScopeGuard: allowListScopeGuard{allowed: map[string]struct{}{"segment-allow": {}}},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		GroupBy: "parent_id",
		Facets:  []types.FacetRequest{{Field: "topic", Limit: 10}},
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Groups) != 1 {
		t.Fatalf("expected one authorized group, got %+v", page.Groups)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != "segment-allow" {
		t.Fatalf("expected only authorized hit, got %+v", page.Hits)
	}
	if len(page.Facets) != 1 || len(page.Facets[0].Values) != 1 || page.Facets[0].Values[0].Value != "archive" {
		t.Fatalf("expected facets to be recomputed from authorized hits, got %+v", page.Facets)
	}
}

func TestSearchScopeGuardKeepsFacetCountsConservativeWhenPageHitsAreAuthorized(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "content", FilterableFields: []string{"topic"}, FacetFields: []string{"topic"}}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	docs := []types.Document{
		{ID: "event-1-title", Index: "content", Title: "Prayer", ResultID: "event:1", Facets: map[string][]string{"topic": {"practice"}}},
		{ID: "event-1-transcript", Index: "content", Body: "Prayer", ResultID: "event:1", Facets: map[string][]string{"topic": {"practice"}}},
		{ID: "event-2-title", Index: "content", Title: "Prayer", ResultID: "event:2", Facets: map[string][]string{"topic": {"practice"}}},
	}
	if err := provider.UpsertDocuments(context.Background(), "content", docs); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	allowed := map[string]struct{}{}
	for _, doc := range docs {
		allowed[doc.ID] = struct{}{}
	}
	p, err := planner.New(planner.Config{Registry: registry, ScopeGuard: allowListScopeGuard{allowed: allowed}})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"content"}, Query: "Prayer", Page: 1, PerPage: 10,
		Facets: []types.FacetRequest{{Field: "topic", CountBy: types.FacetCountByResultID, IdentityLimit: 10}},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Facets) != 1 || len(page.Facets[0].Values) != 1 {
		t.Fatalf("entity facets = %+v", page.Facets)
	}
	value := page.Facets[0].Values[0]
	if value.Count != 2 || !reflect.DeepEqual(value.EntityIDs, []string{"event:1", "event:2"}) {
		t.Fatalf("entity facet value = %+v", value)
	}
	if page.Facets[0].Accuracy != types.FacetCountAccuracyLowerBound {
		t.Fatalf("accuracy = %q", page.Facets[0].Accuracy)
	}
}

func TestSearchScopeGuardRebuildsEntityFacetsAsUniqueLowerBound(t *testing.T) {
	page := types.SearchResultPage{
		Hits: []types.SearchHit{
			{ID: "a1", ResultID: "event:1", Document: &types.Document{ID: "a1", ResultID: "event:1", Facets: map[string][]string{"topic": {"practice"}}}},
			{ID: "a2", ResultID: "event:1", Document: &types.Document{ID: "a2", ResultID: "event:1", Facets: map[string][]string{"topic": {"practice"}}}},
			{ID: "denied", ResultID: "event:2", Document: &types.Document{ID: "denied", ResultID: "event:2", Facets: map[string][]string{"topic": {"practice"}}}},
		},
		Total: 3,
	}
	filtered := filterSearchPage(context.Background(), types.SearchRequest{
		Facets: []types.FacetRequest{{Field: "topic", CountBy: types.FacetCountByResultID, IdentityLimit: 10}},
	}, page, allowListScopeGuard{allowed: map[string]struct{}{"a1": {}, "a2": {}}})
	if len(filtered.Facets) != 1 || len(filtered.Facets[0].Values) != 1 {
		t.Fatalf("facets = %+v", filtered.Facets)
	}
	value := filtered.Facets[0].Values[0]
	if value.Count != 1 || !reflect.DeepEqual(value.EntityIDs, []string{"event:1"}) {
		t.Fatalf("entity facet value = %+v", value)
	}
	if filtered.Facets[0].Accuracy != types.FacetCountAccuracyLowerBound || filtered.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("accuracy = %q total_accuracy = %q", filtered.Facets[0].Accuracy, filtered.TotalAccuracy)
	}
}

func TestSearchPreservesProviderAccuracyWhenGuardAssertsProviderAuthorization(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "content", FacetFields: []string{"topic"}}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "content", []types.Document{
		{
			ID:         "public",
			Index:      "content",
			Title:      "Prayer",
			Visibility: types.Visibility{Public: true},
			Fields:     map[string]any{"visibility_public": true},
			Facets:     map[string][]string{"topic": {"practice"}},
		},
		{
			ID:         "private",
			Index:      "content",
			Title:      "Prayer",
			Visibility: types.Visibility{Public: false},
			Fields:     map[string]any{"visibility_public": false},
			Facets:     map[string][]string{"topic": {"restricted"}},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry, ScopeGuard: providerEnforcedPublicScopeGuard{}})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"content"}, Query: "Prayer", Page: 1, PerPage: 10,
		Filters: types.TermExpr{Field: "visibility_public", Op: types.FilterOpEQ, Value: true},
		Facets:  []types.FacetRequest{{Field: "topic", Limit: 10}},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Hits) != 1 || page.Hits[0].ID != "public" {
		t.Fatalf("expected only provider-authorized result, got %+v", page)
	}
	if page.TotalAccuracy != "" && page.TotalAccuracy != types.TotalAccuracyExact {
		t.Fatalf("total accuracy = %q", page.TotalAccuracy)
	}
	if len(page.Facets) != 1 || (page.Facets[0].Accuracy != "" && page.Facets[0].Accuracy != types.FacetCountAccuracyExact) || len(page.Facets[0].Values) != 1 || page.Facets[0].Values[0].Value != "practice" {
		t.Fatalf("expected exact provider facet, got %+v", page.Facets)
	}
}

func TestSearchFallsBackToConservativeFilteringWithoutProviderAuthorizationAssertion(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "content", FacetFields: []string{"topic"}}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "content", []types.Document{
		{ID: "public", Index: "content", Title: "Prayer", Visibility: types.Visibility{Public: true}, Facets: map[string][]string{"topic": {"practice"}}},
		{ID: "private", Index: "content", Title: "Prayer", Visibility: types.Visibility{Public: false}, Facets: map[string][]string{"topic": {"restricted"}}},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry, ScopeGuard: providerEnforcedPublicScopeGuard{}})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"content"}, Query: "Prayer", Page: 1, PerPage: 10,
		Facets: []types.FacetRequest{{Field: "topic", Limit: 10}},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Hits) != 1 || page.Hits[0].ID != "public" {
		t.Fatalf("expected post-filtered public result, got %+v", page)
	}
	if page.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("total accuracy = %q", page.TotalAccuracy)
	}
	if len(page.Facets) != 1 || page.Facets[0].Accuracy != types.FacetCountAccuracyLowerBound {
		t.Fatalf("expected conservative facet accuracy, got %+v", page.Facets)
	}
}

func TestSearchScopeGuardPreservesExactEmptyTotal(t *testing.T) {
	page := filterSearchPage(context.Background(), types.SearchRequest{}, types.SearchResultPage{
		Total:         0,
		TotalAccuracy: types.TotalAccuracyExact,
	}, allowListScopeGuard{allowed: map[string]struct{}{}})
	if page.Total != 0 || page.TotalAccuracy != types.TotalAccuracyExact {
		t.Fatalf("expected exact empty total, got total=%d accuracy=%q", page.Total, page.TotalAccuracy)
	}
}

func TestSearchScopeGuardKeepsBoundedEmptyTotalConservative(t *testing.T) {
	page := filterSearchPage(context.Background(), types.SearchRequest{}, types.SearchResultPage{
		Total:         0,
		TotalAccuracy: types.TotalAccuracyLowerBound,
	}, allowListScopeGuard{allowed: map[string]struct{}{}})
	if page.Total != 0 || page.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("expected bounded empty total to remain conservative, got total=%d accuracy=%q", page.Total, page.TotalAccuracy)
	}
}

func TestProviderAuthorizationAssertionCoversDisjunctiveFacetFilters(t *testing.T) {
	guard := providerEnforcedPublicScopeGuard{}
	req := types.SearchRequest{
		Filters: types.TermExpr{Field: "visibility_public", Op: types.FilterOpEQ, Value: true},
		Facets:  []types.FacetRequest{{Field: "visibility_public", Disjunctive: true}},
	}
	if searchResponseAuthorizationEnforcedByProvider(context.Background(), guard, req) {
		t.Fatal("expected provider assertion to fail when disjunctive facet counting removes its authorization filter")
	}
}

func TestProviderAuthorizedEvidenceDoesNotReapplyGuard(t *testing.T) {
	registry := indexing.NewRegistry()
	if err := registry.Register(types.IndexDefinition{Name: "content"}, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry, ScopeGuard: providerEnforcedPublicScopeGuard{}})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	provider := new(capturingEvidenceProvider)
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page := types.SearchResultPage{Hits: []types.SearchHit{{ID: "public", ResultID: "public"}}}
	search.attachEvidence(context.Background(), types.SearchRequest{Indexes: []string{"content"}}, &page, true)
	if provider.lastGuard != nil {
		t.Fatal("expected provider-authorized evidence aggregation to omit the post-filter guard")
	}
	if page.Hits[0].Evidence == nil || page.Hits[0].Evidence.Status != types.EvidenceStatusComplete {
		t.Fatalf("expected complete provider evidence, got %+v", page.Hits[0].Evidence)
	}
}

func TestSuggestFiltersUnauthorizedSuggestionsAfterOverfetch(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:       "segment-deny",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Alpha Ocean",
			Body:     "ocean text",
			Fields: map[string]any{
				"parent_title": "Alpha Ocean",
				"parent_url":   "https://example.org/video-1",
			},
		},
		{
			ID:       "segment-allow",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Ocean Wind",
			Body:     "ocean text",
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-2",
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{
		Registry:   registry,
		ScopeGuard: allowListScopeGuard{allowed: map[string]struct{}{"segment-allow": {}}},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	suggest, err := NewSuggest(SuggestConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new suggest query: %v", err)
	}
	result, err := suggest.Query(context.Background(), types.SuggestRequest{
		Indexes:      []string{"media"},
		Query:        "Ocean",
		PreferParent: true,
		Limit:        1,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "video-2" {
		t.Fatalf("expected authorized parent suggestion after filtering, got %+v", result.Items)
	}
}

func TestSuggestUsesConfiguredScopeGuardOverfetch(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	p, err := planner.New(planner.Config{
		Registry:   registry,
		ScopeGuard: allowListScopeGuard{allowed: map[string]struct{}{}},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	provider := new(capturingSuggestProvider)
	suggest, err := NewSuggest(SuggestConfig{
		Planner:                    p,
		Provider:                   provider,
		ScopeGuardFetchMultiplier:  3,
		MinimumScopeGuardFetchSize: 8,
	})
	if err != nil {
		t.Fatalf("new suggest query: %v", err)
	}
	if _, err := suggest.Query(context.Background(), types.SuggestRequest{
		Indexes: []string{"media"},
		Query:   "Ocean",
		Limit:   2,
	}); err != nil {
		t.Fatalf("suggest query: %v", err)
	}
	if provider.lastSuggest.Limit != 8 {
		t.Fatalf("expected configured overfetch limit 8, got %d", provider.lastSuggest.Limit)
	}
}

func TestSuggestAppliesEditorialHideAndPinRules(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:       "segment-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Alpha Ocean",
			Body:     "ocean text",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Alpha Ocean",
				"parent_url":   "https://example.org/video-1",
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Ocean Wind",
			Body:     "ocean text",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-2",
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	pinPosition := 0
	suggest, err := NewSuggest(SuggestConfig{
		Planner:  p,
		Provider: provider,
		Editorial: staticEditorialStore{rules: []types.EditorialRankRule{
			{
				ID:             "hide-video-1",
				ParentTargetID: "video-1",
				Action:         types.EditorialActionHide,
				Enabled:        true,
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Query: "Ocean", Locale: "en"},
			},
			{
				ID:             "pin-video-2",
				ParentTargetID: "video-2",
				Action:         types.EditorialActionPin,
				Enabled:        true,
				Position:       &pinPosition,
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Query: "Ocean", Locale: "en"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("new suggest query: %v", err)
	}
	result, err := suggest.Query(context.Background(), types.SuggestRequest{
		Indexes:      []string{"media"},
		Query:        "Ocean",
		Locale:       "en",
		PreferParent: true,
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "video-2" {
		t.Fatalf("expected editorial suggest result to hide video-1 and keep pinned video-2 first, got %+v", result.Items)
	}
}

func TestSearchUsesPlannerCompiledLocaleRequest(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	p, err := planner.New(planner.Config{
		Registry:      registry,
		LocaleRuntime: queryLocaleRuntime{},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	provider := &capturingSearchProvider{
		page: types.SearchResultPage{
			Hits: []types.SearchHit{{ID: "doc-1", Locale: "es", Document: &types.Document{ID: "doc-1", Locale: "es"}}},
		},
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  " ES_mx ",
		Locales: []string{"BO", "es"},
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search query: %v", err)
	}
	if provider.lastSearch.Locale != "es" {
		t.Fatalf("provider locale = %q", provider.lastSearch.Locale)
	}
	if !reflect.DeepEqual(provider.lastSearch.Locales, []string{"bo"}) {
		t.Fatalf("provider locales = %#v", provider.lastSearch.Locales)
	}
	if page.Hits[0].Retrieval == nil || page.Hits[0].Retrieval.Metadata["locale_match"] != "exact" {
		t.Fatalf("expected locale annotation on result hit, got %+v", page.Hits[0].Retrieval)
	}
}
