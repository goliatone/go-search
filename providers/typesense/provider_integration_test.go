package typesense

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
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
