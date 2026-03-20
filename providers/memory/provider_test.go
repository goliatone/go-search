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
		return New()
	})
}

func TestProviderEnforcesScopeForSearchAndSuggest(t *testing.T) {
	provider := New()
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
	provider := New()
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
