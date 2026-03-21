package query

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers/memory"
)

type staticEditorialStore struct {
	rules []types.EditorialRankRule
}

type countingBatchProvider struct {
	*memory.Provider
	searchCalls int
	batchCalls  int
	batchSize   int
}

func (s staticEditorialStore) ListApplicable(context.Context, types.SearchRequest) ([]types.EditorialRankRule, error) {
	return append([]types.EditorialRankRule(nil), s.rules...), nil
}

func (s staticEditorialStore) Upsert(context.Context, types.EditorialRankRule) error {
	return nil
}

func (s staticEditorialStore) Delete(context.Context, string) error {
	return nil
}

func (p *countingBatchProvider) Search(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	p.searchCalls++
	return p.Provider.Search(ctx, req)
}

func (p *countingBatchProvider) SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
	p.batchCalls++
	p.batchSize = len(requests)
	out := make([]types.SearchResultPage, 0, len(requests))
	for _, req := range requests {
		page, err := p.Provider.Search(ctx, req)
		if err != nil {
			return nil, err
		}
		out = append(out, page)
	}
	return out, nil
}

func TestSearchGroupsAfterEditorialRanking(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
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
			Title:    "Ocean Wind",
			Body:     "prayer on the shore",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-1",
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Mountain Prayer",
			Body:     "prayer in the mountains",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Mountain Prayer",
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
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
		Editorial: staticEditorialStore{rules: []types.EditorialRankRule{
			{
				ID:             "pin-video-2",
				ParentTargetID: "video-2",
				Action:         types.EditorialActionPin,
				Enabled:        true,
				Position:       new(0),
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Locale: "en"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 1,
		GroupBy: "parent_id",
	})
	if err != nil {
		t.Fatalf("query page 1: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected total group count 2, got %d", page.Total)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-2" {
		t.Fatalf("expected pinned group on page 1, got %+v", page.Groups)
	}
	page, err = search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    2,
		PerPage: 1,
		GroupBy: "parent_id",
	})
	if err != nil {
		t.Fatalf("query page 2: %v", err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-1" {
		t.Fatalf("expected second group on page 2, got %+v", page.Groups)
	}
}

func TestSearchHideRuleRemovesParentGroup(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{ID: "segment-1", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-1", Title: "Ocean Wind", Body: "prayer on the shore", Locale: "en"},
		{ID: "segment-2", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-2", Title: "Mountain Prayer", Body: "prayer in the mountains", Locale: "en"},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
		Editorial: staticEditorialStore{rules: []types.EditorialRankRule{
			{
				ID:             "hide-video-1",
				ParentTargetID: "video-1",
				Action:         types.EditorialActionHide,
				Enabled:        true,
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Locale: "en"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
		GroupBy: "parent_id",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-2" {
		t.Fatalf("groups = %#v", page.Groups)
	}
}

func TestSearchParentTargetDoesNotMatchUnrelatedHit(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{ID: "segment-1", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-1", Title: "Ocean Wind", Body: "prayer on the shore", Locale: "en"},
		{ID: "segment-2", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-2", Title: "Mountain Prayer", Body: "prayer in the mountains", Locale: "en"},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
		Editorial: staticEditorialStore{rules: []types.EditorialRankRule{
			{
				ID:             "boost-video-2",
				ParentTargetID: "video-2",
				Action:         types.EditorialActionBoost,
				Weight:         100,
				Enabled:        true,
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Locale: "en"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Hits) < 2 {
		t.Fatalf("hits = %#v", page.Hits)
	}
	if page.Hits[0].Parent != nil && page.Hits[0].Parent.ID != "video-2" {
		t.Fatalf("unexpected top hit after parent-target boost: %#v", page.Hits[0])
	}
	for _, hit := range page.Hits {
		if hit.Parent != nil && hit.Parent.ID == "video-1" && hit.FinalScore >= 100 {
			t.Fatalf("unexpected unrelated boost on hit %#v", hit)
		}
	}
}

func TestSearchGroupedDisjunctiveFacetsCountUniqueGroups(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
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
			Title:    "Architecture One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture One",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Teaching"},
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Architecture One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture One",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Teaching"},
			},
		},
		{
			ID:       "segment-3",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Architecture Two",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture Two",
				"parent_url":   "https://example.org/video-2",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Workshop"},
			},
		},
		{
			ID:       "segment-4",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-3",
			Title:    "UI One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "UI One",
				"parent_url":   "https://example.org/video-3",
			},
			Facets: map[string][]string{
				"topic":  {"ui"},
				"format": {"Teaching"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
		GroupBy: "parent_id",
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: "architecture"},
			types.TermExpr{Field: "format", Op: types.FilterOpEQ, Value: "Teaching"},
		}},
		Facets: []types.FacetRequest{
			{Field: "format", Disjunctive: true},
			{Field: "topic", Disjunctive: true},
		},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-1" {
		t.Fatalf("expected only teaching architecture group in results, got %+v", page.Groups)
	}
	formatFacet := facetByField(page.Facets, "format")
	if formatFacet == nil {
		t.Fatalf("missing format facet: %+v", page.Facets)
	}
	formatCounts := facetCounts(*formatFacet)
	if formatCounts["Teaching"] != 1 || formatCounts["Workshop"] != 1 {
		t.Fatalf("unexpected format counts: %+v", formatCounts)
	}
	if !facetSelected(*formatFacet, "Teaching") {
		t.Fatalf("expected selected teaching value: %+v", formatFacet.Values)
	}
	topicFacet := facetByField(page.Facets, "topic")
	if topicFacet == nil {
		t.Fatalf("missing topic facet: %+v", page.Facets)
	}
	topicCounts := facetCounts(*topicFacet)
	if topicCounts["architecture"] != 1 || topicCounts["ui"] != 1 {
		t.Fatalf("unexpected topic counts: %+v", topicCounts)
	}
	if !facetSelected(*topicFacet, "architecture") {
		t.Fatalf("expected selected architecture value: %+v", topicFacet.Values)
	}
}

func TestSearchGroupedDisjunctiveFacetsUseBatchProviderWhenAvailable(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	baseProvider := memory.New(memory.Config{})
	provider := &countingBatchProvider{Provider: baseProvider}
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:       "segment-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Architecture One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture One",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Teaching"},
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Architecture Two",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture Two",
				"parent_url":   "https://example.org/video-2",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Workshop"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	_, err = search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
		GroupBy: "parent_id",
		Filters: types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: "architecture"},
		Facets: []types.FacetRequest{
			{Field: "format", Disjunctive: true},
			{Field: "topic", Disjunctive: true},
		},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if provider.searchCalls != 1 {
		t.Fatalf("expected one primary search call, got %d", provider.searchCalls)
	}
	if provider.batchCalls != 1 || provider.batchSize != 2 {
		t.Fatalf("expected one batch call for two facets, got batchCalls=%d batchSize=%d", provider.batchCalls, provider.batchSize)
	}
}

func facetByField(facets []types.SearchFacet, field string) *types.SearchFacet {
	for i := range facets {
		if facets[i].Field == field {
			return &facets[i]
		}
	}
	return nil
}

func facetCounts(facet types.SearchFacet) map[string]int {
	out := make(map[string]int, len(facet.Values))
	for _, value := range facet.Values {
		out[value.Value] = value.Count
	}
	return out
}

func facetSelected(facet types.SearchFacet, needle string) bool {
	for _, value := range facet.Values {
		if value.Value == needle {
			return value.Selected
		}
	}
	return false
}

func TestSearchRejectsUnsupportedSemanticMode(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	_, err = search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Mode:    types.SearchModeSemantic,
		Semantic: &types.SemanticRequest{
			Field: "body",
		},
	})
	if err == nil {
		t.Fatalf("expected unsupported semantic mode error")
	}
}

