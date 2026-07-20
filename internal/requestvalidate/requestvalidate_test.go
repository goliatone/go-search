package requestvalidate

import (
	"strings"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

func TestSearchRejectsCallerControlledComplexity(t *testing.T) {
	limits := types.DefaultRequestLimits()
	tests := []types.SearchRequest{
		{Query: strings.Repeat("x", limits.MaxQueryBytes+1)},
		{Indexes: make([]string, limits.MaxIndexes+1)},
		{Page: limits.MaxPage + 1, PerPage: 1},
		{Page: 2, PerPage: limits.MaxCandidateWindow},
		{Facets: []types.FacetRequest{{Field: "topic", Limit: limits.MaxFacetLimit + 1}}},
		{Semantic: &types.SemanticRequest{QueryEmbedding: make([]float32, limits.MaxEmbeddingDimensions+1)}},
	}
	for i, req := range tests {
		if err := Search(req, limits); err == nil {
			t.Fatalf("case %d: expected limit error", i)
		}
	}
}

func TestSearchRejectsDeepAndWideFilters(t *testing.T) {
	limits := types.DefaultRequestLimits()
	expr := types.FilterExpr(types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: "x"})
	for range limits.MaxFilterDepth {
		expr = types.NotExpr{Term: expr}
	}
	if err := Search(types.SearchRequest{Filters: expr}, limits); err == nil {
		t.Fatal("expected filter depth error")
	}
	values := make([]string, limits.MaxFilterListValues+1)
	if err := Search(types.SearchRequest{Filters: types.TermExpr{Field: "topic", Op: types.FilterOpIn, Value: values}}, limits); err == nil {
		t.Fatal("expected filter list error")
	}
}
