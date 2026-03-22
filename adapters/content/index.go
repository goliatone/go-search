package content

import "github.com/goliatone/go-search/pkg/types"

func DefaultIndexDefinition(name string) types.IndexDefinition {
	return types.IndexDefinition{
		Name:               name,
		Label:              "Unified content",
		DefaultQueryFields: []string{"title", "summary", "body", "topic", "people", "series"},
		SearchableFields:   []string{"title", "summary", "body", "topic", "people", "series"},
		FacetFields: []string{
			"entity_type",
			"topic",
			"topic_hierarchy",
			"category",
			"category_hierarchy",
			"people",
			"subject",
			"text",
			"deity",
			"locale",
			"decade",
			"duration_bucket",
			"location",
			"sangha",
			"format",
			"series",
		},
		SortableFields:   []string{"title", "published_year", "duration_seconds"},
		FilterableFields: []string{"entity_type", "topic", "topic_hierarchy", "category", "category_hierarchy", "people", "subject", "text", "deity", "locale", "decade", "duration_bucket", "location", "sangha", "format", "series", "published_year", "duration_seconds"},
		HighlightFields:  []string{"summary", "body"},
		DefaultSort:      []types.Sort{{Field: "published_year", Direction: types.SortDesc}},
		ProviderHints: map[string]any{
			"typesense": map[string]any{
				"field_types": map[string]any{
					"entity_type":        "string",
					"topic":              "string[]",
					"topic_hierarchy":    "string[]",
					"category":           "string",
					"category_hierarchy": "string[]",
					"people":             "string[]",
					"subject":            "string[]",
					"text":               "string[]",
					"deity":              "string[]",
					"locale":             "string",
					"decade":             "string",
					"duration_bucket":    "string",
					"location":           "string",
					"sangha":             "string",
					"format":             "string",
					"series":             "string",
					"published_year":     "float",
					"duration_seconds":   "float",
					"result_badge":       "string",
				},
			},
		},
	}
}
