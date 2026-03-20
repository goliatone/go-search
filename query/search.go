package query

import (
	"context"
	"time"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/internal/observe"
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
	Logger        types.Logger
	Metrics       []types.MetricsHook
	Clock         types.Clock
}

type Search struct {
	planner       *planner.Planner
	provider      providers.Provider
	editorial     types.EditorialRuleStore
	rankingPolicy ranking.Policy
	logger        types.Logger
	metrics       []types.MetricsHook
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
		logger:        cfg.Logger,
		metrics:       cfg.Metrics,
		clock:         cfg.Clock,
	}, nil
}

func (q *Search) Query(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	startedAt := q.clock.Now()
	labels := map[string]string{
		"grouped": boolLabel(req.GroupBy != ""),
	}
	plan, err := q.planner.BuildSearchPlan(ctx, req)
	if err != nil {
		observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
		return types.SearchResultPage{}, err
	}
	if err := q.planner.ValidateSearchCapabilities(ctx, plan.Request, q.provider); err != nil {
		observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
		return types.SearchResultPage{}, err
	}
	rules := []types.EditorialRankRule{}
	if q.editorial != nil {
		rules, err = q.editorial.ListApplicable(ctx, plan.Request)
		if err != nil {
			observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
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
		observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
		return types.SearchResultPage{}, err
	}
	page = annotateSearchPageLocales(page, plan.Locale)
	page = filterSearchPage(ctx, plan.Request, page, q.planner.ScopeGuard())
	if !requiresPost {
		if page.DurationMS <= 0 {
			page.DurationMS = q.clock.Now().Sub(startedAt).Milliseconds()
		}
		finalizeSearchObservation(ctx, q.metrics, q.logger, startedAt, page, len(rules))
		return page, nil
	}
	page.Page = plan.Request.Page
	page.PerPage = plan.Request.PerPage
	page, err = q.rankingPolicy.Apply(plan.Request, page, rules, q.clock.Now())
	if err != nil {
		observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
		return types.SearchResultPage{}, errs.RankingFailure(err, map[string]any{"indexes": req.Indexes})
	}
	if page.DurationMS <= 0 {
		page.DurationMS = q.clock.Now().Sub(startedAt).Milliseconds()
	}
	finalizeSearchObservation(ctx, q.metrics, q.logger, startedAt, page, len(rules))
	return page, nil
}

func requiresPostProcessing(req types.SearchRequest, rules []types.EditorialRankRule, guard types.ScopeGuard) bool {
	return req.GroupBy != "" || len(rules) > 0 || guard != nil
}

func finalizeSearchObservation(ctx context.Context, metrics []types.MetricsHook, logger types.Logger, startedAt time.Time, page types.SearchResultPage, ruleCount int) {
	labels := map[string]string{
		"grouped": boolLabel(len(page.Groups) > 0),
	}
	observe.Count(ctx, metrics, logger, "search.query.count", 1, labels)
	observe.Count(ctx, metrics, logger, "search.query.rules.count", int64(ruleCount), labels)
	observe.Count(ctx, metrics, logger, "search.query.grouped_result.count", int64(len(page.Groups)), labels)
	observe.ObserveDuration(ctx, metrics, logger, "search.query.duration_ms", startedAt, labels)
	observe.Info(logger, "search.query.completed", map[string]any{
		"duration_ms": page.DurationMS,
		"groups":      len(page.Groups),
		"hits":        len(page.Hits),
		"rules":       ruleCount,
		"total":       page.Total,
	})
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
