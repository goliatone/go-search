package media

import (
	"github.com/goliatone/go-search/pkg/types"
)

func DefaultArchiveIndexDefinition(name string) types.IndexDefinition {
	return types.IndexDefinition{
		Name:               name,
		Label:              "Archive media transcripts",
		DefaultQueryFields: []string{"title", "summary", "body", "parent_title", "subject", "text", "people"},
		SearchableFields:   []string{"title", "summary", "body", "parent_title", "subject", "text", "people"},
		FacetFields: []string{
			FacetFieldTopic,
			FacetFieldTopicHierarchy,
			FacetFieldCategory,
			FacetFieldCategoryHierarchy,
			FacetFieldPeople,
			FacetFieldSubject,
			FacetFieldText,
			FacetFieldDeity,
			FacetFieldLocale,
			FacetFieldDecade,
			FacetFieldDurationBucket,
			FacetFieldLocation,
			FacetFieldSangha,
			FacetFieldFormat,
			FacetFieldSeries,
		},
		SortableFields: []string{"title", "start_ms", FieldPublishedYear, FieldDurationSeconds},
		FilterableFields: []string{
			FacetFieldTopic,
			FacetFieldTopicHierarchy,
			FacetFieldCategory,
			FacetFieldCategoryHierarchy,
			FacetFieldPeople,
			FacetFieldSubject,
			FacetFieldText,
			FacetFieldDeity,
			FacetFieldLocale,
			FacetFieldDecade,
			FacetFieldDurationBucket,
			FacetFieldLocation,
			FacetFieldSangha,
			FacetFieldFormat,
			FacetFieldSeries,
			FieldPublishedYear,
			FieldDurationSeconds,
		},
		HighlightFields: []string{"body"},
		DefaultSort:     []types.Sort{{Field: FieldPublishedYear, Direction: types.SortDesc}},
		GroupByDefault:  "parent_id",
		ProviderHints: map[string]any{
			"typesense": map[string]any{
				"field_types": map[string]any{
					FacetFieldTopic:             "string[]",
					FacetFieldTopicHierarchy:    "string[]",
					FacetFieldCategory:          "string",
					FacetFieldCategoryHierarchy: "string[]",
					FacetFieldPeople:            "string[]",
					FacetFieldSubject:           "string[]",
					FacetFieldText:              "string[]",
					FacetFieldDeity:             "string[]",
					FacetFieldDecade:            "string",
					FacetFieldDurationBucket:    "string",
					FacetFieldLocation:          "string",
					FacetFieldSangha:            "string",
					FacetFieldFormat:            "string",
					FacetFieldSeries:            "string",
					FieldPublishedYear:          "float",
					FieldDurationSeconds:        "float",
					FieldResultBadge:            "string",
				},
			},
		},
	}
}
