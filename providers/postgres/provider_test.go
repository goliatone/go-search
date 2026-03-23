package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/goliatone/go-search/adapters/content"
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers"
	"github.com/goliatone/go-search/query"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

func TestNewRequiresDB(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatalf("expected configuration error")
	}
}

func TestCapabilitiesExposePostgresMetadata(t *testing.T) {
	db := bun.NewDB(&sql.DB{}, pgdialect.New())
	provider, err := New(Config{DB: db})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	caps, err := provider.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !caps.TypoTolerance || caps.Metadata["shared_table_v1"] != true {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
}

func TestSearchConfigNormalizationFallsBackSafely(t *testing.T) {
	if got := normalizeSearchConfig("English", "simple"); got != "english" {
		t.Fatalf("normalizeSearchConfig = %q", got)
	}
	if got := normalizeSearchConfig("english;drop table", "simple"); got != "simple" {
		t.Fatalf("unsafe search config fallback = %q", got)
	}
	doc := types.Document{Metadata: map[string]any{"locale_analyzer": "spanish"}}
	if got := resolveDocumentSearchConfig(doc, "simple"); got != "spanish" {
		t.Fatalf("resolveDocumentSearchConfig = %q", got)
	}
}

func TestPostgresProviderContractSuite(t *testing.T) {
	providers.RunContractSuite(t, func(t *testing.T) providers.Provider {
		t.Helper()
		return newIntegrationProvider(t)
	})
}

func newIntegrationProvider(t *testing.T) providers.Provider {
	t.Helper()
	dsn := testkit.Integration.Postgres.DSN
	if dsn == "" {
		t.Skip("testkit.Integration.Postgres.DSN is not set")
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := bun.NewDB(sqlDB, pgdialect.New())
	if err := Migrations().Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate postgres provider: %v", err)
	}
	if _, err := db.NewRaw("TRUNCATE TABLE search_documents").Exec(context.Background()); err != nil {
		t.Fatalf("truncate search_documents: %v", err)
	}
	provider, err := New(Config{DB: db})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return provider
}

func newIntegrationPostgresProvider(t *testing.T) *Provider {
	t.Helper()
	provider, ok := newIntegrationProvider(t).(*Provider)
	if !ok {
		t.Fatalf("expected postgres provider")
	}
	return provider
}

func TestPostgresProviderSupportsArchiveFacetsAndRangeFiltering(t *testing.T) {
	provider, ok := newIntegrationProvider(t).(*Provider)
	if !ok {
		t.Fatalf("expected postgres provider")
	}
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	docs := []types.Document{
		{
			ID:       "segment-architecture-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-architecture",
			SourceID: "track-architecture",
			Title:    "Architecture Walkthrough",
			Body:     "archive architecture prayer",
			URL:      "https://example.org/video-architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title":   "Architecture Walkthrough",
				"parent_url":     "https://example.org/video-architecture",
				"published_year": 2024,
			},
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
			ID:       "segment-tara-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-tara",
			SourceID: "track-tara",
			Title:    "Tara Teachings",
			Body:     "archive tara prayer",
			URL:      "https://example.org/video-tara",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title":   "Tara Teachings",
				"parent_url":     "https://example.org/video-tara",
				"published_year": 2024,
			},
			Facets: map[string][]string{
				"topic_hierarchy": {"Teaching Topics", "Teaching Topics > Tara"},
				"format":          {"Teaching"},
			},
			Numeric: map[string]float64{
				"published_year":   2024,
				"duration_seconds": 2400,
			},
		},
	}
	if err := provider.UpsertDocuments(ctx, "media", docs); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "archive",
		Locale:  "en",
		GroupBy: "parent_id",
		Page:    1,
		PerPage: 10,
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.TermExpr{Field: "topic_hierarchy", Op: types.FilterOpEQ, Value: "Teaching Topics > Architecture"},
			types.RangeExpr{Field: "published_year", GTE: 2024},
			types.RangeExpr{Field: "duration_seconds", GTE: 1800},
		}},
		Facets: []types.FacetRequest{
			{Field: "topic_hierarchy", Kind: types.FacetKindHierarchical, Disjunctive: true},
			{Field: "format", Disjunctive: true},
		},
		Sort: []types.Sort{{Field: "published_year", Direction: types.SortDesc}},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Groups) != 1 {
		t.Fatalf("expected one grouped result, got %+v", page)
	}
	foundSibling := false
	for _, facet := range page.Facets {
		if facet.Field != "topic_hierarchy" {
			continue
		}
		for _, value := range facet.Values {
			if value.Value == "Teaching Topics > Tara" {
				foundSibling = true
			}
		}
	}
	if !foundSibling {
		t.Fatalf("expected sibling hierarchy facet, got %+v", page.Facets)
	}
}

