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
	if status.Documents != 45 {
		t.Fatalf("expected 45 seeded documents across media and content indexes, got %d", status.Documents)
	}
	if len(runtime.IndexNames()) != 5 {
		t.Fatalf("expected five managed indexes, got %v", runtime.IndexNames())
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
	if suggest.Items[0].Type == "" {
		t.Fatalf("expected typed suggestions, got %+v", suggest.Items[0])
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

func TestRuntimeBindSuggestRequestUsesAcceptLanguageTransportPath(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	bound := runtime.BindSuggestRequest(SuggestRequest{
		Query:          "search",
		AcceptLanguage: "fr-CA,fr;q=0.9,en;q=0.8",
		Limit:          5,
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

	result, err := runtime.Suggest(context.Background(), bound)
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatalf("expected suggestions, got none")
	}
	if result.Items[0].Locale != "en" {
		t.Fatalf("expected canonical locale suggestion, got %+v", result.Items[0])
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
	foundDocument := false
	foundBlog := false
	for _, hit := range result.Hits {
		switch hit.Type {
		case "document":
			foundDocument = true
		case "blog_article":
			foundBlog = true
		}
	}
	if !foundDocument || !foundBlog {
		t.Fatalf("expected mixed whole-entity content hits, got %+v", result.Hits)
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

func TestRuntimeSeedDataCoversArchiveFacetFamilies(t *testing.T) {
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
		Group:  true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	required := map[string]bool{
		media.FacetFieldTopicHierarchy:    false,
		media.FacetFieldCategoryHierarchy: false,
		media.FacetFieldPeople:            false,
		media.FacetFieldSubject:           false,
		media.FacetFieldText:              false,
		media.FacetFieldDeity:             false,
		media.FacetFieldLocale:            false,
		media.FacetFieldDecade:            false,
		media.FacetFieldDurationBucket:    false,
		media.FacetFieldLocation:          false,
		media.FacetFieldSangha:            false,
		media.FacetFieldFormat:            false,
		media.FacetFieldSeries:            false,
	}
	for _, facet := range result.Facets {
		if _, ok := required[facet.Field]; !ok {
			continue
		}
		if len(facet.Values) > 0 {
			required[facet.Field] = true
		}
	}
	for field, ok := range required {
		if !ok {
			t.Fatalf("expected populated facet %q in %+v", field, result.Facets)
		}
	}
}

func TestRuntimeSearchSupportsCrossFacetCombinationNarrowing(t *testing.T) {
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
		Locale:      "en",
		LandingSlug: "architecture",
		Group:       true,
		FacetFilters: map[string][]string{
			media.FacetFieldCategoryHierarchy: {"Teaching Categories > Workshop"},
			media.FacetFieldPeople:            {"Archive Research Team"},
			media.FacetFieldLocation:          {"Mexico City"},
			media.FacetFieldFormat:            {"Workshop"},
			media.FacetFieldSeries:            {"Search Case Studies"},
		},
		PublishedYearGTE:   intPtr(2025),
		DurationSecondsGTE: intPtr(1800),
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 1 || len(result.Groups) != 1 {
		t.Fatalf("expected one architecture result after cross-facet narrowing, got total=%d groups=%d", result.Total, len(result.Groups))
	}
	if result.Groups[0].Parent == nil || result.Groups[0].Parent.Title != "Architecture Case Studies" {
		t.Fatalf("unexpected grouped result: %+v", result.Groups[0].Parent)
	}
}

func TestRuntimeContentSharedAndSplitSurfacesReturnEquivalentEntityTypes(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	shared, err := runtime.Search(context.Background(), SearchRequest{
		Locale:  "",
		Surface: SurfaceContentShared,
		Query:   "search",
	})
	if err != nil {
		t.Fatalf("search shared surface: %v", err)
	}
	split, err := runtime.Search(context.Background(), SearchRequest{
		Locale:  "",
		Surface: SurfaceContentSplit,
		Query:   "search",
	})
	if err != nil {
		t.Fatalf("search split surface: %v", err)
	}
	if shared.Total == 0 || split.Total == 0 {
		t.Fatalf("expected results on both content surfaces, shared=%d split=%d", shared.Total, split.Total)
	}
	sharedTypes := map[string]bool{}
	for _, hit := range shared.Hits {
		sharedTypes[hit.Type] = true
	}
	splitTypes := map[string]bool{}
	for _, hit := range split.Hits {
		splitTypes[hit.Type] = true
	}
	for _, typ := range []string{"video", "document", "blog_article"} {
		if !sharedTypes[typ] {
			t.Fatalf("expected shared surface to include %s hits, got %+v", typ, shared.Hits)
		}
		if !splitTypes[typ] {
			t.Fatalf("expected split surface to include %s hits, got %+v", typ, split.Hits)
		}
	}
}
