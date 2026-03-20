package query

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/indexing"
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

type capturingSuggestProvider struct {
	lastSuggest types.SuggestRequest
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

func (p *capturingSuggestProvider) ReplaceDocuments(context.Context, string, []string, []types.Document) error {
	return nil
}

func (p *capturingSuggestProvider) DeleteDocuments(context.Context, string, []string) error {
	return nil
}

func (p *capturingSuggestProvider) DeleteBySource(context.Context, string, []string) error {
	return nil
}

func (p *capturingSuggestProvider) Health(context.Context, types.HealthRequest) (types.HealthStatus, error) {
	return types.HealthStatus{}, nil
}

func TestSearchFiltersUnauthorizedHitsGroupsAndFacets(t *testing.T) {
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
