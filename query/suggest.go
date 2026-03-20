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
	Planner  *planner.Planner
	Provider providers.Provider
}

type Suggest struct {
	planner  *planner.Planner
	provider providers.Provider
}

var _ gcommand.Querier[types.SuggestRequest, types.SuggestResult] = (*Suggest)(nil)

func NewSuggest(cfg SuggestConfig) (*Suggest, error) {
	if cfg.Planner == nil {
		return nil, errs.ConfigurationError("planner is required", nil)
	}
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	return &Suggest{planner: cfg.Planner, provider: cfg.Provider}, nil
}

func (q *Suggest) Query(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	plan, err := q.planner.BuildSuggestPlan(ctx, req)
	if err != nil {
		return types.SuggestResult{}, err
	}
	return q.provider.Suggest(ctx, plan.Request)
}
