package goadmin

import (
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

func TestToSearchRequestUsesCollectionsMetadataWhenIndexesMissing(t *testing.T) {
	req := ToSearchRequest(nil, SiteSearchRequest{
		Query:   "archive",
		Locale:  "en",
		Page:    2,
		PerPage: 5,
		Sort:    "published_at:desc",
		Filters: map[string][]string{"topic": {"archive"}},
		Metadata: map[string]any{
			"collections": []string{"media"},
		},
	})
	if len(req.Indexes) != 1 || req.Indexes[0] != "media" {
		t.Fatalf("indexes = %#v", req.Indexes)
	}
	if len(req.Sort) != 1 || req.Sort[0].Direction != types.SortDesc {
		t.Fatalf("sort = %#v", req.Sort)
	}
	if req.Page != 2 || req.PerPage != 5 {
		t.Fatalf("pagination = %#v", req)
	}
}

func TestSiteResultFromPagePreservesGroupsInMetadata(t *testing.T) {
	page := types.SearchResultPage{
		Page:    1,
		PerPage: 10,
		Total:   1,
		Groups: []types.SearchGroup{
			{
				Key: "video-1",
				TopHit: &types.SearchHit{
					ID:    "segment-1",
					Title: "Ocean Wind",
				},
			},
		},
		Hits: []types.SearchHit{
			{
				ID:      "segment-1",
				Type:    "transcript_segment",
				Title:   "Ocean Wind",
				Summary: "archive chant",
				Score:   12,
				Parent:  &types.SearchParent{ID: "video-1", Title: "Ocean Wind"},
			},
		},
	}
	result := SiteResultFromPage(page)
	if len(result.Hits) != 1 {
		t.Fatalf("hits = %#v", result.Hits)
	}
	if _, ok := result.Metadata["groups"]; !ok {
		t.Fatalf("expected groups metadata, got %#v", result.Metadata)
	}
}

func TestGlobalResultsFromPageUsesTopHitsWhenGrouped(t *testing.T) {
	page := types.SearchResultPage{
		Groups: []types.SearchGroup{
			{
				Key: "video-1",
				TopHit: &types.SearchHit{
					ID:    "segment-1",
					Type:  "transcript_segment",
					Title: "Ocean Wind",
				},
			},
		},
	}
	results := GlobalResultsFromPage(page, "search")
	if len(results) != 1 || results[0].ID != "segment-1" {
		t.Fatalf("results = %#v", results)
	}
}
