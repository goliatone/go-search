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
