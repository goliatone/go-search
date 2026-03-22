package typesense

import (
	"strings"
	"testing"

	"github.com/goliatone/go-search/adapters/media"
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

func TestBuildCollectionSchemaMarksArchiveRangeFields(t *testing.T) {
	schema, _, err := buildCollectionSchema(Config{}, media.DefaultArchiveIndexDefinition("media"))
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	fields := map[string]tsapi.Field{}
	for _, field := range schema.Fields {
		fields[field.Name] = field
	}
	for _, name := range []string{media.FieldPublishedYear, media.FieldDurationSeconds} {
		field, ok := fields[name]
		if !ok || field.RangeIndex == nil || !*field.RangeIndex {
			t.Fatalf("expected range index on %s, got %+v", name, field)
		}
		if field.Sort == nil || !*field.Sort {
			t.Fatalf("expected sort on %s, got %+v", name, field)
		}
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

func TestCompileSearchParamsCompilesArchiveRangeAndHierarchyFilters(t *testing.T) {
	params, err := compileSearchParams(Config{}, media.DefaultArchiveIndexDefinition("media"), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.TermExpr{Field: media.FacetFieldTopicHierarchy, Op: types.FilterOpEQ, Value: "Teaching Topics > Architecture"},
			types.RangeExpr{Field: media.FieldPublishedYear, GTE: 2024},
			types.RangeExpr{Field: media.FieldDurationSeconds, GTE: 1800, LTE: 3600},
		}},
		Facets: []types.FacetRequest{
			{Field: media.FacetFieldTopicHierarchy, Kind: types.FacetKindHierarchical, Disjunctive: true},
			{Field: media.FacetFieldDurationBucket, Disjunctive: true},
		},
		Sort: []types.Sort{{Field: media.FieldPublishedYear, Direction: types.SortDesc}},
	})
	if err != nil {
		t.Fatalf("compile search params: %v", err)
	}
	if params.FilterBy == nil {
		t.Fatalf("expected filter_by")
	}
	filter := *params.FilterBy
	for _, fragment := range []string{
		"topic_hierarchy:=`Teaching Topics > Architecture`",
		"published_year:>=2024",
		"duration_seconds:[1800..3600]",
		"locale:=en",
	} {
		if !strings.Contains(filter, fragment) {
			t.Fatalf("expected filter fragment %q in %q", fragment, filter)
		}
	}
	if params.SortBy == nil || *params.SortBy != "_eval(locale:=en):desc,published_year:desc" {
		t.Fatalf("unexpected sort_by: %+v", params.SortBy)
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
				TextMatch: new(int64(100)),
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
				TextMatch: new(int64(90)),
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

func TestMapFacetsBuildsHierarchicalFacetMetadata(t *testing.T) {
	result := &tsapi.SearchResult{
		FacetCounts: &[]tsapi.FacetCounts{
			{
				FieldName: new("topic_hierarchy"),
				Counts: &[]struct {
					Count       *int            `json:"count,omitempty"`
					Highlighted *string         `json:"highlighted,omitempty"`
					Parent      *map[string]any `json:"parent,omitempty"`
					Value       *string         `json:"value,omitempty"`
				}{
					{Value: new("Teaching Topics"), Count: new(3)},
					{Value: new("Teaching Topics > Tara"), Count: new(2)},
				},
			},
		},
	}
	facets := mapFacets(result, types.SearchRequest{
		Facets: []types.FacetRequest{
			{Field: "topic_hierarchy", Kind: types.FacetKindHierarchical, Disjunctive: true},
		},
		Filters: types.TermExpr{Field: "topic_hierarchy", Op: types.FilterOpEQ, Value: "Teaching Topics > Tara"},
	})
	if len(facets) != 1 {
		t.Fatalf("facets = %+v", facets)
	}
	if facets[0].Kind != types.FacetKindHierarchical || !facets[0].Disjunctive {
		t.Fatalf("facet metadata = %+v", facets[0])
	}
	if facets[0].Metadata["separator"] != types.DefaultFacetPathSeparator {
		t.Fatalf("facet metadata = %+v", facets[0].Metadata)
	}
	if len(facets[0].Values) < 2 {
		t.Fatalf("facet values = %+v", facets[0].Values)
	}
	for _, value := range facets[0].Values {
		if value.Value == "Teaching Topics > Tara" && !value.Selected {
			t.Fatalf("expected selected value in %+v", facets[0].Values)
		}
	}
}

//go:fix inline
func int64Ptr(value int64) *int64 { return new(value) }
