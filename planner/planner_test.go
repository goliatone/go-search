package planner

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers/memory"
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

func TestPlannerRejectsUnsupportedSearchMode(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media"}, nil)
	p, err := New(Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	err = p.ValidateSearchCapabilities(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Mode:    types.SearchModeSemantic,
		Semantic: &types.SemanticRequest{
			Field: "body",
		},
	}, memory.New(memory.Config{}))
	if err == nil {
		t.Fatalf("expected unsupported capability error")
	}
}

func TestPlannerUsesConfiguredDefaults(t *testing.T) {
	registry := indexing.NewRegistry()
	_ = registry.Register(types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}, nil)
	p, err := New(Config{
		Registry: registry,
		Defaults: Defaults{
			SearchPage:                 2,
			SearchPerPage:              15,
			SuggestLimit:               7,
			DefaultSearchMode:          types.SearchModeHybrid,
			DisableIndexGroupByDefault: true,
		},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	searchPlan, err := p.BuildSearchPlan(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
	})
	if err != nil {
		t.Fatalf("build search plan: %v", err)
	}
	if searchPlan.Request.Page != 2 || searchPlan.Request.PerPage != 15 {
		t.Fatalf("expected configured pagination defaults, got %+v", searchPlan.Request)
	}
	if searchPlan.Request.Mode != types.SearchModeHybrid {
		t.Fatalf("expected configured default mode, got %s", searchPlan.Request.Mode)
	}
	if searchPlan.Request.GroupBy != "" {
		t.Fatalf("expected index group default to be disabled, got %q", searchPlan.Request.GroupBy)
	}
	suggestPlan, err := p.BuildSuggestPlan(context.Background(), types.SuggestRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
	})
	if err != nil {
		t.Fatalf("build suggest plan: %v", err)
	}
	if suggestPlan.Request.Limit != 7 {
		t.Fatalf("expected configured suggest limit, got %d", suggestPlan.Request.Limit)
	}
}
