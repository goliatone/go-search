package memory

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

func TestProviderContractSuite(t *testing.T) {
	providers.RunContractSuite(t, func(t *testing.T) providers.Provider {
		t.Helper()
		return New(Config{})
	})
}

func TestProviderEnforcesScopeForSearchAndSuggest(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", []types.Document{
		{
			ID:    "doc-1",
			Index: "media",
			Title: "Ocean Wind",
			Body:  "archive prayer",
			Scope: types.Scope{TenantID: "tenant-a"},
		},
		{
			ID:    "doc-2",
			Index: "media",
			Title: "Mountain Chant",
			Body:  "archive prayer",
			Scope: types.Scope{TenantID: "tenant-b"},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Scope:   types.Scope{TenantID: "tenant-a"},
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || page.Hits[0].ID != "doc-1" {
		t.Fatalf("expected only tenant-a hit, got %+v", page.Hits)
	}
	suggest, err := provider.Suggest(ctx, types.SuggestRequest{
		Indexes: []string{"media"},
		Query:   "Ocean",
		Scope:   types.Scope{TenantID: "tenant-a"},
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggest.Items) != 1 || suggest.Items[0].ID != "doc-1" {
		t.Fatalf("expected only tenant-a suggestion, got %+v", suggest.Items)
	}
}

func TestProviderPreferParentSuggestionsAreDeduplicated(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", []types.Document{
		{
			ID:       "segment-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Ocean Wind",
			Body:     "prayer one",
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-1",
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Ocean Wind",
			Body:     "prayer two",
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-1",
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	suggest, err := provider.Suggest(ctx, types.SuggestRequest{
		Indexes:      []string{"media"},
		Query:        "Ocean",
		PreferParent: true,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggest.Items) != 1 || suggest.Items[0].ID != "video-1" {
		t.Fatalf("expected one parent suggestion, got %+v", suggest.Items)
	}
}

func TestProviderRejectsInvalidFilterOperator(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	_, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "Ocean",
		Filters: types.TermExpr{Field: "topic", Op: types.FilterOp("wildcard"), Value: "archive"},
		Page:    1,
		PerPage: 10,
	})
	if err == nil {
		t.Fatalf("expected invalid filter operator error")
	}
}

func TestProviderPrefersExactLocaleAndAnnotatesLocaleMatch(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", []types.Document{
		{
			ID:     "doc-exact",
			Index:  "media",
			Title:  "Ocean Wind",
			Body:   "prayer",
			Locale: "en",
		},
		{
			ID:     "doc-fallback",
			Index:  "media",
			Title:  "Ocean Wind",
			Body:   "prayer",
			Locale: "bo",
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Locales: []string{"bo"},
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Hits) != 2 {
		t.Fatalf("expected two hits, got %+v", page.Hits)
	}
	if page.Hits[0].ID != "doc-exact" {
		t.Fatalf("expected exact locale hit first, got %+v", page.Hits)
	}
	if page.Hits[0].Retrieval == nil || page.Hits[0].Retrieval.Metadata["locale_match"] != "exact" {
		t.Fatalf("expected exact locale retrieval metadata, got %+v", page.Hits[0].Retrieval)
	}
	if page.Hits[1].Retrieval == nil || page.Hits[1].Retrieval.Metadata["locale_match"] != "fallback" {
		t.Fatalf("expected fallback locale retrieval metadata, got %+v", page.Hits[1].Retrieval)
	}
}

func TestProviderSuggestUsesExactPrimaryLocaleOnly(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", []types.Document{
		{ID: "doc-exact", Index: "media", Title: "Ocean Wind", Body: "prayer", Locale: "en"},
		{ID: "doc-alt", Index: "media", Title: "Ocean Wind", Body: "prayer", Locale: "bo"},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	result, err := provider.Suggest(ctx, types.SuggestRequest{
		Indexes: []string{"media"},
		Query:   "Ocean",
		Locale:  "en",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "doc-exact" {
		t.Fatalf("expected exact locale suggestion only, got %+v", result.Items)
	}
}

func TestProviderBuildsDisjunctiveHierarchicalFacets(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", []types.Document{
		{
			ID:    "doc-1",
			Index: "media",
			Title: "Ocean Wind",
			Body:  "prayer",
			Facets: map[string][]string{
				"topic_hierarchy": {"Teaching Topics", "Teaching Topics > Tara"},
				"format":          {"Teaching"},
			},
		},
		{
			ID:    "doc-2",
			Index: "media",
			Title: "Mountain Wind",
			Body:  "prayer",
			Facets: map[string][]string{
				"topic_hierarchy": {"Teaching Topics", "Teaching Topics > Architecture"},
				"format":          {"Teaching"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.TermExpr{Field: "format", Op: types.FilterOpEQ, Value: "Teaching"},
			types.TermExpr{Field: "topic_hierarchy", Op: types.FilterOpEQ, Value: "Teaching Topics > Tara"},
		}},
		Facets: []types.FacetRequest{
			{Field: "topic_hierarchy", Kind: types.FacetKindHierarchical, Disjunctive: true},
			{Field: "format", Disjunctive: true},
		},
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Facets) != 2 {
		t.Fatalf("facets = %+v", page.Facets)
	}
	if page.Facets[0].Kind != types.FacetKindHierarchical {
		t.Fatalf("hierarchical facet metadata = %+v", page.Facets[0])
	}
	foundArchitecture := false
	for _, value := range page.Facets[0].Values {
		if value.Value == "Teaching Topics > Architecture" {
			foundArchitecture = true
		}
	}
	if !foundArchitecture {
		t.Fatalf("expected disjunctive hierarchy count to preserve sibling branch, got %+v", page.Facets[0].Values)
	}
}

func TestProviderSupportsRangeFilteringWithSelectedDisjunctiveArchiveFacets(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", []types.Document{
		{
			ID:    "doc-1",
			Index: "media",
			Title: "Architecture Walkthrough",
			Body:  "archive prayer",
			Facets: map[string][]string{
				"topic_hierarchy": {"Teaching Topics", "Teaching Topics > Architecture"},
				"format":          {"Teaching"},
			},
			Numeric: map[string]float64{
				"published_year":   2024,
				"duration_seconds": 2400,
			},
		},
		{
			ID:    "doc-2",
			Index: "media",
			Title: "Tara Teachings",
			Body:  "archive prayer",
			Facets: map[string][]string{
				"topic_hierarchy": {"Teaching Topics", "Teaching Topics > Tara"},
				"format":          {"Teaching"},
			},
			Numeric: map[string]float64{
				"published_year":   2024,
				"duration_seconds": 2400,
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.TermExpr{Field: "topic_hierarchy", Op: types.FilterOpEQ, Value: "Teaching Topics > Architecture"},
			types.RangeExpr{Field: "published_year", GTE: 2024},
			types.RangeExpr{Field: "duration_seconds", GTE: 1800},
		}},
		Facets: []types.FacetRequest{
			{Field: "topic_hierarchy", Kind: types.FacetKindHierarchical, Disjunctive: true},
			{Field: "format", Disjunctive: true},
		},
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected one ranged result, got %+v", page)
	}
	foundArchitecture := false
	foundTara := false
	for _, facet := range page.Facets {
		if facet.Field != "topic_hierarchy" {
			continue
		}
		for _, value := range facet.Values {
			if value.Value == "Teaching Topics > Architecture" && value.Selected {
				foundArchitecture = true
			}
			if value.Value == "Teaching Topics > Tara" {
				foundTara = true
			}
		}
	}
	if !foundArchitecture {
		t.Fatalf("expected selected hierarchical value in %+v", page.Facets)
	}
	if !foundTara {
		t.Fatalf("expected disjunctive sibling count to remain visible in %+v", page.Facets)
	}
}

func TestProviderExistsExprMatchesCanonicalAndFacetFields(t *testing.T) {
	provider := New(Config{})
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	start := int64(1000)
	if err := provider.UpsertDocuments(ctx, "media", []types.Document{{
		ID:      "doc-1",
		Index:   "media",
		Title:   "Ocean Wind",
		Body:    "prayer",
		StartMS: &start,
		Facets:  map[string][]string{"topic": {"archive"}},
	}}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	for _, filter := range []types.FilterExpr{
		types.ExistsExpr{Field: "start_ms", Exists: true},
		types.ExistsExpr{Field: "topic", Exists: true},
	} {
		page, err := provider.Search(ctx, types.SearchRequest{
			Indexes: []string{"media"},
			Query:   "prayer",
			Filters: filter,
			Page:    1,
			PerPage: 10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if page.Total != 1 {
			t.Fatalf("expected exists filter %+v to match canonical field, got %+v", filter, page)
		}
	}
}
