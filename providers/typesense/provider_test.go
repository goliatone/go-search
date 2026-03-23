package typesense

import (
	"encoding/json"
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
	if field, ok := fields["scope_tenant_id"]; !ok || field.Facet == nil || !*field.Facet {
		t.Fatalf("expected scope_tenant_id facet field, got %+v", field)
	}
	if field, ok := fields["visibility_public"]; !ok || field.Type != "bool" {
		t.Fatalf("expected visibility_public bool field, got %+v", field)
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

func TestCompileSearchParamsAddsScopeFilters(t *testing.T) {
	def := types.IndexDefinition{
		Name:               "media",
		DefaultQueryFields: []string{"title"},
	}
	params, err := compileSearchParams(Config{}, def, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "ocean",
		Page:    1,
		PerPage: 10,
		Scope: types.Scope{
			TenantID: "tenant-a",
			OrgID:    "org-1",
			Labels:   map[string]string{"role": "member", "surface": "support"},
		},
	})
	if err != nil {
		t.Fatalf("compile search params: %v", err)
	}
	if params.FilterBy == nil {
		t.Fatalf("expected scope filter")
	}
	filter := *params.FilterBy
	for _, fragment := range []string{
		"scope_tenant_id:=tenant-a",
		"scope_org_id:=org-1",
		"scope_labels:=role=member",
		"scope_labels:=surface=support",
	} {
		if !strings.Contains(filter, fragment) {
			t.Fatalf("expected filter fragment %q in %q", fragment, filter)
		}
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
		ID:              "segment-1",
		RegistrationKey: "transcript",
		Type:            types.DocumentTypeTranscriptSegment,
		Title:           "Ocean Wind",
		Locale:          "en",
		Facets:          map[string][]string{"topic": {"archive"}},
		Scope:           types.Scope{TenantID: "tenant-a", OrgID: "org-1", Labels: map[string]string{"role": "member"}},
		Visibility: types.Visibility{
			Public:      true,
			Roles:       []string{"editor"},
			Permissions: []string{"search:view"},
			Status:      "published",
		},
	}
	payload := compileDocument(types.IndexDefinition{
		Name:             "media",
		FilterableFields: []string{"topic", "locale"},
	}, doc)
	if payload["id"] != "transcript::segment-1" {
		t.Fatalf("expected internal storage id, got %+v", payload["id"])
	}
	if payload["document_id"] != "segment-1" {
		t.Fatalf("expected external document id field, got %+v", payload["document_id"])
	}
	if payload["index"] != "media" {
		t.Fatalf("expected index fallback from definition, got %+v", payload["index"])
	}
	if payload["scope_tenant_id"] != "tenant-a" || payload["scope_org_id"] != "org-1" {
		t.Fatalf("expected scope fields, got %+v", payload)
	}
	if payload["visibility_public"] != true || payload["visibility_status"] != "published" {
		t.Fatalf("expected visibility fields, got %+v", payload)
	}
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

func TestMapDocumentRestoresScopeAndVisibility(t *testing.T) {
	doc := mapDocument(&map[string]any{
		"id":                     "video::shared-1",
		"document_id":            "shared-1",
		"index":                  "media",
		"registration_key":       "video",
		"type":                   types.DocumentTypeVideo,
		"title":                  "Shared Video",
		"scope_tenant_id":        "tenant-a",
		"scope_org_id":           "org-1",
		"scope_labels":           []any{"role=member", "surface=support"},
		"visibility_public":      true,
		"visibility_roles":       []any{"editor"},
		"visibility_permissions": []any{"search:view"},
		"visibility_status":      "published",
	})
	if doc.Scope.TenantID != "tenant-a" || doc.Scope.OrgID != "org-1" {
		t.Fatalf("expected scope fields, got %+v", doc.Scope)
	}
	if doc.Scope.Labels["role"] != "member" || doc.Scope.Labels["surface"] != "support" {
		t.Fatalf("expected scope labels, got %+v", doc.Scope.Labels)
	}
	if !doc.Visibility.Public || doc.Visibility.Status != "published" {
		t.Fatalf("expected visibility fields, got %+v", doc.Visibility)
	}
	if len(doc.Visibility.Roles) != 1 || doc.Visibility.Roles[0] != "editor" {
		t.Fatalf("expected visibility roles, got %+v", doc.Visibility)
	}
}

func TestCompareSearchHitsHonorsRequestedSorts(t *testing.T) {
	left := types.SearchHit{
		ID:       "b",
		Title:    "Later",
		Score:    100,
		Anchor:   &types.MediaAnchor{StartMS: 200},
		Document: &types.Document{Numeric: map[string]float64{"start_ms": 200}},
	}
	right := types.SearchHit{
		ID:       "a",
		Title:    "Earlier",
		Score:    10,
		Anchor:   &types.MediaAnchor{StartMS: 100},
		Document: &types.Document{Numeric: map[string]float64{"start_ms": 100}},
	}
	if !compareSearchHits(types.SearchRequest{
		Sort: []types.Sort{{Field: "start_ms", Direction: types.SortAsc}},
	}, right, left) {
		t.Fatalf("expected requested sort to outrank score")
	}
}

func TestDocumentPayloadHashIgnoresMapOrder(t *testing.T) {
	left := map[string]any{
		"id":    "video::shared-1",
		"title": "Shared Video",
		"scope_labels": []string{
			"role=member",
			"surface=support",
		},
	}
	right := map[string]any{}
	raw := []byte(`{"title":"Shared Video","scope_labels":["role=member","surface=support"],"id":"video::shared-1"}`)
	if err := json.Unmarshal(raw, &right); err != nil {
		t.Fatalf("unmarshal right payload: %v", err)
	}
	if documentPayloadHash(left) != documentPayloadHash(right) {
		t.Fatalf("expected stable payload hash across map order")
	}
}

func TestMapDocumentPrefersExternalDocumentID(t *testing.T) {
	doc := mapDocument(&map[string]any{
		"id":               "video::shared-1",
		"document_id":      "shared-1",
		"registration_key": "video",
		"type":             types.DocumentTypeVideo,
		"title":            "Shared Video",
		"source_id":        "shared-1",
	})
	if doc.ID != "shared-1" {
		t.Fatalf("expected external document id, got %+v", doc)
	}
	if doc.RegistrationKey != "video" {
		t.Fatalf("expected registration key, got %+v", doc)
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