func TestSearchPrefersExactLocaleAndAnnotatesFallbackOrigins(t *testing.T) {
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
			ID:     "doc-exact",
			Index:  "media",
			Title:  "Oracion exacta",
			Body:   "prayer in spanish",
			Locale: "es",
		},
		{
			ID:     "doc-fallback",
			Index:  "media",
			Title:  "Fallback prayer",
			Body:   "prayer in english",
			Locale: "en",
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}

	runtime, err := locale.NewI18nRuntimeFromCultureData("../testdata/locale_search_culture.json", "en")
	if err != nil {
		t.Fatalf("new locale runtime: %v", err)
	}
	p, err := planner.New(planner.Config{
		Registry:      registry,
		LocaleRuntime: runtime,
		LocalePolicy: planner.LocalePolicy{
			MatchStrategy:   locale.MatchExactOrParent,
			Scope:           locale.ScopeActiveOnly,
			ExpandFallbacks: true,
			IncludeDefault:  true,
		},
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
		Locale:  "es",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search query: %v", err)
	}

	if len(page.Hits) != 2 {
		t.Fatalf("expected two hits, got %+v", page.Hits)
	}
	if page.Hits[0].ID != "doc-exact" {
		t.Fatalf("expected exact hit first, got %+v", page.Hits)
	}
	if page.Hits[0].Retrieval == nil || page.Hits[0].Retrieval.Metadata["locale_match"] != "exact" {
		t.Fatalf("exact hit metadata = %+v", page.Hits[0].Retrieval)
	}
	if origin := page.Hits[0].Retrieval.Metadata["locale_origin"]; origin != "matched" {
		t.Fatalf("exact hit locale origin = %#v", origin)
	}
	if page.Hits[1].Retrieval == nil || page.Hits[1].Retrieval.Metadata["locale_match"] != "fallback" {
		t.Fatalf("fallback hit metadata = %+v", page.Hits[1].Retrieval)
	}
	if origin := page.Hits[1].Retrieval.Metadata["locale_origin"]; origin != "default" {
		t.Fatalf("fallback hit locale origin = %#v", origin)
	}
}

//go:fix inline
func ptr[T any](value T) *T {
	return new(value)
}
