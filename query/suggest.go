package query

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers"
)

type SuggestConfig struct {
	Planner                    *planner.Planner
	Provider                   providers.Provider
	ScopeGuardFetchMultiplier  int
	MinimumScopeGuardFetchSize int
}

type Suggest struct {
	planner                   *planner.Planner
	provider                  providers.Provider
	scopeGuardFetchMultiplier int
	minimumScopeFetchSize     int
}

var _ gcommand.Querier[types.SuggestRequest, types.SuggestResult] = (*Suggest)(nil)

func NewSuggest(cfg SuggestConfig) (*Suggest, error) {
	if cfg.Planner == nil {
		return nil, errs.ConfigurationError("planner is required", nil)
	}
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.ScopeGuardFetchMultiplier <= 0 {
		cfg.ScopeGuardFetchMultiplier = 4
	}
	if cfg.MinimumScopeGuardFetchSize <= 0 {
		cfg.MinimumScopeGuardFetchSize = 20
	}
	return &Suggest{
		planner:                   cfg.Planner,
		provider:                  cfg.Provider,
		scopeGuardFetchMultiplier: cfg.ScopeGuardFetchMultiplier,
		minimumScopeFetchSize:     cfg.MinimumScopeGuardFetchSize,
	}, nil
}

func (q *Suggest) Query(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	plan, err := q.planner.BuildSuggestPlan(ctx, req)
	if err != nil {
		return types.SuggestResult{}, err
	}
	providerReq := plan.Request
	if q.planner.ScopeGuard() != nil {
		providerReq.Limit = max(plan.Request.Limit*q.scopeGuardFetchMultiplier, q.minimumScopeFetchSize)
	}
	result, err := q.provider.Suggest(ctx, providerReq)
	if err != nil {
		return types.SuggestResult{}, err
	}
	return filterSuggestResult(ctx, plan.Request.Actor, result, q.planner.ScopeGuard(), plan.Request.Limit), nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