func TestPostgresProviderHealthIncludesEmptyEnsuredIndex(t *testing.T) {
	provider, ok := newIntegrationProvider(t).(*Provider)
	if !ok {
		t.Fatalf("expected postgres provider")
	}
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "empty"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	health, err := provider.Health(ctx, types.HealthRequest{Indexes: []string{"empty"}})
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if len(health.Indexes) != 1 {
		t.Fatalf("expected one health index, got %+v", health.Indexes)
	}
	if !health.Indexes[0].Ready || health.Indexes[0].Documents != 0 {
		t.Fatalf("expected ready empty index health, got %+v", health.Indexes[0])
	}
}

func TestPostgresProviderSchemaBootstrapRetriesAfterFailure(t *testing.T) {
	provider, ok := newIntegrationProvider(t).(*Provider)
	if !ok {
		t.Fatalf("expected postgres provider")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.EnsureIndex(cancelled, types.IndexDefinition{Name: "retry_media"}); err == nil {
		t.Fatalf("expected cancelled ensure index to fail")
	}
	if err := provider.EnsureIndex(context.Background(), types.IndexDefinition{Name: "retry_media"}); err != nil {
		t.Fatalf("expected ensure index retry to succeed, got %v", err)
	}
}

func TestPostgresProviderUsesConfiguredTextSearchConfig(t *testing.T) {
	dsn := testkit.Integration.Postgres.DSN
	if dsn == "" {
		t.Skip("testkit.Integration.Postgres.DSN is not set")
	}
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db := bun.NewDB(sqlDB, pgdialect.New())
	if err := Migrations().Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate postgres provider: %v", err)
	}
	if _, err := db.NewRaw("TRUNCATE TABLE search_documents").Exec(context.Background()); err != nil {
		t.Fatalf("truncate search_documents: %v", err)
	}
	provider, err := New(Config{DB: db, SearchConfig: "english"})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	ctx := context.Background()
	if err := provider.EnsureIndex(ctx, types.IndexDefinition{Name: "english_media"}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "english_media", []types.Document{
		{ID: "run-1", Type: types.DocumentTypeDocument, Title: "Runner", Body: "I was running daily", Locale: "en"},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"english_media"},
		Query:   "run",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total == 0 {
		t.Fatalf("expected english stemming to match running with query run")
	}
}

func TestPostgresProviderFacetsUseTheSameQueryCandidateSetAsHits(t *testing.T) {
	provider := newIntegrationPostgresProvider(t)
	ctx := context.Background()
	def := types.IndexDefinition{Name: "english_media"}
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, def.Name, []types.Document{{
		ID:     "run-1",
		Type:   types.DocumentTypeDocument,
		Title:  "Runner",
		Body:   "I was running daily",
		Locale: "en",
		Facets: map[string][]string{"topic": {"movement"}},
		Metadata: map[string]any{
			"locale_analyzer": "english",
		},
	}}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{def.Name},
		Query:   "run",
		Page:    1,
		PerPage: 10,
		Facets:  []types.FacetRequest{{Field: "topic", Disjunctive: true}},
		Metadata: map[string]any{
			"search_config": "english",
		},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected hit, got %+v", page)
	}
	if len(page.Facets) != 1 || len(page.Facets[0].Values) != 1 || page.Facets[0].Values[0].Value != "movement" {
		t.Fatalf("expected facet counts to include the matched stemmed document, got %+v", page.Facets)
	}
}

