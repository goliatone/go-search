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

func TestToSearchRequestPrefersIndexesMetadataWhenPresent(t *testing.T) {
	req := ToSearchRequest(nil, SiteSearchRequest{
		Query: "archive",
		Metadata: map[string]any{
			"indexes":     []string{"media", "docs"},
			"collections": []string{"legacy"},
		},
	})
	if len(req.Indexes) != 2 || req.Indexes[0] != "media" || req.Indexes[1] != "docs" {
		t.Fatalf("indexes = %#v", req.Indexes)
	}
}

func TestToSearchRequestCombinesTermAndRangeFilters(t *testing.T) {
	req := ToSearchRequest([]string{"media"}, SiteSearchRequest{
		Query:   "archive",
		Filters: map[string][]string{"topic": {"archive", "tara"}},
		Ranges: []SiteSearchRange{
			{Field: "published_year", GTE: 2024},
			{Field: "duration_seconds", LTE: 3600},
		},
	})
	andExpr, ok := req.Filters.(types.AndExpr)
	if !ok {
		t.Fatalf("expected combined filter expression, got %#v", req.Filters)
	}
	var foundTopic, foundPublishedYear, foundDuration bool
	for _, term := range flattenFilterExpr(andExpr) {
		switch expr := term.(type) {
		case types.TermExpr:
			if expr.Field == "topic" && expr.Op == types.FilterOpIn {
				values, ok := expr.Value.([]string)
				if ok && len(values) == 2 && values[0] == "archive" && values[1] == "tara" {
					foundTopic = true
				}
			}
		case types.RangeExpr:
			if expr.Field == "published_year" && expr.GTE == 2024 {
				foundPublishedYear = true
			}
			if expr.Field == "duration_seconds" && expr.LTE == 3600 {
				foundDuration = true
			}
		}
	}
	if !foundTopic || !foundPublishedYear || !foundDuration {
		t.Fatalf("missing combined term/range filters in %#v", andExpr.Terms)
	}
}

func flattenFilterExpr(expr types.FilterExpr) []types.FilterExpr {
	switch typed := expr.(type) {
	case types.AndExpr:
		out := make([]types.FilterExpr, 0, len(typed.Terms))
		for _, term := range typed.Terms {
			out = append(out, flattenFilterExpr(term)...)
		}
		return out
	default:
		return []types.FilterExpr{expr}
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
				Parent:  &types.SearchParent{ID: "video-1", Title: "Ocean Wind", URL: "/media/ocean", Thumbnail: "/thumb.jpg"},
				Snippet: &types.SearchSnippet{Text: "archive chant", Highlighted: "<mark>archive</mark> chant"},
				Anchor:  &types.MediaAnchor{StartMS: 1000, EndMS: 2000, URL: "/media/ocean#t=1"},
				Fields: map[string]any{
					"parent_summary": "Ocean summary",
					"result_badge":   "Featured",
				},
				Ranking: &types.AppliedRankingSignals{
					Editorial: []types.AppliedEditorialSignal{{RuleID: "pin-1", Action: types.EditorialActionPin}},
				},
			},
		},
		Facets: []types.SearchFacet{
			{
				Field:       "topic_hierarchy",
				Kind:        types.FacetKindHierarchical,
				Disjunctive: true,
				Metadata:    map[string]any{"separator": " > "},
				Values: []types.SearchFacetValue{
					{
						Value:       "Teaching Topics > Architecture",
						Label:       "Architecture",
						Count:       2,
						Selected:    true,
						Path:        []string{"Teaching Topics", "Architecture"},
						Level:       1,
						ParentValue: "Teaching Topics",
					},
				},
			},
		},
	}
	result := SiteResultFromPage(page)
	if len(result.Hits) != 1 {
		t.Fatalf("hits = %#v", result.Hits)
	}
	if result.Hits[0].Highlighted != "<mark>archive</mark> chant" {
		t.Fatalf("highlighted = %#v", result.Hits[0].Highlighted)
	}
	if result.Hits[0].ParentURL != "/media/ocean" || result.Hits[0].ParentThumbnail != "/thumb.jpg" {
		t.Fatalf("parent metadata = %#v", result.Hits[0])
	}
	if result.Hits[0].ParentSummary != "Ocean summary" {
		t.Fatalf("parent summary = %#v", result.Hits[0].ParentSummary)
	}
	if result.Hits[0].Anchor == nil {
		t.Fatalf("expected anchor on hit")
	}
	if result.Hits[0].Metadata == nil || result.Hits[0].Metadata["ranking"] == nil {
		t.Fatalf("expected ranking metadata, got %#v", result.Hits[0].Metadata)
	}
	if len(result.Facets) != 1 || result.Facets[0].Kind != "hierarchical" || !result.Facets[0].Disjunctive {
		t.Fatalf("facet metadata = %#v", result.Facets)
	}
	if len(result.Facets[0].Buckets) != 1 || !result.Facets[0].Buckets[0].Selected || result.Facets[0].Buckets[0].Label != "Architecture" {
		t.Fatalf("facet bucket = %#v", result.Facets[0].Buckets)
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

func TestSiteResultFromPagePreservesFlatMixedContentTypes(t *testing.T) {
	page := types.SearchResultPage{
		Page:    1,
		PerPage: 10,
		Total:   2,
		Hits: []types.SearchHit{
			{
				ID:      "document-1",
				Type:    types.DocumentTypeDocument,
				Title:   "Search Rollout Workbook",
				Summary: "Planning workbook",
				URL:     "/documents/1",
				Locale:  "en",
				Fields: map[string]any{
					"entity_type": "document",
					"format":      "Workbook",
				},
			},
			{
				ID:      "blog-1",
				Type:    types.DocumentTypeBlogArticle,
				Title:   "Search Notes",
				Summary: "Blog summary",
				URL:     "/blog/1",
				Locale:  "en",
				Fields: map[string]any{
					"entity_type": "blog_article",
					"format":      "Blog",
				},
			},
		},
	}
	result := SiteResultFromPage(page)
	if len(result.Hits) != 2 {
		t.Fatalf("hits = %#v", result.Hits)
	}
	if result.Hits[0].Type != types.DocumentTypeDocument || result.Hits[1].Type != types.DocumentTypeBlogArticle {
		t.Fatalf("unexpected mixed content types: %#v", result.Hits)
	}
	if result.Hits[0].ParentID != "" || result.Hits[1].ParentID != "" {
		t.Fatalf("expected flat whole-entity hits without parent metadata, got %#v", result.Hits)
	}
}
