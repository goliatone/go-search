package media

import (
	"github.com/goliatone/go-search/pkg/types"
)

func DefaultArchiveIndexDefinition(name string) types.IndexDefinition {
	return types.IndexDefinition{
		Name:               name,
		Label:              "Archive media transcripts",
		DefaultQueryFields: []string{"title", "summary", "body"},
		SearchableFields:   []string{"title", "summary", "body"},
		FacetFields:        []string{"topic", "locale"},
		FilterableFields:   []string{"topic", "locale"},
		HighlightFields:    []string{"body"},
		GroupByDefault:     "parent_id",
	}
}
