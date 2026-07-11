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
		if plan.Profile != nil {
			providerReq.PerPage = candidateWindow(plan.Request, plan.Profile.Candidates)
		} else {
			providerReq.PerPage = 0
		}
		providerReq.GroupBy = ""
		if plan.Request.GroupBy != "" {
			providerReq.Facets = nil
		}
	}
	page, err := q.searchCandidates(ctx, providerReq, plan.Profile)
	if err != nil {
		observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
		return types.SearchResultPage{}, err
	}
	if requiresPost && plan.Profile != nil {
		page, err = q.refillCandidates(ctx, providerReq, page, plan.Profile.Candidates)
		if err != nil {
			return types.SearchResultPage{}, err
		}
	}
	page = annotateSearchPageLocales(page, plan.Locale)
	page = filterSearchPage(ctx, plan.Request, page, q.planner.ScopeGuard())
	if plan.Profile != nil {
		page.Hits = ranking.GroupEntities(page.Hits, 1)
		page.Total = len(page.Hits)
	}
	if !requiresPost {
		if page.DurationMS <= 0 {
			page.DurationMS = q.clock.Now().Sub(startedAt).Milliseconds()
		}
		finalizeSearchObservation(ctx, q.metrics, q.logger, startedAt, page, len(rules))
		return page, nil
	}
	page.Page = plan.Request.Page
	page.PerPage = plan.Request.PerPage
	if plan.Profile != nil {
		if page.Total >= providerReq.PerPage {
			page.TotalAccuracy = types.TotalAccuracyLowerBound
		} else {
			page.TotalAccuracy = types.TotalAccuracyExact
		}
	}
	rankedAt := q.clock.Now()
	var groupedFacets []types.SearchFacet
	if plan.Request.GroupBy != "" && len(plan.Request.Facets) > 0 {
		groupedFacets, err = q.buildGroupedDisjunctiveFacets(ctx, plan, rules, rankedAt)
		if err != nil {
			observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
			return types.SearchResultPage{}, err
		}
	}
	page, err = q.rankingPolicy.Apply(plan.Request, page, rules, rankedAt)
	if err != nil {
		observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
		return types.SearchResultPage{}, errs.RankingFailure(err, map[string]any{"indexes": req.Indexes})
	}
	if len(groupedFacets) > 0 {
		page.Facets = groupedFacets
	}
	if plan.Profile != nil {
		q.attachEvidence(ctx, plan.Request, &page)
	}
	if page.DurationMS <= 0 {
		page.DurationMS = q.clock.Now().Sub(startedAt).Milliseconds()
	}
	finalizeSearchObservation(ctx, q.metrics, q.logger, startedAt, page, len(rules))
	return page, nil
}

func (q *Search) attachEvidence(ctx context.Context, req types.SearchRequest, page *types.SearchResultPage) {
	aggregator, ok := q.provider.(providers.EvidenceAggregator)
	if !ok {
		if page.Metadata == nil {
			page.Metadata = map[string]any{}
		}
		page.Metadata["evidence_diagnostic"] = "provider does not support batched evidence"
		return
	}
	ids := make([]string, 0, len(page.Hits))
	for _, hit := range page.Hits {
		ids = append(ids, ranking.ResultID(hit))
	}
	summaries, err := aggregator.AggregateEvidence(ctx, types.EvidenceRequest{Search: req, ResultIDs: ids, MaxSamplesPerLocation: 3})
	if err != nil {
		if page.Metadata == nil {
			page.Metadata = map[string]any{}
		}
		page.Metadata["evidence_diagnostic"] = "aggregation_failed"
		return
	}
	for i := range page.Hits {
		summary, ok := summaries[ranking.ResultID(page.Hits[i])]
		if !ok || summary == nil || !summary.Exact {
			if page.Metadata == nil {
				page.Metadata = map[string]any{}
			}
			page.Metadata["evidence_diagnostic"] = "incomplete_aggregation"
			continue
		}
		page.Hits[i].Evidence = summary
	}
}

func (q *Search) searchCandidates(ctx context.Context, req types.SearchRequest, profile *ranking.RankingProfile) (types.SearchResultPage, error) {
	if profile == nil || len(req.Indexes) < 2 {
		return q.provider.Search(ctx, req)
	}
	requests := make([]types.SearchRequest, 0, len(req.Indexes))
	for _, index := range req.Indexes {
		one := req
		one.Indexes = []string{index}
		requests = append(requests, one)
	}
	pages, err := searchPages(ctx, q.provider, requests)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	lists := make([]ranking.RankedList, 0, len(pages))
	total := 0
	for i, page := range pages {
		weight := profile.Indexes[req.Indexes[i]].Weight
		lists = append(lists, ranking.RankedList{Index: req.Indexes[i], Weight: weight, Hits: page.Hits})
		total += page.Total
	}
	return types.SearchResultPage{Hits: ranking.FuseRRF(lists, 60), Page: req.Page, PerPage: req.PerPage, Total: total, Metadata: map[string]any{"fusion": "rrf"}}, nil
}

