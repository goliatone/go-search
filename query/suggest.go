package query

import (
	"context"
	"time"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers"
	"github.com/goliatone/go-search/ranking"
)

type SuggestConfig struct {
	Editorial                  types.EditorialRuleStore
	Planner                    *planner.Planner
	Provider                   providers.Provider
	ScopeGuardFetchMultiplier  int
	MinimumScopeGuardFetchSize int
}

type Suggest struct {
	editorial                 types.EditorialRuleStore
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
		editorial:                 cfg.Editorial,
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
	providerReq := plan.ProviderRequest()
	if q.planner.ScopeGuard() != nil {
		providerReq.Limit = max(plan.Request.Limit*q.scopeGuardFetchMultiplier, q.minimumScopeFetchSize)
	}
	result, err := q.provider.Suggest(ctx, providerReq)
	if err != nil {
		return types.SuggestResult{}, err
	}
	result = filterSuggestResult(ctx, plan.Request.Actor, result, q.planner.ScopeGuard(), 0)
	if q.editorial != nil && len(result.Items) > 0 {
		rules, err := q.editorial.ListApplicable(ctx, types.SearchRequest{
			Indexes:        plan.Request.Indexes,
			Query:          plan.Request.Query,
			Locale:         plan.Request.Locale,
			RankingProfile: "",
			Metadata:       plan.Request.Metadata,
			Actor:          plan.Request.Actor,
			Scope:          plan.Request.Scope,
			Request:        plan.Request,
		})
		if err != nil {
			return types.SuggestResult{}, err
		}
		if len(rules) > 0 {
			result = applyEditorialToSuggestResult(plan.Request, result, rules, time.Now())
		}
	}
	return limitSuggests(result, plan.Request.Limit), nil
}

func applyEditorialToSuggestResult(req types.SuggestRequest, result types.SuggestResult, rules []types.EditorialRankRule, now time.Time) types.SuggestResult {
	hits := make([]types.SearchHit, 0, len(result.Items))
	for _, item := range result.Items {
		hits = append(hits, suggestHitToSearchHit(item))
	}
	ranked := ranking.ApplyRulesToHits(types.SearchRequest{
		Indexes: planRequestIndexes(req),
		Query:   req.Query,
		Locale:  req.Locale,
		Actor:   req.Actor,
		Scope:   req.Scope,
		Request: req,
	}, hits, rules, now)
	items := make([]types.SuggestHit, 0, len(ranked))
	for _, hit := range ranked {
		items = append(items, searchHitToSuggestHit(hit))
	}
	result.Items = items
	return result
}

func suggestHitToSearchHit(item types.SuggestHit) types.SearchHit {
	parent := item.Parent
	if parent == nil && item.ID != "" {
		parent = &types.SearchParent{
			ID:    item.ID,
			Type:  item.Type,
			Title: item.Title,
			URL:   item.URL,
		}
	}
	return types.SearchHit{
		ID:         item.ID,
		Type:       item.Type,
		Title:      item.Title,
		URL:        item.URL,
		Locale:     item.Locale,
		Score:      item.Score,
		BaseScore:  item.Score,
		FinalScore: item.Score,
		Parent:     parent,
		Document:   item.Document,
	}
}

func searchHitToSuggestHit(hit types.SearchHit) types.SuggestHit {
	return types.SuggestHit{
		ID:       hit.ID,
		Type:     hit.Type,
		Title:    hit.Title,
		URL:      hit.URL,
		Locale:   hit.Locale,
		Score:    hit.FinalScore,
		Parent:   hit.Parent,
		Document: hit.Document,
	}
}

func planRequestIndexes(req types.SuggestRequest) []string {
	if len(req.Indexes) == 0 {
		return nil
	}
	return append([]string(nil), req.Indexes...)
}
