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

func (s staticEditorialStore) ListApplicable(context.Context, types.SearchRequest) ([]types.EditorialRankRule, error) {
	return append([]types.EditorialRankRule(nil), s.rules...), nil
}

func (s staticEditorialStore) Upsert(context.Context, types.EditorialRankRule) error {
	return nil
}

func (s staticEditorialStore) Delete(context.Context, string) error {
	return nil
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
