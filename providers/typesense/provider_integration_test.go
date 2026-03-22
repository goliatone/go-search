package typesense

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/goliatone/go-search/adapters/content"
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers"
	"github.com/goliatone/go-search/query"
)

func TestTypesenseProviderContractSuite(t *testing.T) {
	providers.RunContractSuite(t, func(t *testing.T) providers.Provider {
		t.Helper()
		return newIntegrationProvider(t)
	})
}

func TestTypesenseProviderArchiveWorkflow(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := integrationIndexDefinition("media")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	record := testkit.SampleTranscriptRecord()
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:        "media",
		SourceType:   "transcript",
		MergeVersion: "v1",
		MaxChars:     120,
		MaxGapMS:     750,
	})
	docs, err := projector.Project(ctx, record)
	if err != nil {
		t.Fatalf("project transcript: %v", err)
	}
	if err := provider.UpsertDocuments(ctx, "media", docs); err != nil {
		t.Fatalf("upsert transcript docs: %v", err)
	}

	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes:   []string{"media"},
		Query:     "prayer",
		Locale:    "en",
		Page:      1,
		PerPage:   10,
		GroupBy:   "parent_id",
		Highlight: []string{"body"},
		Facets:    []types.FacetRequest{{Field: "topic", Limit: 10}},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Groups) != 1 {
		t.Fatalf("expected one grouped archive result, got %+v", page)
	}
	if page.Groups[0].Parent == nil || page.Groups[0].Parent.ID != record.Media.ID {
		t.Fatalf("expected media parent group, got %+v", page.Groups[0])
	}
	if page.Groups[0].TopHit == nil || page.Groups[0].TopHit.Anchor == nil || page.Groups[0].TopHit.Anchor.URL == "" {
		t.Fatalf("expected anchored top hit, got %+v", page.Groups[0].TopHit)
	}
	if page.Groups[0].TopHit.Snippet == nil || page.Groups[0].TopHit.Snippet.Highlighted == "" {
		t.Fatalf("expected transcript highlight snippet, got %+v", page.Groups[0].TopHit.Snippet)
	}
}