func TestPostgresProviderScopesSharedIndexReplaceAndDeleteByRegistrationKey(t *testing.T) {
	provider := newIntegrationPostgresProvider(t)
	ctx := context.Background()
	def := content.DefaultIndexDefinition("content_shared")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	videoDocs := []types.Document{{
		ID:              "shared-1",
		RegistrationKey: "video",
		Type:            types.DocumentTypeVideo,
		SourceType:      "video",
		SourceID:        "shared-1",
		Title:           "Shared Video",
		Body:            "architecture video",
		URL:             "/videos/shared-1",
		Locale:          "en",
	}}
	documentDocs := []types.Document{{
		ID:              "shared-1",
		RegistrationKey: "document",
		Type:            types.DocumentTypeDocument,
		SourceType:      "document",
		SourceID:        "shared-1",
		Title:           "Shared Document",
		Body:            "architecture workbook",
		URL:             "/documents/shared-1",
		Locale:          "en",
	}}
	if err := provider.ReplaceDocuments(ctx, def.Name, "video", []string{"shared-1"}, videoDocs); err != nil {
		t.Fatalf("replace video docs: %v", err)
	}
	if err := provider.ReplaceDocuments(ctx, def.Name, "document", []string{"shared-1"}, documentDocs); err != nil {
		t.Fatalf("replace document docs: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{def.Name},
		Query:   "architecture",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected both registrations to coexist for shared upstream id, got %+v", page.Hits)
	}
	if err := provider.DeleteBySource(ctx, def.Name, "video", []string{"shared-1"}); err != nil {
		t.Fatalf("delete video docs: %v", err)
	}
	page, err = provider.Search(ctx, types.SearchRequest{
		Indexes: []string{def.Name},
		Query:   "architecture",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if page.Total != 1 || len(page.Hits) != 1 || page.Hits[0].Type != types.DocumentTypeDocument {
		t.Fatalf("expected document registration to remain after scoped delete, got %+v", page.Hits)
	}
}

func TestPostgresProviderSupportsFlatHeterogeneousSearchAcrossSharedAndSplitIndexes(t *testing.T) {
	provider, ok := newIntegrationProvider(t).(*Provider)
	if !ok {
		t.Fatalf("expected postgres provider")
	}
	ctx := context.Background()
	for _, def := range []types.IndexDefinition{
		{Name: "content_shared"},
		{Name: "videos"},
		{Name: "documents"},
		{Name: "blog_articles"},
	} {
		if err := provider.EnsureIndex(ctx, def); err != nil {
			t.Fatalf("ensure index %s: %v", def.Name, err)
		}
	}
	sharedDocs := []types.Document{
		{ID: "video-1", Index: "content_shared", Type: types.DocumentTypeVideo, Title: "Search Architecture Walkthrough", Body: "search architecture video", URL: "/videos/1", Locale: "en", SourceType: "video", SourceID: "video-1"},
		{ID: "document-1", Index: "content_shared", Type: types.DocumentTypeDocument, Title: "Search Rollout Workbook", Body: "search rollout document", URL: "/documents/1", Locale: "en", SourceType: "document", SourceID: "document-1"},
		{ID: "blog-1", Index: "content_shared", Type: types.DocumentTypeBlogArticle, Title: "Search Notes", Body: "search notes article", URL: "/blog/1", Locale: "en", SourceType: "blog_article", SourceID: "blog-1"},
	}
	if err := provider.UpsertDocuments(ctx, "content_shared", sharedDocs); err != nil {
		t.Fatalf("upsert shared docs: %v", err)
	}
	for _, item := range []struct {
		index string
		doc   types.Document
	}{
		{index: "videos", doc: types.Document{ID: "video-split", Type: types.DocumentTypeVideo, Title: "Search Split Video", Body: "search split video", URL: "/videos/split", Locale: "en"}},
		{index: "documents", doc: types.Document{ID: "document-split", Type: types.DocumentTypeDocument, Title: "Search Split Document", Body: "search split document", URL: "/documents/split", Locale: "en"}},
		{index: "blog_articles", doc: types.Document{ID: "blog-split", Type: types.DocumentTypeBlogArticle, Title: "Search Split Blog", Body: "search split article", URL: "/blog/split", Locale: "en"}},
	} {
		if err := provider.UpsertDocuments(ctx, item.index, []types.Document{item.doc}); err != nil {
			t.Fatalf("upsert %s doc: %v", item.index, err)
		}
	}

	sharedPage, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"content_shared"},
		Query:   "search",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("shared search: %v", err)
	}
	sharedFound := map[string]bool{}
	for _, hit := range sharedPage.Hits {
		sharedFound[hit.Type] = true
	}
	for _, typ := range []string{types.DocumentTypeVideo, types.DocumentTypeDocument, types.DocumentTypeBlogArticle} {
		if !sharedFound[typ] {
			t.Fatalf("expected shared hit type %q in %+v", typ, sharedPage.Hits)
		}
	}

	splitPage, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"videos", "documents", "blog_articles"},
		Query:   "search",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("split search: %v", err)
	}
	splitFound := map[string]bool{}
	for _, hit := range splitPage.Hits {
		splitFound[hit.Type] = true
	}
	for _, typ := range []string{types.DocumentTypeVideo, types.DocumentTypeDocument, types.DocumentTypeBlogArticle} {
		if !splitFound[typ] {
			t.Fatalf("expected split hit type %q in %+v", typ, splitPage.Hits)
		}
	}
}

