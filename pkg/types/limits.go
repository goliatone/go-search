package types

// RequestLimits bounds caller-controlled search complexity. Zero values in a
// configuration are replaced by DefaultRequestLimits.
type RequestLimits struct {
	MaxQueryBytes          int `json:"max_query_bytes"`
	MaxIndexes             int `json:"max_indexes"`
	MaxPage                int `json:"max_page"`
	MaxPerPage             int `json:"max_per_page"`
	MaxCandidateWindow     int `json:"max_candidate_window"`
	MaxSuggestLimit        int `json:"max_suggest_limit"`
	MaxBatchRequests       int `json:"max_batch_requests"`
	MaxSortFields          int `json:"max_sort_fields"`
	MaxFacets              int `json:"max_facets"`
	MaxFacetLimit          int `json:"max_facet_limit"`
	MaxFacetIdentityLimit  int `json:"max_facet_identity_limit"`
	MaxFilterDepth         int `json:"max_filter_depth"`
	MaxFilterNodes         int `json:"max_filter_nodes"`
	MaxFilterListValues    int `json:"max_filter_list_values"`
	MaxHighlightFields     int `json:"max_highlight_fields"`
	MaxIncludeFields       int `json:"max_include_fields"`
	MaxLocales             int `json:"max_locales"`
	MaxSemanticK           int `json:"max_semantic_k"`
	MaxEmbeddingDimensions int `json:"max_embedding_dimensions"`
}

func DefaultRequestLimits() RequestLimits {
	return RequestLimits{
		MaxQueryBytes:          4096,
		MaxIndexes:             16,
		MaxPage:                1000,
		MaxPerPage:             100,
		MaxCandidateWindow:     10000,
		MaxSuggestLimit:        50,
		MaxBatchRequests:       32,
		MaxSortFields:          8,
		MaxFacets:              32,
		MaxFacetLimit:          100,
		MaxFacetIdentityLimit:  10000,
		MaxFilterDepth:         16,
		MaxFilterNodes:         256,
		MaxFilterListValues:    256,
		MaxHighlightFields:     32,
		MaxIncludeFields:       64,
		MaxLocales:             16,
		MaxSemanticK:           1000,
		MaxEmbeddingDimensions: 4096,
	}
}

func NormalizeRequestLimits(in RequestLimits) RequestLimits {
	out := DefaultRequestLimits()
	applyPositive := func(target *int, value int) {
		if value > 0 {
			*target = value
		}
	}
	applyPositive(&out.MaxQueryBytes, in.MaxQueryBytes)
	applyPositive(&out.MaxIndexes, in.MaxIndexes)
	applyPositive(&out.MaxPage, in.MaxPage)
	applyPositive(&out.MaxPerPage, in.MaxPerPage)
	applyPositive(&out.MaxCandidateWindow, in.MaxCandidateWindow)
	applyPositive(&out.MaxSuggestLimit, in.MaxSuggestLimit)
	applyPositive(&out.MaxBatchRequests, in.MaxBatchRequests)
	applyPositive(&out.MaxSortFields, in.MaxSortFields)
	applyPositive(&out.MaxFacets, in.MaxFacets)
	applyPositive(&out.MaxFacetLimit, in.MaxFacetLimit)
	applyPositive(&out.MaxFacetIdentityLimit, in.MaxFacetIdentityLimit)
	applyPositive(&out.MaxFilterDepth, in.MaxFilterDepth)
	applyPositive(&out.MaxFilterNodes, in.MaxFilterNodes)
	applyPositive(&out.MaxFilterListValues, in.MaxFilterListValues)
	applyPositive(&out.MaxHighlightFields, in.MaxHighlightFields)
	applyPositive(&out.MaxIncludeFields, in.MaxIncludeFields)
	applyPositive(&out.MaxLocales, in.MaxLocales)
	applyPositive(&out.MaxSemanticK, in.MaxSemanticK)
	applyPositive(&out.MaxEmbeddingDimensions, in.MaxEmbeddingDimensions)
	return out
}
