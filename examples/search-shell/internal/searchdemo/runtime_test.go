package searchdemo

import (
	"context"
	"strings"
	"testing"

	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
)

func intPtr(value int) *int { return &value }

func hasHitType(result types.SearchResultPage, typ string) bool {
	for _, hit := range result.Hits {
		if hit.Type == typ {
			return true
		}
	}
	return false
}

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
	if status.Documents <= 45 {
		t.Fatalf("expected cms-backed fixtures to increase seeded document count beyond 45, got %d", status.Documents)
	}
	if len(runtime.IndexNames()) != 7 {
		t.Fatalf("expected seven managed indexes with users coverage, got %v", runtime.IndexNames())
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

func TestRuntimeUsersSurfaceFiltersByScopeAndSupportVisibility(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	tenantAResult, err := runtime.Search(context.Background(), SearchRequest{
		Query:       "admin",
		Surface:     SurfaceUsers,
		TenantID:    "00000000-0000-0000-0000-000000000101",
		OrgID:       "00000000-0000-0000-0000-000000000201",
		ActorUserID: "00000000-0000-0000-0000-000000001001",
		ActorRole:   "admin",
	})
	if err != nil {
		t.Fatalf("tenant search: %v", err)
	}
	if tenantAResult.Total == 0 {
		t.Fatalf("expected tenant scoped user results")
	}
	for _, hit := range tenantAResult.Hits {
		if hit.Document == nil || hit.Document.Scope.TenantID != "00000000-0000-0000-0000-000000000101" {
			t.Fatalf("unexpected tenant scoped hit %+v", hit)
		}
	}

	supportResult, err := runtime.Search(context.Background(), SearchRequest{
		Surface:     SurfaceUsers,
		ActorRole:   "support",
		ActorUserID: "00000000-0000-0000-0000-000000001002",
		TenantID:    "00000000-0000-0000-0000-000000000101",
		OrgID:       "00000000-0000-0000-0000-000000000201",
	})
	if err != nil {
		t.Fatalf("support search: %v", err)
	}
	if supportResult.Total != 1 || len(supportResult.Hits) != 1 {
		t.Fatalf("expected support actor to see only self, got %+v", supportResult.Hits)
	}
	if supportResult.Hits[0].ID != "00000000-0000-0000-0000-000000001002" {
		t.Fatalf("unexpected support hit %+v", supportResult.Hits[0])
	}
}

func TestRuntimeUserLifecycleDeletesArchivedUserFromSearch(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	if err := runtime.TransitionUser(context.Background(), "00000000-0000-0000-0000-000000001004", userstypes.LifecycleStateArchived); err != nil {
		t.Fatalf("archive user: %v", err)
	}

	result, err := runtime.Search(context.Background(), SearchRequest{
		Query:   "editor",
		Surface: SurfaceUsers,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, hit := range result.Hits {
		if hit.ID == "00000000-0000-0000-0000-000000001004" {
			t.Fatalf("archived user should be removed from search %+v", hit)
		}
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

func TestRuntimeSearchRespectsUngroupedMediaSurface(t *testing.T) {
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
		Query:   "transcript",
		Surface: SurfaceMediaGrouped,
		Group:   false,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Groups) != 0 {
		t.Fatalf("expected explicit media surface to respect group=false, got groups: %+v", result.Groups)
	}
	if len(result.Hits) == 0 {
		t.Fatalf("expected transcript hits, got none")
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

func TestRuntimeCMSLifecyclePagePublishUnpublishUpdatesSharedAndSplit(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	fixture := runtime.cmsFixture
	if fixture == nil || runtime.cmsModule == nil {
		t.Fatalf("expected cms fixture runtime wiring")
	}

	assertPageVisible := func(surface string, want bool) {
		t.Helper()
		result, err := runtime.Search(context.Background(), SearchRequest{
			Query:   fixture.pageQuery,
			Locale:  fixture.defaultLocale,
			Surface: surface,
			PerPage: 50,
		})
		if err != nil {
			t.Fatalf("search %s: %v", surface, err)
		}
		if got := hasHitType(result, "page"); got != want {
			t.Fatalf("surface %s page visibility = %v, want %v; hits=%+v", surface, got, want, result.Hits)
		}
	}

	assertPageVisible(SurfaceContentShared, true)
	assertPageVisible(SurfaceContentSplit, true)

	if _, err := runtime.cmsModule.Pages().Update(context.Background(), cmspages.UpdatePageRequest{
		ID:                       fixture.pageID,
		Status:                   "draft",
		UpdatedBy:                fixture.actorID,
		AllowMissingTranslations: true,
	}); err != nil {
		t.Fatalf("unpublish page: %v", err)
	}

	assertPageVisible(SurfaceContentShared, false)
	assertPageVisible(SurfaceContentSplit, false)

	if _, err := runtime.cmsModule.Pages().Update(context.Background(), cmspages.UpdatePageRequest{
		ID:                       fixture.pageID,
		Status:                   "published",
		UpdatedBy:                fixture.actorID,
		AllowMissingTranslations: true,
	}); err != nil {
		t.Fatalf("publish page: %v", err)
	}

	assertPageVisible(SurfaceContentShared, true)
	assertPageVisible(SurfaceContentSplit, true)
}

func TestRuntimeCMSLifecycleTranslationAndDeleteFlowsUpdateSharedAndSplit(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	fixture := runtime.cmsFixture
	if fixture == nil || runtime.cmsModule == nil {
		t.Fatalf("expected cms fixture runtime wiring")
	}

	assertHitType := func(surface, locale, query, typ string, want bool) {
		t.Helper()
		result, err := runtime.Search(context.Background(), SearchRequest{
			Query:   query,
			Locale:  locale,
			Surface: surface,
			PerPage: 50,
		})
		if err != nil {
			t.Fatalf("search %s/%s/%s: %v", surface, locale, query, err)
		}
		if got := hasHitType(result, typ); got != want {
			t.Fatalf("surface %s locale %s query %s %s visibility=%v want=%v hits=%+v", surface, locale, query, typ, got, want, result.Hits)
		}
	}

	assertHitType(SurfaceContentShared, fixture.secondaryLocale, fixture.blogQuery, "blog_article", true)
	assertHitType(SurfaceContentSplit, fixture.secondaryLocale, fixture.blogQuery, "blog_article", true)

	if _, err := runtime.cmsModule.Content().UpdateTranslation(context.Background(), cmscontent.UpdateContentTranslationRequest{
		ContentID: fixture.blogID,
		Locale:    fixture.secondaryLocale,
		Title:     "Notas actualizadas de busqueda",
		Content: map[string]any{
			"headline": fixture.blogUpdatedESQuery,
			"body":     "Actualizacion de traduccion para verificar reindexado en busqueda.",
		},
		UpdatedBy: fixture.actorID,
	}); err != nil {
		t.Fatalf("update blog translation: %v", err)
	}

	assertHitType(SurfaceContentShared, fixture.secondaryLocale, fixture.blogUpdatedESQuery, "blog_article", true)
	assertHitType(SurfaceContentSplit, fixture.secondaryLocale, fixture.blogUpdatedESQuery, "blog_article", true)

	if err := runtime.cmsModule.Content().DeleteTranslation(context.Background(), cmscontent.DeleteContentTranslationRequest{
		ContentID: fixture.documentID,
		Locale:    fixture.secondaryLocale,
		DeletedBy: fixture.actorID,
	}); err != nil {
		t.Fatalf("delete document translation: %v", err)
	}

	assertHitType(SurfaceContentShared, fixture.secondaryLocale, fixture.documentDeletedESQuery, "document", false)
	assertHitType(SurfaceContentSplit, fixture.secondaryLocale, fixture.documentDeletedESQuery, "document", false)
	assertHitType(SurfaceContentShared, fixture.defaultLocale, fixture.documentQuery, "document", true)
	assertHitType(SurfaceContentSplit, fixture.defaultLocale, fixture.documentQuery, "document", true)

	if err := runtime.cmsModule.Content().Delete(context.Background(), cmscontent.DeleteContentRequest{
		ID:         fixture.blogID,
		DeletedBy:  fixture.actorID,
		HardDelete: true,
	}); err != nil {
		t.Fatalf("delete blog content: %v", err)
	}

	assertHitType(SurfaceContentShared, fixture.defaultLocale, fixture.blogQuery, "blog_article", false)
	assertHitType(SurfaceContentSplit, fixture.defaultLocale, fixture.blogQuery, "blog_article", false)
	assertHitType(SurfaceContentShared, fixture.secondaryLocale, fixture.blogUpdatedESQuery, "blog_article", false)
	assertHitType(SurfaceContentSplit, fixture.secondaryLocale, fixture.blogUpdatedESQuery, "blog_article", false)
}

func TestRuntimeStatusReportsCacheAndSmokeFlows(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		CacheEnabled:  true,
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
	if !status.CacheEnabled {
		t.Fatalf("expected cache to be enabled in status")
	}
	if !status.CacheWrappers.Search || !status.CacheWrappers.Suggest || !status.CacheWrappers.ProviderMetadata {
		t.Fatalf("expected cache wrapper status, got %+v", status.CacheWrappers)
	}
	if status.GenerationBackend != generationBackendMemory {
		t.Fatalf("generation backend = %q", status.GenerationBackend)
	}
	if len(status.SmokeFlows) < 4 {
		t.Fatalf("expected smoke flows in status, got %+v", status.SmokeFlows)
	}
}

func TestRuntimeCacheDisabledBypassesSearchAndSuggestCaches(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		CacheEnabled:  true,
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	ctx := context.Background()
	searchReq := SearchRequest{Query: "search", Locale: "en"}
	if _, err := runtime.Search(ctx, searchReq); err != nil {
		t.Fatalf("search 1: %v", err)
	}
	if _, err := runtime.Search(ctx, searchReq); err != nil {
		t.Fatalf("search 2: %v", err)
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status after cached search: %v", err)
	}
	searchHits := status.Cache.Search.Hits
	if searchHits == 0 {
		t.Fatalf("expected cached search hit, got %+v", status.Cache.Search)
	}
	if _, err := runtime.Search(ctx, SearchRequest{Query: "search", Locale: "en", CacheDisabled: true}); err != nil {
		t.Fatalf("search cache disabled: %v", err)
	}
	afterSearchBypass, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status after uncached search: %v", err)
	}
	if afterSearchBypass.Cache.Search.Hits != searchHits {
		t.Fatalf("expected cache-disabled search to bypass cache hits, before=%d after=%d", searchHits, afterSearchBypass.Cache.Search.Hits)
	}

	suggestReq := SuggestRequest{Query: "search", Locale: "en", Limit: 5}
	if _, err := runtime.Suggest(ctx, suggestReq); err != nil {
		t.Fatalf("suggest 1: %v", err)
	}
	if _, err := runtime.Suggest(ctx, suggestReq); err != nil {
		t.Fatalf("suggest 2: %v", err)
	}
	status, err = runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status after cached suggest: %v", err)
	}
	suggestHits := status.Cache.Suggest.Hits
	if suggestHits == 0 {
		t.Fatalf("expected cached suggest hit, got %+v", status.Cache.Suggest)
	}
	if _, err := runtime.Suggest(ctx, SuggestRequest{Query: "search", Locale: "en", Limit: 5, CacheDisabled: true}); err != nil {
		t.Fatalf("suggest cache disabled: %v", err)
	}
	afterSuggestBypass, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status after uncached suggest: %v", err)
	}
	if afterSuggestBypass.Cache.Suggest.Hits != suggestHits {
		t.Fatalf("expected cache-disabled suggest to bypass cache hits, before=%d after=%d", suggestHits, afterSuggestBypass.Cache.Suggest.Hits)
	}
	if afterSuggestBypass.Metrics.Counts["search.cache.bypass.count"] < 2 {
		t.Fatalf("expected cache bypass metrics for search and suggest, got %+v", afterSuggestBypass.Metrics.Counts)
	}
}

func TestRuntimeLocaleNormalizationReusesSearchCacheKey(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		CacheEnabled:  true,
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	ctx := context.Background()
	reqUpper := SearchRequest{Query: "search", Locale: "EN"}
	reqLower := SearchRequest{Query: "search", Locale: "en"}
	if _, err := runtime.Search(ctx, reqUpper); err != nil {
		t.Fatalf("search upper: %v", err)
	}
	if _, err := runtime.Search(ctx, reqLower); err != nil {
		t.Fatalf("search lower: %v", err)
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Cache.Search.Hits == 0 {
		t.Fatalf("expected locale-normalized search cache hit, got %+v", status.Cache.Search)
	}
}

func TestRuntimeActorSensitiveSearchBypassesCache(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		CacheEnabled:  true,
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	ctx := context.Background()
	req := SearchRequest{
		Query:       "support",
		Surface:     SurfaceUsers,
		ActorRole:   "support",
		ActorUserID: "00000000-0000-0000-0000-000000001002",
		TenantID:    "00000000-0000-0000-0000-000000000101",
		OrgID:       "00000000-0000-0000-0000-000000000201",
	}
	if _, err := runtime.Search(ctx, req); err != nil {
		t.Fatalf("search 1: %v", err)
	}
	if _, err := runtime.Search(ctx, req); err != nil {
		t.Fatalf("search 2: %v", err)
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Cache.Search.Hits != 0 {
		t.Fatalf("expected actor-sensitive requests to bypass search cache, got %+v", status.Cache.Search)
	}
	if status.Metrics.Counts["search.cache.bypass.count"] < 2 {
		t.Fatalf("expected actor-sensitive bypass metrics, got %+v", status.Metrics.Counts)
	}
}

func TestRuntimeUserWriteBumpsGenerationAndInvalidatesSearchCache(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		CacheEnabled:  true,
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	ctx := context.Background()
	req := SearchRequest{Query: "editor", Surface: SurfaceUsers, Locale: "en"}
	if _, err := runtime.Search(ctx, req); err != nil {
		t.Fatalf("search 1: %v", err)
	}
	if _, err := runtime.Search(ctx, req); err != nil {
		t.Fatalf("search 2: %v", err)
	}
	beforeStatus, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status before write: %v", err)
	}
	if beforeStatus.Cache.Search.Hits == 0 {
		t.Fatalf("expected cached search hit before user write, got %+v", beforeStatus.Cache.Search)
	}
	beforeStats, err := runtime.Stats(ctx)
	if err != nil {
		t.Fatalf("stats before write: %v", err)
	}
	beforeGeneration := int64(0)
	for _, item := range beforeStats.Indexes {
		if item.Name == runtime.usersIndex.Name {
			beforeGeneration = item.Generation
			break
		}
	}
	if err := runtime.TransitionUser(ctx, "00000000-0000-0000-0000-000000001004", userstypes.LifecycleStateArchived); err != nil {
		t.Fatalf("archive user: %v", err)
	}
	page, err := runtime.Search(ctx, req)
	if err != nil {
		t.Fatalf("search after write: %v", err)
	}
	for _, hit := range page.Hits {
		if hit.ID == "00000000-0000-0000-0000-000000001004" {
			t.Fatalf("archived user should be removed after cache invalidation %+v", hit)
		}
	}
	afterStats, err := runtime.Stats(ctx)
	if err != nil {
		t.Fatalf("stats after write: %v", err)
	}
	afterGeneration := int64(0)
	for _, item := range afterStats.Indexes {
		if item.Name == runtime.usersIndex.Name {
			afterGeneration = item.Generation
			break
		}
	}
	if afterGeneration <= beforeGeneration {
		t.Fatalf("expected users index generation bump, before=%d after=%d", beforeGeneration, afterGeneration)
	}
	afterStatus, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status after write: %v", err)
	}
	if afterStatus.Cache.Search.Misses <= beforeStatus.Cache.Search.Misses {
		t.Fatalf("expected user write to force cache miss, before=%+v after=%+v", beforeStatus.Cache.Search, afterStatus.Cache.Search)
	}
	if afterStatus.Metrics.Counts["search.cache.stale_generation_fallback.count"] == 0 {
		t.Fatalf("expected stale generation fallback metric after write, got %+v", afterStatus.Metrics.Counts)
	}
}

func TestRuntimeReindexBumpsGenerationAndInvalidatesSearchCache(t *testing.T) {
	runtime, err := New(Config{
		Provider:      "memory",
		CacheEnabled:  true,
		SeedOnStart:   true,
		IndexName:     "media_transcripts",
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}

	ctx := context.Background()
	req := SearchRequest{Query: "search", Locale: "en"}
	if _, err := runtime.Search(ctx, req); err != nil {
		t.Fatalf("search 1: %v", err)
	}
	if _, err := runtime.Search(ctx, req); err != nil {
		t.Fatalf("search 2: %v", err)
	}
	before, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status before reindex: %v", err)
	}
	if before.Cache.Search.Hits == 0 {
		t.Fatalf("expected cache hit before reindex, got %+v", before.Cache.Search)
	}
	if err := runtime.Reindex(ctx, 10); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if _, err := runtime.Search(ctx, req); err != nil {
		t.Fatalf("search after reindex: %v", err)
	}
	after, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("status after reindex: %v", err)
	}
	if after.Generation <= before.Generation {
		t.Fatalf("expected generation bump, before=%d after=%d", before.Generation, after.Generation)
	}
	if after.Cache.Search.Misses <= before.Cache.Search.Misses {
		t.Fatalf("expected reindex to force a fresh cache lookup, before=%+v after=%+v", before.Cache.Search, after.Cache.Search)
	}
	if after.Metrics.Counts["search.cache.stale_generation_fallback.count"] == 0 {
		t.Fatalf("expected stale generation fallback metric after reindex, got %+v", after.Metrics.Counts)
	}
}

func TestRuntimePostgresUsesBunGenerationStore(t *testing.T) {
	if testkit.Integration.Postgres.DSN == "" {
		t.Skip("testkit.Integration.Postgres.DSN is not set")
	}
	runtime, err := New(Config{
		Provider:      "postgres",
		CacheEnabled:  true,
		SeedOnStart:   true,
		IndexName:     "media_transcripts_pg",
		DefaultLocale: "en",
		PostgresDSN:   testkit.Integration.Postgres.DSN,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(func() {
		_ = runtime.Close()
	})

	status, err := runtime.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Provider != "postgres" {
		t.Fatalf("provider = %q", status.Provider)
	}
	if status.GenerationBackend != generationBackendBunPostgres {
		t.Fatalf("generation backend = %q", status.GenerationBackend)
	}
	if !status.CacheEnabled || !status.CacheWrappers.ProviderMetadata {
		t.Fatalf("expected postgres runtime cache wrappers, got %+v", status.CacheWrappers)
	}
	if status.Documents == 0 {
		t.Fatalf("expected seeded postgres demo documents")
	}
}