func TestPostgresProviderSupportsArchiveMultiSelectFacetRefinement(t *testing.T) {
	provider := newIntegrationPostgresProvider(t)
	ctx := context.Background()
	def := media.DefaultArchiveIndexDefinition("media")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", testkit.ArchiveFacetDocuments("media")); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}

	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "archive",
		Locale:  "en",
		GroupBy: "parent_id",
		Page:    1,
		PerPage: 10,
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.TermExpr{Field: media.FacetFieldTopicHierarchy, Op: types.FilterOpIn, Value: []string{
				"Teaching Topics > Architecture",
				"Teaching Topics > Tara",
			}},
			types.TermExpr{Field: media.FacetFieldFormat, Op: types.FilterOpEQ, Value: "Teaching"},
		}},
		Facets: []types.FacetRequest{
			{Field: media.FacetFieldTopicHierarchy, Kind: types.FacetKindHierarchical, Disjunctive: true},
			{Field: media.FacetFieldFormat, Disjunctive: true},
		},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 2 || len(page.Groups) != 2 {
		t.Fatalf("expected two grouped archive results, got %+v", page)
	}
	selected := map[string]bool{}
	foundSibling := false
	for _, facet := range page.Facets {
		if facet.Field != media.FacetFieldTopicHierarchy {
			continue
		}
		for _, value := range facet.Values {
			if value.Selected {
				selected[value.Value] = true
			}
			if value.Value == "Teaching Topics > Ranking" {
				foundSibling = true
			}
		}
	}
	if !selected["Teaching Topics > Architecture"] || !selected["Teaching Topics > Tara"] {
		t.Fatalf("expected selected multi-value hierarchy state in %+v", page.Facets)
	}
	if !foundSibling {
		t.Fatalf("expected disjunctive sibling hierarchy value in %+v", page.Facets)
	}
}

func TestPostgresProviderSupportsArchiveSortRangeAndGroupedPaginationParity(t *testing.T) {
	provider := newIntegrationPostgresProvider(t)
	ctx := context.Background()
	def := media.DefaultArchiveIndexDefinition("media")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", testkit.ArchiveFacetDocuments("media")); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}

	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "archive",
		Locale:  "en",
		GroupBy: "parent_id",
		Page:    1,
		PerPage: 1,
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.RangeExpr{Field: media.FieldPublishedYear, GTE: 2024},
			types.RangeExpr{Field: media.FieldDurationSeconds, GTE: 1800},
			types.TermExpr{Field: media.FacetFieldLocation, Op: types.FilterOpEQ, Value: "Mexico City"},
			types.TermExpr{Field: media.FacetFieldSangha, Op: types.FilterOpEQ, Value: "Cloud Sangha"},
		}},
		Facets: []types.FacetRequest{
			{Field: media.FacetFieldTopicHierarchy, Kind: types.FacetKindHierarchical, Disjunctive: true},
			{Field: media.FacetFieldLocation, Disjunctive: true},
			{Field: media.FacetFieldSangha, Disjunctive: true},
		},
		Sort: []types.Sort{{Field: media.FieldPublishedYear, Direction: types.SortDesc}},
	})
	if err != nil {
		t.Fatalf("search page 1: %v", err)
	}
	if page.Total != 2 || len(page.Groups) != 1 {
		t.Fatalf("expected grouped total by parent with one page item, got %+v", page)
	}
	if page.Groups[0].Parent == nil || page.Groups[0].Parent.ID != "video-architecture" {
		t.Fatalf("expected newest grouped result first, got %+v", page.Groups[0].Parent)
	}
	foundSibling := false
	for _, facet := range page.Facets {
		if facet.Field != media.FacetFieldTopicHierarchy {
			continue
		}
		for _, value := range facet.Values {
			if value.Value == "Teaching Topics > Ranking" {
				foundSibling = true
			}
		}
	}
	if !foundSibling {
		t.Fatalf("expected disjunctive sibling hierarchy value in %+v", page.Facets)
	}

	page2, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "archive",
		Locale:  "en",
		GroupBy: "parent_id",
		Page:    2,
		PerPage: 1,
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.RangeExpr{Field: media.FieldPublishedYear, GTE: 2024},
			types.RangeExpr{Field: media.FieldDurationSeconds, GTE: 1800},
			types.TermExpr{Field: media.FacetFieldLocation, Op: types.FilterOpEQ, Value: "Mexico City"},
			types.TermExpr{Field: media.FacetFieldSangha, Op: types.FilterOpEQ, Value: "Cloud Sangha"},
		}},
		Sort: []types.Sort{{Field: media.FieldPublishedYear, Direction: types.SortDesc}},
	})
	if err != nil {
		t.Fatalf("search page 2: %v", err)
	}
	if page2.Total != 2 || len(page2.Groups) != 1 {
		t.Fatalf("expected second grouped page item, got %+v", page2)
	}
	if page2.Groups[0].Parent == nil || page2.Groups[0].Parent.ID != "video-tara" {
		t.Fatalf("expected second grouped result, got %+v", page2.Groups[0].Parent)
	}
}

