package planner

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/pkg/types"
)

func TestPlannerNormalizesAndValidates(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}, nil)
	p, err := New(Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	req := types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "  prayer ",
		Page:    0,
		PerPage: 0,
		Sort:    []types.Sort{{Field: "title", Direction: types.SortAsc}},
		Filters: types.AndExpr{Terms: []types.FilterExpr{types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: "archive"}}},
	}
	plan, err := p.BuildSearchPlan(context.Background(), req)
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if plan.Request.Page != 1 || plan.Request.PerPage != 20 {
		t.Fatalf("expected default pagination, got page=%d perPage=%d", plan.Request.Page, plan.Request.PerPage)
	}
	if plan.Request.GroupBy != "parent_id" {
		t.Fatalf("expected default group by to be applied")
	}
}

func TestPlannerRejectsInvalidFilter(t *testing.T) {
	err := ValidateFilter(types.TermExpr{Field: "", Op: types.FilterOpEQ, Value: "x"})
	if err == nil {
		t.Fatalf("expected invalid filter error")
	}
}
