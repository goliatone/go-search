package query

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers"
	"github.com/goliatone/go-search/ranking"
)

type SearchConfig struct {
	Planner       *planner.Planner
	Provider      providers.Provider
	Editorial     types.EditorialRuleStore
	RankingPolicy ranking.Policy
	Clock         types.Clock
}

type Search struct {
	planner       *planner.Planner
	provider      providers.Provider
	editorial     types.EditorialRuleStore
	rankingPolicy ranking.Policy
	clock         types.Clock
}

var _ gcommand.Querier[types.SearchRequest, types.SearchResultPage] = (*Search)(nil)

func NewSearch(cfg SearchConfig) (*Search, error) {
	if cfg.Planner == nil {
		return nil, errs.ConfigurationError("planner is required", nil)
	}
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.RankingPolicy == nil {
		cfg.RankingPolicy = ranking.NewDefaultPolicy()
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &Search{
		planner:       cfg.Planner,
		provider:      cfg.Provider,
		editorial:     cfg.Editorial,
		rankingPolicy: cfg.RankingPolicy,
		clock:         cfg.Clock,
	}, nil
}

func (q *Search) Query(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	plan, err := q.planner.BuildSearchPlan(ctx, req)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	if err := q.planner.ValidateSearchCapabilities(ctx, plan.Request, q.provider); err != nil {
		return types.SearchResultPage{}, err
	}
	rules := []types.EditorialRankRule{}
	if q.editorial != nil {
		rules, err = q.editorial.ListApplicable(ctx, plan.Request)
		if err != nil {
			return types.SearchResultPage{}, err
		}
	}
	requiresPost := requiresPostProcessing(plan.Request, rules, q.planner.ScopeGuard())
	providerReq := plan.ProviderRequest()
	if requiresPost {
		providerReq.Page = 1
		providerReq.PerPage = 0
		providerReq.GroupBy = ""
	}
	page, err := q.provider.Search(ctx, providerReq)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	page = annotateSearchPageLocales(page, plan.Locale)
	page = filterSearchPage(ctx, plan.Request, page, q.planner.ScopeGuard())
	if !requiresPost {
		return page, nil
	}
	page.Page = plan.Request.Page
	page.PerPage = plan.Request.PerPage
	page, err = q.rankingPolicy.Apply(plan.Request, page, rules, q.clock.Now())
	if err != nil {
		return types.SearchResultPage{}, errs.RankingFailure(err, map[string]any{"indexes": req.Indexes})
	}
	return page, nil
}

func requiresPostProcessing(req types.SearchRequest, rules []types.EditorialRankRule, guard types.ScopeGuard) bool {
	return req.GroupBy != "" || len(rules) > 0 || guard != nil
}
