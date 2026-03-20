package typesense

import (
	"testing"

	"github.com/goliatone/go-search/pkg/types"
	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

func TestBuildCollectionSchemaRequiresCustomFieldHint(t *testing.T) {
	_, _, err := buildCollectionSchema(Config{}, types.IndexDefinition{
		Name:             "media",
		SearchableFields: []string{"custom_field"},
	})
	if err == nil {
		t.Fatalf("expected custom field type hint error")
	}
}

func TestBuildCollectionSchemaIncludesFixedTranscriptFields(t *testing.T) {
	schema, _, err := buildCollectionSchema(Config{CollectionPrefix: "test_"}, types.IndexDefinition{
		Name:           "media",
		GroupByDefault: "parent_id",
	})
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	fields := map[string]tsapi.Field{}
	for _, field := range schema.Fields {
		fields[field.Name] = field
	}
	if _, ok := fields["body"]; !ok {
		t.Fatalf("expected body field in schema")
	}
	if field, ok := fields["parent_id"]; !ok || field.Facet == nil || !*field.Facet {
		t.Fatalf("expected parent_id to be faceted for grouped search")
	}
	if field, ok := fields["topic"]; !ok || field.Type != "string[]" {
		t.Fatalf("expected topic string[] facet field, got %+v", field)
	}
}

func TestCompileSearchParamsAddsLocaleFilterGroupingAndSortBoost(t *testing.T) {
	def := types.IndexDefinition{
		Name:               "media",
		DefaultQueryFields: []string{"title", "body"},
		HighlightFields:    []string{"body"},
		FacetFields:        []string{"topic"},
		FilterableFields:   []string{"topic", "locale"},
		GroupByDefault:     "parent_id",
	}
	params, err := compileSearchParams(Config{}, def, types.SearchRequest{
		Indexes:   []string{"media"},
		Query:     "prayer",
		Locale:    "en",
		Locales:   []string{"bo"},
		Page:      2,
		PerPage:   10,
		GroupBy:   "parent_id",
		Highlight: []string{"body"},
		Facets:    []types.FacetRequest{{Field: "topic", Limit: 5}},
		Sort:      []types.Sort{{Field: "start_ms", Direction: types.SortAsc}},
	})
	if err != nil {
		t.Fatalf("compile search params: %v", err)
	}
	if params.FilterBy == nil || *params.FilterBy != "locale:=[en,bo]" {
		t.Fatalf("expected locale filter, got %+v", params.FilterBy)
	}
	if params.GroupBy == nil || *params.GroupBy != "parent_id" {
		t.Fatalf("expected group_by parent_id, got %+v", params.GroupBy)
	}
	if params.GroupLimit == nil || *params.GroupLimit != normalizeConfig(Config{}).GroupedEvidenceLimit {
		t.Fatalf("expected evidence limit %d, got %+v", normalizeConfig(Config{}).GroupedEvidenceLimit, params.GroupLimit)
	}
	if params.SortBy == nil || *params.SortBy != "_eval(locale:=en):desc,start_ms:asc" {
		t.Fatalf("unexpected sort_by: %+v", params.SortBy)
	}
}

func TestCompileSearchParamsCompilesExistsExprToShadowField(t *testing.T) {
	def := types.IndexDefinition{
		Name:               "media",
		DefaultQueryFields: []string{"title"},
		FilterableFields:   []string{"topic"},
	}
	params, err := compileSearchParams(Config{}, def, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "ocean",
		Page:    1,
		PerPage: 10,
		Filters: types.ExistsExpr{Field: "topic", Exists: true},
	})
	if err != nil {
		t.Fatalf("compile search params: %v", err)
	}
	if params.FilterBy == nil || *params.FilterBy != "__exists_topic:=true" {
		t.Fatalf("expected exists shadow filter, got %+v", params.FilterBy)
	}
}

func TestCompileDocumentAddsExistsShadowFields(t *testing.T) {
	doc := types.Document{
		ID:     "segment-1",
		Index:  "media",
		Type:   types.DocumentTypeTranscriptSegment,
		Title:  "Ocean Wind",
		Locale: "en",
		Facets: map[string][]string{"topic": {"archive"}},
	}
	payload := compileDocument(types.IndexDefinition{
		Name:             "media",
		FilterableFields: []string{"topic", "locale"},
	}, doc)
	if payload["__exists_topic"] != true {
		t.Fatalf("expected topic exists shadow field, got %+v", payload["__exists_topic"])
	}
	if payload["__exists_locale"] != true {
		t.Fatalf("expected locale exists shadow field, got %+v", payload["__exists_locale"])
	}
	if payload["__exists_source_id"] != false {
		t.Fatalf("expected empty source_id shadow field to be false, got %+v", payload["__exists_source_id"])
	}
}

func TestMapSuggestHitsPreferParentDeduplicates(t *testing.T) {
	result := &tsapi.SearchResult{
		Hits: &[]tsapi.SearchResultHit{
			{
				Document: &map[string]any{
					"id":           "segment-1",
					"type":         types.DocumentTypeTranscriptSegment,
					"title":        "Ocean Wind",
					"url":          "https://example.org/video-1#t=1",
					"locale":       "en",
					"parent_id":    "video-1",
					"parent_title": "Ocean Wind",
					"parent_url":   "https://example.org/video-1",
				},
				TextMatch: int64Ptr(100),
			},
			{
				Document: &map[string]any{
					"id":           "segment-2",
					"type":         types.DocumentTypeTranscriptSegment,
					"title":        "Ocean Wind",
					"url":          "https://example.org/video-1#t=2",
					"locale":       "en",
					"parent_id":    "video-1",
					"parent_title": "Ocean Wind",
					"parent_url":   "https://example.org/video-1",
				},
				TextMatch: int64Ptr(90),
			},
		},
	}
	items := mapSuggestHits(result, types.SuggestRequest{
		Query:        "Ocean",
		Limit:        5,
		PreferParent: true,
	})
	if len(items) != 1 {
		t.Fatalf("expected one deduplicated suggestion, got %d", len(items))
	}
	if items[0].ID != "video-1" {
		t.Fatalf("expected parent suggestion id, got %+v", items[0])
	}
}

//go:fix inline
func int64Ptr(value int64) *int64 { return new(value) }
