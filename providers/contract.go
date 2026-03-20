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
}
