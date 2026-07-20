package requestvalidate

import (
	"fmt"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/internal/filtervalidate"
	"github.com/goliatone/go-search/pkg/types"
)

func Search(req types.SearchRequest, limits types.RequestLimits) error {
	limits = types.NormalizeRequestLimits(limits)
	if len(req.Query) > limits.MaxQueryBytes {
		return tooLarge("query", len(req.Query), limits.MaxQueryBytes)
	}
	if len(req.Indexes) > limits.MaxIndexes {
		return tooLarge("indexes", len(req.Indexes), limits.MaxIndexes)
	}
	if req.Page > limits.MaxPage {
		return tooLarge("page", req.Page, limits.MaxPage)
	}
	if req.PerPage > limits.MaxPerPage {
		return tooLarge("per_page", req.PerPage, limits.MaxPerPage)
	}
	if req.Page > 0 && req.PerPage > 0 && req.Page > limits.MaxCandidateWindow/req.PerPage {
		return tooLarge("candidate_window", limits.MaxCandidateWindow+1, limits.MaxCandidateWindow)
	}
	if len(req.Sort) > limits.MaxSortFields {
		return tooLarge("sort", len(req.Sort), limits.MaxSortFields)
	}
	if len(req.Facets) > limits.MaxFacets {
		return tooLarge("facets", len(req.Facets), limits.MaxFacets)
	}
	for _, facet := range req.Facets {
		if facet.Limit > limits.MaxFacetLimit {
			return tooLarge("facet_limit", facet.Limit, limits.MaxFacetLimit)
		}
		if facet.IdentityLimit > limits.MaxFacetIdentityLimit {
			return tooLarge("facet_identity_limit", facet.IdentityLimit, limits.MaxFacetIdentityLimit)
		}
	}
	if len(req.Highlight) > limits.MaxHighlightFields {
		return tooLarge("highlight", len(req.Highlight), limits.MaxHighlightFields)
	}
	if len(req.IncludeFields) > limits.MaxIncludeFields {
		return tooLarge("include_fields", len(req.IncludeFields), limits.MaxIncludeFields)
	}
	if len(req.Locales) > limits.MaxLocales {
		return tooLarge("locales", len(req.Locales), limits.MaxLocales)
	}
	if req.Semantic != nil {
		if req.Semantic.K > limits.MaxSemanticK {
			return tooLarge("semantic_k", req.Semantic.K, limits.MaxSemanticK)
		}
		if len(req.Semantic.QueryEmbedding) > limits.MaxEmbeddingDimensions {
			return tooLarge("embedding_dimensions", len(req.Semantic.QueryEmbedding), limits.MaxEmbeddingDimensions)
		}
	}
	return filtervalidate.ValidateWithLimits(req.Filters, filtervalidate.Limits{
		MaxDepth:      limits.MaxFilterDepth,
		MaxNodes:      limits.MaxFilterNodes,
		MaxListValues: limits.MaxFilterListValues,
	})
}

// ProviderSearch permits bounded internal candidate windows that are larger
// than a caller-facing page while retaining the global candidate ceiling.
func ProviderSearch(req types.SearchRequest, limits types.RequestLimits) error {
	limits = types.NormalizeRequestLimits(limits)
	limits.MaxPerPage = limits.MaxCandidateWindow
	return Search(req, limits)
}

func Suggest(req types.SuggestRequest, limits types.RequestLimits) error {
	limits = types.NormalizeRequestLimits(limits)
	if len(req.Query) > limits.MaxQueryBytes {
		return tooLarge("query", len(req.Query), limits.MaxQueryBytes)
	}
	if len(req.Indexes) > limits.MaxIndexes {
		return tooLarge("indexes", len(req.Indexes), limits.MaxIndexes)
	}
	if req.Limit > limits.MaxSuggestLimit {
		return tooLarge("suggest_limit", req.Limit, limits.MaxSuggestLimit)
	}
	return nil
}

func Batch(count int, limits types.RequestLimits) error {
	limits = types.NormalizeRequestLimits(limits)
	if count > limits.MaxBatchRequests {
		return tooLarge("batch_requests", count, limits.MaxBatchRequests)
	}
	return nil
}

func tooLarge(field string, actual, maximum int) error {
	return errs.InvalidInput(fmt.Sprintf("%s exceeds configured limit", field), map[string]any{
		"field": field, "actual": actual, "maximum": maximum,
	})
}
