package searchdemo

import (
	"context"
	"strings"
	"testing"

	"github.com/goliatone/go-search/adapters/media"
)

func intPtr(value int) *int { return &value }

func TestRuntimeBootstrapsSeededSearch(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	status, err := runtime.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Documents != 16 {
		t.Fatalf("expected 16 seeded transcript documents, got %d", status.Documents)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Query:  "transcript",
		Locale: "en",
		Group:  true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total == 0 {
		t.Fatalf("expected seeded results, got none")
	}

	suggest, err := runtime.Suggest(context.Background(), SuggestRequest{
		Query: "search",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggest.Items) == 0 {
		t.Fatalf("expected suggestions, got none")
	}
}

func TestRuntimeBindsAcceptLanguageAndFallsBackThroughLocalePolicy(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	bound := runtime.BindSearchRequest(SearchRequest{
		Query:          "transcript",
		AcceptLanguage: "fr-CA,fr;q=0.9,en;q=0.8",
	})
	if bound.Locale != "en" {
		t.Fatalf("bound locale = %q", bound.Locale)
	}
	if bound.LocaleSource != "accept_language" {
		t.Fatalf("locale source = %q", bound.LocaleSource)
	}
	if !bound.LocaleSupported {
		t.Fatalf("expected locale to be supported")
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Query:  "transcript",
		Locale: "es-MX",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total == 0 {
		t.Fatalf("expected fallback results for es-MX request")
	}
	if len(result.Hits) == 0 {
		t.Fatalf("expected hits, got none")
	}
	if result.Hits[0].Retrieval == nil || result.Hits[0].Retrieval.Metadata["locale_origin"] != "fallback" {
		t.Fatalf("expected fallback locale annotation, got %+v", result.Hits[0].Retrieval)
	}
}

func TestRuntimeSearchReturnsHierarchicalArchiveFacetsAndLandingPreset(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Query:       "search",
		Locale:      "en",
		LandingSlug: "architecture",
		Group:       true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Facets) == 0 {
		t.Fatalf("expected facets, got none")
	}
	foundHierarchy := false
	for _, facet := range result.Facets {
		if facet.Field != "topic_hierarchy" {
			continue
		}
		foundHierarchy = true
		if facet.Kind != "hierarchical" || !facet.Disjunctive {
			t.Fatalf("unexpected facet metadata: %+v", facet)
		}
		if len(facet.Values) == 0 {
			t.Fatalf("expected topic hierarchy values")
		}
	}
	if !foundHierarchy {
		t.Fatalf("expected topic_hierarchy facet in %+v", result.Facets)
	}
}

func TestRuntimeSearchSupportsMultiTopicFiltering(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Locale: "en",
		Topics: []string{"architecture", "ui"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total == 0 {
		t.Fatalf("expected multi-topic results, got none")
	}
	for _, hit := range result.Hits {
		if hit.Document == nil {
			t.Fatalf("expected hit document, got nil")
		}
		values := hit.Document.Facets["topic"]
		if len(values) == 0 {
			t.Fatalf("expected topic facet on hit %+v", hit)
		}
		topic := strings.ToLower(values[0])
		if topic != "architecture" && topic != "ui" {
			t.Fatalf("unexpected topic %q in hit %+v", values[0], hit)
		}
	}
}

func TestRuntimeSearchSortsByTitle(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Locale:    "en",
		SortField: "title",
		SortDir:   "asc",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Hits) < 2 {
		t.Fatalf("expected multiple hits, got %d", len(result.Hits))
	}
	if result.Hits[0].Title > result.Hits[1].Title {
		t.Fatalf("expected ascending title sort, got %q before %q", result.Hits[0].Title, result.Hits[1].Title)
	}
}

func TestRuntimeSearchCanDisableGrouping(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Locale: "en",
		Group:  false,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Groups) != 0 {
		t.Fatalf("expected individual hits, got groups: %+v", result.Groups)
	}
	if len(result.Hits) < 2 {
		t.Fatalf("expected multiple hits, got %d", len(result.Hits))
	}
}

func TestRuntimeSearchSortsGroupedResultsByPublishedYear(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Locale:    "en",
		Group:     true,
		SortField: media.FieldPublishedYear,
		SortDir:   "desc",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Groups) < 2 {
		t.Fatalf("expected multiple groups, got %d", len(result.Groups))
	}
	if result.Groups[0].Parent == nil || result.Groups[0].Parent.Title != "Architecture Case Studies" {
		t.Fatalf("expected newest grouped result first, got %+v", result.Groups[0].Parent)
	}
}

func TestRuntimeSearchArchitectureFixturesSupportVisibleFacetNarrowing(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	allArchitecture, err := runtime.Search(context.Background(), SearchRequest{
		Locale:      "en",
		LandingSlug: "architecture",
		Group:       true,
	})
	if err != nil {
		t.Fatalf("search all architecture: %v", err)
	}
	if len(allArchitecture.Groups) < 2 {
		t.Fatalf("expected multiple architecture groups, got %+v", allArchitecture.Groups)
	}

	filtered, err := runtime.Search(context.Background(), SearchRequest{
		Locale:      "en",
		LandingSlug: "architecture",
		Group:       true,
		FacetFilters: map[string][]string{
			media.FacetFieldSeries: {"Search Case Studies"},
		},
	})
	if err != nil {
		t.Fatalf("search filtered architecture: %v", err)
	}
	if filtered.Total != 1 || len(filtered.Groups) != 1 {
		t.Fatalf("expected one grouped result after facet narrowing, got total=%d groups=%d", filtered.Total, len(filtered.Groups))
	}
	if filtered.Groups[0].Parent == nil || filtered.Groups[0].Parent.Title != "Architecture Case Studies" {
		t.Fatalf("unexpected filtered group: %+v", filtered.Groups[0].Parent)
	}
}

func TestRuntimeSearchSupportsArchiveRangeFilteringAndBadgeMetadata(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Locale:             "en",
		LandingSlug:        "architecture",
		Group:              true,
		PublishedYearGTE:   intPtr(2024),
		DurationSecondsGTE: intPtr(1800),
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total == 0 || len(result.Groups) == 0 {
		t.Fatalf("expected filtered grouped results, got %+v", result)
	}
	for _, group := range result.Groups {
		if group.Parent == nil {
			t.Fatalf("expected group parent, got %+v", group)
		}
		for _, hit := range group.Hits {
			if hit.Document == nil {
				t.Fatalf("expected document metadata, got %+v", hit)
			}
			if got := hit.Document.Numeric[media.FieldPublishedYear]; got < 2024 {
				t.Fatalf("published year filter not applied: %+v", hit.Document.Numeric)
			}
			if got := hit.Document.Numeric[media.FieldDurationSeconds]; got < 1800 {
				t.Fatalf("duration filter not applied: %+v", hit.Document.Numeric)
			}
			if badge := hit.Fields[media.FieldResultBadge]; badge == nil || badge == "" {
				t.Fatalf("expected result badge metadata on hit %+v", hit.Fields)
			}
		}
	}
	foundSelected := false
	for _, facet := range result.Facets {
		if facet.Field != media.FacetFieldTopicHierarchy {
			continue
		}
		for _, value := range facet.Values {
			if value.Value == "Teaching Topics > Architecture" && value.Selected {
				foundSelected = true
				break
			}
		}
	}
	if !foundSelected {
		t.Fatalf("expected architecture landing preset to mark the hierarchy selection in %+v", result.Facets)
	}
}