func TestPostgresProviderSupportsFlatHeterogeneousSearchAcrossSeparateIndexes(t *testing.T) {
	provider := newIntegrationPostgresProvider(t)
	ctx := context.Background()
	for _, def := range []types.IndexDefinition{
		content.DefaultIndexDefinition("videos"),
		content.DefaultIndexDefinition("documents"),
		content.DefaultIndexDefinition("blog_articles"),
	} {
		if err := provider.EnsureIndex(ctx, def); err != nil {
			t.Fatalf("ensure index %s: %v", def.Name, err)
		}
	}
	for _, item := range testkit.HeterogeneousDocuments() {
		if err := provider.UpsertDocuments(ctx, item.Index, item.Docs); err != nil {
			t.Fatalf("upsert %s docs: %v", item.Index, err)
		}
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
	found := map[string]bool{}
	for _, hit := range page.Hits {
		found[hit.Type] = true
	}
	for _, typ := range []string{types.DocumentTypeVideo, types.DocumentTypeDocument, types.DocumentTypeBlogArticle} {
		if !found[typ] {
			t.Fatalf("expected hit type %q in %+v", typ, page.Hits)
		}
	}
}

func TestPostgresProviderSupportsSharedIndexMultiRegistrationReindex(t *testing.T) {
	provider := newIntegrationPostgresProvider(t)
	ctx := context.Background()
	def := content.DefaultIndexDefinition("content_shared")
	registry := indexing.NewRegistry()
	for _, record := range testkit.SharedIndexContentRecords() {
		record := record
		if err := registry.Register(def, indexing.NewRegistrationWithKey(
			def.Name,
			def,
			record.Type,
			record.Type,
			content.NewSource([]content.Record{record}),
			content.NewProjector(content.ProjectorConfig{Index: def.Name, SourceType: record.Type}),
			func(value content.Record) string { return value.ID },
		)); err != nil {
			t.Fatalf("register %s: %v", record.Type, err)
		}
	}
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	indexer, err := indexing.NewIndexer(indexing.IndexerConfig{Registry: registry, Provider: provider})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}
	if err := indexer.ReindexIndex(ctx, def.Name, "", 10); err != nil {
		t.Fatalf("reindex shared index: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{def.Name},
		Query:   "search",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
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
}

func TestPostgresProviderRejectsGroupedMixedIndexRequestsBeforeExecution(t *testing.T) {
	provider := newIntegrationPostgresProvider(t)
	ctx := context.Background()
	mediaDef := media.DefaultArchiveIndexDefinition("media")
	documentDef := content.DefaultIndexDefinition("documents")
	for _, def := range []types.IndexDefinition{mediaDef, documentDef} {
		if err := provider.EnsureIndex(ctx, def); err != nil {
			t.Fatalf("ensure index %s: %v", def.Name, err)
		}
	}
	registry := indexing.NewRegistry()
	if err := registry.Register(mediaDef, nil); err != nil {
		t.Fatalf("register media: %v", err)
	}
	if err := registry.Register(documentDef, nil); err != nil {
		t.Fatalf("register documents: %v", err)
	}
	pln, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	searchQuery, err := query.NewSearch(query.SearchConfig{Planner: pln, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	if _, err := searchQuery.Query(ctx, types.SearchRequest{
		Indexes: []string{"media", "documents"},
		Query:   "search",
		GroupBy: "parent_id",
	}); err == nil {
		t.Fatalf("expected grouped mixed-index request to fail")
	}
}