func (q *Search) refillCandidates(ctx context.Context, req types.SearchRequest, page types.SearchResultPage, cfg ranking.CandidateConfig) (types.SearchResultPage, error) {
	seen := len(page.Hits)
	rounds := 1
	for rounds < cfg.MaxRefillRounds && seen < cfg.MaxPerIndex && page.Total > seen {
		next := req
		next.Page = rounds + 1
		remaining := cfg.MaxPerIndex - seen
		if next.PerPage > remaining {
			next.PerPage = remaining
		}
		extra, err := q.provider.Search(ctx, next)
		if err != nil {
			return types.SearchResultPage{}, err
		}
		if len(extra.Hits) == 0 {
			break
		}
		page.Hits = append(page.Hits, extra.Hits...)
		if len(page.Hits) > cfg.MaxPerIndex {
			page.Hits = page.Hits[:cfg.MaxPerIndex]
		}
		seen += len(extra.Hits)
		if seen > cfg.MaxPerIndex {
			seen = cfg.MaxPerIndex
		}
		rounds++
	}
	if page.Metadata == nil {
		page.Metadata = map[string]any{}
	}
	page.Metadata["candidate_rounds"] = rounds
	page.Metadata["candidate_count"] = seen
	return page, nil
}

func candidateWindow(req types.SearchRequest, cfg ranking.CandidateConfig) int {
	window := req.Page * req.PerPage * cfg.Multiplier
	if window > cfg.MaxPerIndex {
		window = cfg.MaxPerIndex
	}
	if window < req.PerPage {
		window = req.PerPage
	}
	return window
}

func requiresPostProcessing(req types.SearchRequest, rules []types.EditorialRankRule, guard types.ScopeGuard) bool {
	return req.GroupBy != "" || len(rules) > 0 || guard != nil
}

func (q *Search) buildGroupedDisjunctiveFacets(ctx context.Context, plan planner.SearchPlan, rules []types.EditorialRankRule, now time.Time) ([]types.SearchFacet, error) {
	if len(plan.Request.Facets) == 0 {
		return nil, nil
	}
	baseProviderReq := plan.ProviderRequest()
	baseProviderReq.Page = 1
	if plan.Profile != nil {
		baseProviderReq.PerPage = plan.Profile.Candidates.MaxPerIndex
	} else {
		baseProviderReq.PerPage = 0
	}
	baseProviderReq.GroupBy = ""
	baseProviderReq.Facets = nil

	out := make([]types.SearchFacet, 0, len(plan.Request.Facets))
	countRequests := make([]types.SearchRequest, 0, len(plan.Request.Facets))
	providerRequests := make([]types.SearchRequest, 0, len(plan.Request.Facets))
	for _, facetReq := range plan.Request.Facets {
		countRequest := plan.Request
		if facetReq.Disjunctive {
			countRequest.Filters = types.RemoveFacetFilter(plan.Request.Filters, facetReq.Field)
		}
		providerReq := baseProviderReq
		providerReq.Filters = countRequest.Filters
		countRequests = append(countRequests, countRequest)
		providerRequests = append(providerRequests, providerReq)
	}
	pages, err := searchPages(ctx, q.provider, providerRequests)
	if err != nil {
		return nil, err
	}
	for i, facetReq := range plan.Request.Facets {
		page := pages[i]
		countRequest := countRequests[i]
		page = annotateSearchPageLocales(page, plan.Locale)
		hits := filterAuthorizedHits(ctx, countRequest.Actor, page.Hits, q.planner.ScopeGuard())
		hits = ranking.ApplyRulesToHits(countRequest, hits, rules, now)
		out = append(out, buildGroupedFacet(facetReq, plan.Request.Filters, hits))
	}
	return out, nil
}

func searchPages(ctx context.Context, provider providers.Provider, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	if batcher, ok := provider.(providers.SearchBatcher); ok {
		return batcher.SearchBatch(ctx, requests)
	}
	out := make([]types.SearchResultPage, 0, len(requests))
	for _, req := range requests {
		page, err := provider.Search(ctx, req)
		if err != nil {
			return nil, err
		}
		out = append(out, page)
	}
	return out, nil
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