func TestTypesenseProviderSupportsArchiveFacetsAndRangeFiltering(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := media.DefaultArchiveIndexDefinition("media")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	startA, endA := int64(1000), int64(2000)
	startB, endB := int64(3000), int64(4000)
	docs := []types.Document{
		{
			ID:         "segment-architecture-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-architecture",
			SourceType: "transcript",
			SourceID:   "track-architecture",
			Title:      "Architecture Walkthrough",
			Body:       "archive architecture prayer",
			URL:        "https://example.org/video-architecture",
			AnchorURL:  "https://example.org/video-architecture#t=1",
			Locale:     "en",
			StartMS:    &startA,
			EndMS:      &endA,
			Fields: map[string]any{
				"parent_title":           "Architecture Walkthrough",
				"parent_url":             "https://example.org/video-architecture",
				media.FieldResultBadge:   "Blueprint",
				media.FieldPublishedYear: 2024,
			},
			Facets: map[string][]string{
				media.FacetFieldTopic:          {"architecture"},
				media.FacetFieldTopicHierarchy: {"Teaching Topics", "Teaching Topics > Architecture"},
				media.FacetFieldDurationBucket: {"30-60 min"},
				media.FacetFieldDecade:         {"2020s"},
				media.FacetFieldFormat:         {"Teaching"},
				media.FacetFieldLocale:         {"en"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear:   2024,
				media.FieldDurationSeconds: 2400,
			},
		},
		{
			ID:         "segment-tara-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-tara",
			SourceType: "transcript",
			SourceID:   "track-tara",
			Title:      "Tara Teachings",
			Body:       "archive tara prayer",
			URL:        "https://example.org/video-tara",
			AnchorURL:  "https://example.org/video-tara#t=3",
			Locale:     "en",
			StartMS:    &startB,
			EndMS:      &endB,
			Fields: map[string]any{
				"parent_title":           "Tara Teachings",
				"parent_url":             "https://example.org/video-tara",
				media.FieldResultBadge:   "Featured",
				media.FieldPublishedYear: 2024,
			},
			Facets: map[string][]string{
				media.FacetFieldTopic:          {"tara"},
				media.FacetFieldTopicHierarchy: {"Teaching Topics", "Teaching Topics > Tara"},
				media.FacetFieldDurationBucket: {"15-30 min"},
				media.FacetFieldDecade:         {"2020s"},
				media.FacetFieldFormat:         {"Teaching"},
				media.FacetFieldLocale:         {"en"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear:   2024,
				media.FieldDurationSeconds: 2400,
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
			types.TermExpr{Field: media.FacetFieldTopicHierarchy, Op: types.FilterOpEQ, Value: "Teaching Topics > Architecture"},
			types.RangeExpr{Field: media.FieldPublishedYear, GTE: 2024},
			types.RangeExpr{Field: media.FieldDurationSeconds, GTE: 1800},
		}},
		Facets: []types.FacetRequest{
			{Field: media.FacetFieldTopicHierarchy, Kind: types.FacetKindHierarchical, Disjunctive: true},
			{Field: media.FacetFieldDurationBucket, Disjunctive: true},
			{Field: media.FacetFieldDecade, Disjunctive: true},
		},
		Sort: []types.Sort{{Field: media.FieldPublishedYear, Direction: types.SortDesc}},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Groups) != 1 {
		t.Fatalf("expected one grouped archive result, got %+v", page)
	}
	if page.Groups[0].TopHit == nil || page.Groups[0].TopHit.Fields[media.FieldResultBadge] != "Blueprint" {
		t.Fatalf("expected badge metadata on hit, got %+v", page.Groups[0].TopHit)
	}
	foundSelected := false
	foundSibling := false
	for _, facet := range page.Facets {
		if facet.Field != media.FacetFieldTopicHierarchy {
			continue
		}
		for _, value := range facet.Values {
			if value.Value == "Teaching Topics > Architecture" && value.Selected {
				foundSelected = true
			}
			if value.Value == "Teaching Topics > Tara" {
				foundSibling = true
			}
		}
	}
	if !foundSelected || !foundSibling {
		t.Fatalf("expected selected and sibling hierarchy values in %+v", page.Facets)
	}
}

func TestTypesenseProviderSupportsFlatHeterogeneousSearchAcrossSeparateIndexes(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	defs := []types.IndexDefinition{
		content.DefaultIndexDefinition("videos"),
		content.DefaultIndexDefinition("documents"),
		content.DefaultIndexDefinition("blog_articles"),
	}
	for _, def := range defs {
		if err := provider.EnsureIndex(ctx, def); err != nil {
			t.Fatalf("ensure index %s: %v", def.Name, err)
		}
	}
	videoDoc := content.NewProjector(content.ProjectorConfig{Index: "videos", SourceType: "video"})
	documentDoc := content.NewProjector(content.ProjectorConfig{Index: "documents", SourceType: "document"})
	blogDoc := content.NewProjector(content.ProjectorConfig{Index: "blog_articles", SourceType: "blog_article"})
	videoDocs, _ := videoDoc.Project(ctx, content.Record{ID: "video-1", Type: types.DocumentTypeVideo, Title: "Search Architecture Walkthrough", Body: "search architecture video", URL: "/videos/1", Locale: "en"})
	documentDocs, _ := documentDoc.Project(ctx, content.Record{ID: "document-1", Type: types.DocumentTypeDocument, Title: "Search Rollout Workbook", Body: "search rollout document", URL: "/documents/1", Locale: "en"})
	blogDocs, _ := blogDoc.Project(ctx, content.Record{ID: "blog-1", Type: types.DocumentTypeBlogArticle, Title: "Search Notes", Body: "search notes article", URL: "/blog/1", Locale: "en"})
	for _, item := range []struct {
		index string
		docs  []types.Document
	}{
		{index: "videos", docs: videoDocs},
		{index: "documents", docs: documentDocs},
		{index: "blog_articles", docs: blogDocs},
	} {
		if err := provider.UpsertDocuments(ctx, item.index, item.docs); err != nil {
			t.Fatalf("upsert %s docs: %v", item.index, err)
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

func TestTypesenseProviderSupportsSharedIndexMultiRegistrationReindex(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := content.DefaultIndexDefinition("content_shared")
	registry := indexing.NewRegistry()
	if err := registry.Register(def, indexing.NewRegistrationWithKey(
		def.Name,
		def,
		"video",
		"video",
		content.NewSource([]content.Record{{ID: "video-1", Type: types.DocumentTypeVideo, Title: "Search Architecture Walkthrough", Body: "search architecture video", URL: "/videos/1", Locale: "en"}}),
		content.NewProjector(content.ProjectorConfig{Index: def.Name, SourceType: "video"}),
		func(record content.Record) string { return record.ID },
	)); err != nil {
		t.Fatalf("register video: %v", err)
	}
	if err := registry.Register(def, indexing.NewRegistrationWithKey(
		def.Name,
		def,
		"document",
		"document",
		content.NewSource([]content.Record{{ID: "document-1", Type: types.DocumentTypeDocument, Title: "Search Rollout Workbook", Body: "search rollout document", URL: "/documents/1", Locale: "en"}}),
		content.NewProjector(content.ProjectorConfig{Index: def.Name, SourceType: "document"}),
		func(record content.Record) string { return record.ID },
	)); err != nil {
		t.Fatalf("register document: %v", err)
	}
	if err := registry.Register(def, indexing.NewRegistrationWithKey(
		def.Name,
		def,
		"blog_article",
		"blog_article",
		content.NewSource([]content.Record{{ID: "blog-1", Type: types.DocumentTypeBlogArticle, Title: "Search Notes", Body: "search notes article", URL: "/blog/1", Locale: "en"}}),
		content.NewProjector(content.ProjectorConfig{Index: def.Name, SourceType: "blog_article"}),
		func(record content.Record) string { return record.ID },
	)); err != nil {
		t.Fatalf("register blog: %v", err)
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

func TestTypesenseProviderRejectsGroupedMixedIndexRequestsBeforeExecution(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	mediaDef := integrationIndexDefinition("media")
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
		t.Fatalf("register docs: %v", err)
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

func TestTypesenseProviderHonorsProjectedParentType(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := integrationIndexDefinition("notes")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	start, end := int64(1000), int64(2000)
	if err := provider.UpsertDocuments(ctx, def.Name, []types.Document{{
		ID:        "segment-1",
		Index:     def.Name,
		Type:      types.DocumentTypeTranscriptSegment,
		ParentID:  "document-42",
		Title:     "Reference Note",
		Body:      "reference note content",
		URL:       "/documents/42",
		AnchorURL: "/documents/42#t=1",
		Locale:    "en",
		StartMS:   &start,
		EndMS:     &end,
		Fields: map[string]any{
			"parent_title": "Reference Note",
			"parent_url":   "/documents/42",
			"parent_type":  types.DocumentTypeDocument,
		},
	}}); err != nil {
		t.Fatalf("upsert note doc: %v", err)
	}
	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{def.Name},
		Query:   "reference",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Hits) != 1 {
		t.Fatalf("expected one hit, got %+v", page)
	}
	if page.Hits[0].Parent == nil || page.Hits[0].Parent.Type != types.DocumentTypeDocument {
		t.Fatalf("expected mapped document parent type, got %+v", page.Hits[0].Parent)
	}
	if page.Hits[0].Anchor == nil || page.Hits[0].Anchor.ParentType != types.DocumentTypeDocument {
		t.Fatalf("expected mapped document anchor parent type, got %+v", page.Hits[0].Anchor)
	}
}

func TestTypesenseProviderSupportsArchiveMultiSelectFacetRefinement(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := media.DefaultArchiveIndexDefinition("media")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	startA, endA := int64(1000), int64(2000)
	startB, endB := int64(3000), int64(4000)
	startC, endC := int64(5000), int64(6000)
	docs := []types.Document{
		{
			ID:         "segment-architecture-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-architecture",
			SourceType: "transcript",
			SourceID:   "track-architecture",
			Title:      "Architecture Walkthrough",
			Body:       "archive architecture prayer",
			URL:        "https://example.org/video-architecture",
			AnchorURL:  "https://example.org/video-architecture#t=1",
			Locale:     "en",
			StartMS:    &startA,
			EndMS:      &endA,
			Fields: map[string]any{
				"parent_title":         "Architecture Walkthrough",
				"parent_url":           "https://example.org/video-architecture",
				media.FieldResultBadge: "Blueprint",
			},
			Facets: map[string][]string{
				media.FacetFieldTopicHierarchy: {"Teaching Topics", "Teaching Topics > Architecture"},
				media.FacetFieldFormat:         {"Teaching"},
				media.FacetFieldLocale:         {"en"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear: 2024,
			},
		},
		{
			ID:         "segment-tara-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-tara",
			SourceType: "transcript",
			SourceID:   "track-tara",
			Title:      "Tara Teachings",
			Body:       "archive tara prayer",
			URL:        "https://example.org/video-tara",
			AnchorURL:  "https://example.org/video-tara#t=3",
			Locale:     "en",
			StartMS:    &startB,
			EndMS:      &endB,
			Fields: map[string]any{
				"parent_title":         "Tara Teachings",
				"parent_url":           "https://example.org/video-tara",
				media.FieldResultBadge: "Featured",
			},
			Facets: map[string][]string{
				media.FacetFieldTopicHierarchy: {"Teaching Topics", "Teaching Topics > Tara"},
				media.FacetFieldFormat:         {"Teaching"},
				media.FacetFieldLocale:         {"en"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear: 2024,
			},
		},
		{
			ID:         "segment-ranking-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-ranking",
			SourceType: "transcript",
			SourceID:   "track-ranking",
			Title:      "Ranking Workshop",
			Body:       "archive ranking prayer",
			URL:        "https://example.org/video-ranking",
			AnchorURL:  "https://example.org/video-ranking#t=5",
			Locale:     "en",
			StartMS:    &startC,
			EndMS:      &endC,
			Fields: map[string]any{
				"parent_title": "Ranking Workshop",
				"parent_url":   "https://example.org/video-ranking",
			},
			Facets: map[string][]string{
				media.FacetFieldTopicHierarchy: {"Teaching Topics", "Teaching Topics > Ranking"},
				media.FacetFieldFormat:         {"Workshop"},
				media.FacetFieldLocale:         {"en"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear: 2022,
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

	selectedTopics := map[string]bool{}
	foundSibling := false
	for _, facet := range page.Facets {
		if facet.Field != media.FacetFieldTopicHierarchy {
			continue
		}
		for _, value := range facet.Values {
			if value.Selected {
				selectedTopics[value.Value] = true
			}
			if value.Value == "Teaching Topics > Ranking" {
				foundSibling = true
			}
		}
	}
	if !selectedTopics["Teaching Topics > Architecture"] || !selectedTopics["Teaching Topics > Tara"] {
		t.Fatalf("expected selected multi-value hierarchy facet state in %+v", page.Facets)
	}
	if !foundSibling {
		t.Fatalf("expected disjunctive sibling topic facet in %+v", page.Facets)
	}
}

func TestTypesenseProviderSupportsArchiveTopicLandingPreset(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := media.DefaultArchiveIndexDefinition("media")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	startA, endA := int64(1000), int64(2000)
	startB, endB := int64(3000), int64(4000)
	docs := []types.Document{
		{
			ID:         "segment-architecture-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-architecture",
			SourceType: "transcript",
			SourceID:   "track-architecture",
			Title:      "Architecture Walkthrough",
			Body:       "archive architecture prayer",
			URL:        "https://example.org/video-architecture",
			AnchorURL:  "https://example.org/video-architecture#t=1",
			Locale:     "en",
			StartMS:    &startA,
			EndMS:      &endA,
			Fields: map[string]any{
				"parent_title": "Architecture Walkthrough",
				"parent_url":   "https://example.org/video-architecture",
			},
			Facets: map[string][]string{
				media.FacetFieldTopicHierarchy: {"Teaching Topics", "Teaching Topics > Architecture"},
				media.FacetFieldLocale:         {"en"},
			},
		},
		{
			ID:         "segment-tara-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-tara",
			SourceType: "transcript",
			SourceID:   "track-tara",
			Title:      "Tara Teachings",
			Body:       "archive tara prayer",
			URL:        "https://example.org/video-tara",
			AnchorURL:  "https://example.org/video-tara#t=3",
			Locale:     "en",
			StartMS:    &startB,
			EndMS:      &endB,
			Fields: map[string]any{
				"parent_title": "Tara Teachings",
				"parent_url":   "https://example.org/video-tara",
			},
			Facets: map[string][]string{
				media.FacetFieldTopicHierarchy: {"Teaching Topics", "Teaching Topics > Tara"},
				media.FacetFieldLocale:         {"en"},
			},
		},
	}
	if err := provider.UpsertDocuments(ctx, "media", docs); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}

	preset, ok := media.TopicLandingPreset("architecture")
	if !ok {
		t.Fatalf("expected architecture landing preset")
	}
	terms := make([]types.FilterExpr, 0, len(preset.FacetFilter))
	for field, values := range preset.FacetFilter {
		switch len(values) {
		case 0:
			continue
		case 1:
			terms = append(terms, types.TermExpr{Field: field, Op: types.FilterOpEQ, Value: values[0]})
		default:
			terms = append(terms, types.TermExpr{Field: field, Op: types.FilterOpIn, Value: values})
		}
	}

	page, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "archive",
		Locale:  "en",
		GroupBy: "parent_id",
		Page:    1,
		PerPage: 10,
		Filters: types.AndExpr{Terms: terms},
		Facets: []types.FacetRequest{
			{Field: media.FacetFieldTopicHierarchy, Kind: types.FacetKindHierarchical, Disjunctive: true},
		},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total != 1 || len(page.Groups) != 1 {
		t.Fatalf("expected one landing preset result, got %+v", page)
	}
	if page.Groups[0].Parent == nil || page.Groups[0].Parent.ID != "video-architecture" {
		t.Fatalf("expected architecture parent result, got %+v", page.Groups[0])
	}
	foundSelected := false
	for _, facet := range page.Facets {
		if facet.Field != media.FacetFieldTopicHierarchy {
			continue
		}
		for _, value := range facet.Values {
			if value.Value == "Teaching Topics > Architecture" && value.Selected {
				foundSelected = true
			}
		}
	}
	if !foundSelected {
		t.Fatalf("expected landing preset hierarchy selection in %+v", page.Facets)
	}
}

func TestTypesenseProviderLocaleSuggestDeleteBySourceAndStats(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := integrationIndexDefinition("media")
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	startA, endA := int64(1000), int64(2000)
	startB, endB := int64(2500), int64(3500)
	startC, endC := int64(4000), int64(5000)
	docs := []types.Document{
		{
			ID:         "segment-en-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-1",
			SourceType: "transcript",
			SourceID:   "track-en",
			Title:      "Ocean Wind",
			Body:       "archive prayer by the ocean",
			URL:        "https://example.org/video-1",
			AnchorURL:  "https://example.org/video-1#t=1",
			Locale:     "en",
			StartMS:    &startA,
			EndMS:      &endA,
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{"topic": {"archive"}},
		},
		{
			ID:         "segment-en-2",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-1",
			SourceType: "transcript",
			SourceID:   "track-en",
			Title:      "Ocean Wind",
			Body:       "archive prayer continues",
			URL:        "https://example.org/video-1",
			AnchorURL:  "https://example.org/video-1#t=2",
			Locale:     "en",
			StartMS:    &startB,
			EndMS:      &endB,
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{"topic": {"archive"}},
		},
		{
			ID:         "segment-bo-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-2",
			SourceType: "transcript",
			SourceID:   "track-bo",
			Title:      "Ocean Wind Tibetan",
			Body:       "archive prayer by the ocean",
			URL:        "https://example.org/video-2",
			AnchorURL:  "https://example.org/video-2#t=4",
			Locale:     "bo",
			StartMS:    &startC,
			EndMS:      &endC,
			Fields: map[string]any{
				"parent_title": "Ocean Wind Tibetan",
				"parent_url":   "https://example.org/video-2",
			},
			Facets: map[string][]string{"topic": {"archive"}},
		},
	}
	if err := provider.UpsertDocuments(ctx, "media", docs); err != nil {
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
		t.Fatalf("search locale ranking: %v", err)
	}
	if len(page.Hits) < 2 {
		t.Fatalf("expected hits across exact and fallback locales, got %+v", page.Hits)
	}
	if page.Hits[0].Locale != "en" {
		t.Fatalf("expected exact locale hit first, got %+v", page.Hits[0])
	}
	if page.Hits[0].Retrieval == nil || page.Hits[0].Retrieval.Metadata["locale_match"] != "exact" {
		t.Fatalf("expected exact locale retrieval metadata, got %+v", page.Hits[0].Retrieval)
	}

	suggest, err := provider.Suggest(ctx, types.SuggestRequest{
		Indexes:      []string{"media"},
		Query:        "Ocean",
		Locale:       "en",
		Limit:        10,
		PreferParent: true,
	})
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggest.Items) != 1 || suggest.Items[0].ID != "video-1" {
		t.Fatalf("expected parent-deduplicated suggestion, got %+v", suggest.Items)
	}

	if err := provider.DeleteBySource(ctx, "media", []string{"track-en"}); err != nil {
		t.Fatalf("delete by source: %v", err)
	}
	afterDelete, err := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search after delete by source: %v", err)
	}
	if afterDelete.Total != 1 || afterDelete.Hits[0].Document == nil || afterDelete.Hits[0].Document.SourceID != "track-bo" {
		t.Fatalf("expected only sibling track to remain, got %+v", afterDelete.Hits)
	}

	registry := indexing.NewRegistry()
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	statsQuery, err := query.NewStats(query.StatsConfig{
		Provider: provider,
		Registry: registry,
	})
	if err != nil {
		t.Fatalf("new stats query: %v", err)
	}
	stats, err := statsQuery.Query(ctx, types.StatsRequest{Indexes: []string{"media"}})
	if err != nil {
		t.Fatalf("stats query: %v", err)
	}
	if len(stats.Indexes) != 1 || stats.Indexes[0].Documents != 1 {
		t.Fatalf("expected stats to reflect remaining docs, got %+v", stats.Indexes)
	}

	health, err := provider.Health(ctx, types.HealthRequest{Indexes: []string{"media"}})
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Metadata["collection_name"] == "" || health.Indexes[0].Metadata["schema_hash"] == "" {
		t.Fatalf("expected health metadata for index, got %+v", health.Indexes)
	}
}

func TestTypesenseProviderReplaceDocumentsPreservesDataUntilUpsertSucceeds(t *testing.T) {
	provider := newIntegrationProvider(t)
	ctx := context.Background()
	def := integrationIndexDefinition("media")
	def.SearchableFields = append(def.SearchableFields, "custom_runtime_field")
	def.ProviderHints = map[string]any{
		"typesense": map[string]any{
			"field_types": map[string]any{
				"custom_runtime_field": "string",
			},
		},
	}
	if err := provider.EnsureIndex(ctx, def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	start, end := int64(1000), int64(2000)
	original := []types.Document{
		{
			ID:         "segment-1",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-1",
			SourceType: "transcript",
			SourceID:   "track-1",
			Title:      "Ocean Wind",
			Body:       "archive prayer",
			URL:        "https://example.org/video-1",
			AnchorURL:  "https://example.org/video-1#t=1",
			Locale:     "en",
			StartMS:    &start,
			EndMS:      &end,
		},
	}
	if err := provider.UpsertDocuments(ctx, "media", original); err != nil {
		t.Fatalf("upsert original docs: %v", err)
	}

	invalid := []types.Document{
		{
			ID:         "segment-2",
			Index:      "media",
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-1",
			SourceType: "transcript",
			SourceID:   "track-1",
			Title:      "Ocean Wind",
			Body:       "archive prayer replacement",
			URL:        "https://example.org/video-1",
			AnchorURL:  "https://example.org/video-1#t=3",
			Locale:     "en",
			StartMS:    &start,
			EndMS:      &end,
			Fields: map[string]any{
				"custom_runtime_field": "boom",
			},
		},
	}
	err := provider.ReplaceDocuments(ctx, "media", []string{"track-1"}, invalid)
	if err == nil {
		t.Fatalf("expected replace documents to fail on invalid payload")
	}

	page, searchErr := provider.Search(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Page:    1,
		PerPage: 10,
	})
	if searchErr != nil {
		t.Fatalf("search after failed replace: %v", searchErr)
	}
	if page.Total == 0 {
		t.Fatalf("expected original docs to remain after failed replace")
	}
}

func newIntegrationProvider(t *testing.T) *Provider {
	t.Helper()
	url := testkit.Integration.Typesense.ServerURL
	key := testkit.Integration.Typesense.APIKey
	if url == "" || key == "" {
		t.Skip("testkit.Integration.Typesense.ServerURL and APIKey are required")
	}

	provider, err := New(Config{
		ServerURL:            url,
		APIKey:               key,
		CollectionPrefix:     uniqueCollectionPrefix(t.Name()),
		ConnectionTimeout:    testkit.Integration.Typesense.ConnectionTimeout,
		GroupedEvidenceLimit: 5,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	t.Cleanup(func() {
		if err := provider.cleanup(context.Background()); err != nil {
			t.Fatalf("cleanup provider collections: %v", err)
		}
	})
	return provider
}

func integrationIndexDefinition(name string) types.IndexDefinition {
	return types.IndexDefinition{
		Name:               name,
		DefaultQueryFields: []string{"title", "body", "parent_title", "parent_summary"},
		SearchableFields:   []string{"title", "summary", "body", "parent_title", "parent_summary"},
		FacetFields:        []string{"topic", "locale", "parent_id", "source_type", "source_id"},
		FilterableFields:   []string{"topic", "locale", "parent_id", "source_type", "source_id", "start_ms", "end_ms"},
		SortableFields:     []string{"start_ms", "end_ms"},
		HighlightFields:    []string{"body", "summary", "title"},
		GroupByDefault:     "parent_id",
	}
}

func uniqueCollectionPrefix(name string) string {
	return fmt.Sprintf("it_%d_%s_", time.Now().UnixNano(), sanitizeName(name))
}

func sanitizeName(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
