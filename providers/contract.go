package providers

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

type Factory func(t *testing.T) Provider

func RunContractSuite(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("ensure index and health", func(t *testing.T) {
		provider := factory(t)
		ctx := context.Background()
		def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
		if err := provider.EnsureIndex(ctx, def); err != nil {
			t.Fatalf("ensure index: %v", err)
		}
		health, err := provider.Health(ctx, types.HealthRequest{})
		if err != nil {
			t.Fatalf("health: %v", err)
		}
		if !health.Healthy {
			t.Fatalf("expected healthy provider")
		}
	})

	t.Run("upsert search facets group and delete by source", func(t *testing.T) {
		provider := factory(t)
		ctx := context.Background()
		def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
		if err := provider.EnsureIndex(ctx, def); err != nil {
			t.Fatalf("ensure index: %v", err)
		}
		start1, end1 := int64(1000), int64(2000)
		start2, end2 := int64(3000), int64(4000)
		docs := []types.Document{
			{
				ID:         "segment-1",
				Index:      "media",
				Type:       types.DocumentTypeTranscriptSegment,
				ParentID:   "video-1",
				SourceType: "transcript",
				SourceID:   "track-1",
				Title:      "Ocean Wind",
				Body:       "ocean wind chanting prayer",
				URL:        "https://example.org/video-1",
				AnchorURL:  "https://example.org/video-1#t=1",
				Locale:     "en",
				StartMS:    &start1,
				EndMS:      &end1,
				Facets:     map[string][]string{"topic": {"archive"}},
			},
			{
				ID:         "segment-2",
				Index:      "media",
				Type:       types.DocumentTypeTranscriptSegment,
				ParentID:   "video-1",
				SourceType: "transcript",
				SourceID:   "track-1",
				Title:      "Ocean Wind",
				Body:       "prayer continues",
				URL:        "https://example.org/video-1",
				AnchorURL:  "https://example.org/video-1#t=3",
				Locale:     "en",
				StartMS:    &start2,
				EndMS:      &end2,
				Facets:     map[string][]string{"topic": {"archive"}},
			},
		}
		if err := provider.UpsertDocuments(ctx, "media", docs); err != nil {
			t.Fatalf("upsert documents: %v", err)
		}
		page, err := provider.Search(ctx, types.SearchRequest{
			Indexes: []string{"media"},
			Query:   "prayer",
			Page:    1,
			PerPage: 10,
			GroupBy: "parent_id",
			Facets:  []types.FacetRequest{{Field: "topic", Limit: 10}},
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(page.Groups) != 1 {
			t.Fatalf("expected one group, got %d", len(page.Groups))
		}
		if len(page.Facets) != 1 || len(page.Facets[0].Values) != 1 {
			t.Fatalf("expected facet values, got %+v", page.Facets)
		}
		suggest, err := provider.Suggest(ctx, types.SuggestRequest{
			Indexes: []string{"media"},
			Query:   "Ocean",
			Limit:   5,
		})
		if err != nil {
			t.Fatalf("suggest: %v", err)
		}
		if len(suggest.Items) == 0 {
			t.Fatalf("expected suggestions")
		}
		if err := provider.DeleteBySource(ctx, "media", []string{"track-1"}); err != nil {
			t.Fatalf("delete by source: %v", err)
		}
		page, err = provider.Search(ctx, types.SearchRequest{
			Indexes: []string{"media"},
			Query:   "prayer",
			Page:    1,
			PerPage: 10,
		})
		if err != nil {
			t.Fatalf("search after delete: %v", err)
		}
		if page.Total != 0 {
			t.Fatalf("expected zero results after delete by source, got %d", page.Total)
		}
	})

	t.Run("heterogeneous flat multi-index search", func(t *testing.T) {
		provider := factory(t)
		ctx := context.Background()
		defs := []types.IndexDefinition{
			{Name: "videos"},
			{Name: "documents"},
			{Name: "blog_articles"},
		}
		for _, def := range defs {
			if err := provider.EnsureIndex(ctx, def); err != nil {
				t.Fatalf("ensure index %s: %v", def.Name, err)
			}
		}
		if err := provider.UpsertDocuments(ctx, "videos", []types.Document{{
			ID:      "video-1",
			Index:   "videos",
			Type:    types.DocumentTypeVideo,
			Title:   "Search Architecture Walkthrough",
			Summary: "video summary",
			Body:    "search architecture video",
			URL:     "/videos/1",
			Locale:  "en",
			Facets:  map[string][]string{"entity_type": {types.DocumentTypeVideo}},
		}}); err != nil {
			t.Fatalf("upsert video doc: %v", err)
		}
		if err := provider.UpsertDocuments(ctx, "documents", []types.Document{{
			ID:      "document-1",
			Index:   "documents",
			Type:    types.DocumentTypeDocument,
			Title:   "Search Rollout Workbook",
			Summary: "document summary",
			Body:    "search rollout document",
			URL:     "/documents/1",
			Locale:  "en",
			Facets:  map[string][]string{"entity_type": {types.DocumentTypeDocument}},
		}}); err != nil {
			t.Fatalf("upsert document doc: %v", err)
		}
		if err := provider.UpsertDocuments(ctx, "blog_articles", []types.Document{{
			ID:      "blog-1",
			Index:   "blog_articles",
			Type:    types.DocumentTypeBlogArticle,
			Title:   "Search Notes",
			Summary: "blog summary",
			Body:    "search notes article",
			URL:     "/blog/1",
			Locale:  "en",
			Facets:  map[string][]string{"entity_type": {types.DocumentTypeBlogArticle}},
		}}); err != nil {
			t.Fatalf("upsert blog doc: %v", err)
		}
		page, err := provider.Search(ctx, types.SearchRequest{
			Indexes: []string{"videos", "documents", "blog_articles"},
			Query:   "search",
			Page:    1,
			PerPage: 10,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if page.Total < 3 {
			t.Fatalf("expected at least three hits, got %+v", page)
		}
		found := map[string]bool{}
		for _, hit := range page.Hits {
			found[hit.Type] = true
		}
		for _, typ := range []string{types.DocumentTypeVideo, types.DocumentTypeDocument, types.DocumentTypeBlogArticle} {
			if !found[typ] {
				t.Fatalf("expected hit type %q in %+v", typ, page.Hits)
			}
		}
	})
}
