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
			providerReq.PerPage = candidateWindow(plan.Request, defaultCandidateConfig())
		}
		providerReq.GroupBy = ""
		if plan.Request.GroupBy != "" {
			providerReq.Facets = nil
		}
	}
	page, candidates, err := q.searchCandidates(ctx, providerReq, plan.Profile)
	if err != nil {
		observe.Count(ctx, q.metrics, q.logger, "search.query.error.count", 1, labels)
		return types.SearchResultPage{}, err
	}
	if requiresPost && plan.Profile != nil {
		page, err = q.refillCandidates(ctx, providerReq, candidates, plan.Profile.Candidates)
		if err != nil {
			return types.SearchResultPage{}, err
		}
	} else if requiresPost {
		page = normalizeLegacyCandidatePage(page, providerReq.PerPage)
	}
	page = annotateSearchPageLocales(page, plan.Locale)
	page = filterSearchPage(ctx, plan.Request, page, q.planner.ScopeGuard())
	if plan.Profile == nil && plan.Request.GroupBy == "" && len(rules) == 0 && scopeGuardRequiresCandidateExpansion(q.planner.ScopeGuard()) {
		page.Page = plan.Request.Page
		page.PerPage = plan.Request.PerPage
		page.Hits = ranking.PaginateHits(page.Hits, plan.Request.Page, plan.Request.PerPage)
		if page.DurationMS <= 0 {
			page.DurationMS = q.clock.Now().Sub(startedAt).Milliseconds()
		}
		finalizeSearchObservation(ctx, q.metrics, q.logger, startedAt, page, 0)
		return page, nil
	}
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
		setEvidenceStatus(page.Hits, types.EvidenceStatusUnsupported, "provider does not support batched evidence")
		return
	}
	ids := make([]string, 0, len(page.Hits))
	for _, hit := range page.Hits {
		ids = append(ids, ranking.ResultID(hit))
	}
	summaries, err := aggregator.AggregateEvidence(ctx, types.EvidenceRequest{Search: req, ResultIDs: ids, MaxSamplesPerLocation: 3, Guard: q.planner.ScopeGuard()})
	if err != nil {
		if page.Metadata == nil {
			page.Metadata = map[string]any{}
		}
		page.Metadata["evidence_diagnostic"] = "aggregation_failed"
		setEvidenceStatus(page.Hits, types.EvidenceStatusUnavailable, "aggregation_failed")
		return
	}
	for i := range page.Hits {
		summary, ok := summaries[ranking.ResultID(page.Hits[i])]
		if !ok || summary == nil {
			if page.Metadata == nil {
				page.Metadata = map[string]any{}
			}
			page.Metadata["evidence_diagnostic"] = "incomplete_aggregation"
			page.Hits[i].Evidence = &types.MatchEvidenceSummary{Status: types.EvidenceStatusPartial, Diagnostic: "incomplete_aggregation"}
			continue
		}
		if summary.Status == "" {
			if summary.Exact {
				summary.Status = types.EvidenceStatusComplete
			} else {
				summary.Status = types.EvidenceStatusPartial
				if summary.Diagnostic == "" {
					summary.Diagnostic = "incomplete_aggregation"
				}
			}
		}
		page.Hits[i].Evidence = summary
	}
}

func setEvidenceStatus(hits []types.SearchHit, status types.EvidenceStatus, diagnostic string) {
	for i := range hits {
		hits[i].Evidence = &types.MatchEvidenceSummary{Status: status, Diagnostic: diagnostic}
	}
}

type candidateIndexState struct {
	index         string
	hits          []types.SearchHit
	total         int
	fetched       int
	rounds        int
	exhausted     bool
	providerExact bool
}

type candidateSearchState struct {
	profile *ranking.RankingProfile
	indexes []*candidateIndexState
}

func (q *Search) searchCandidates(ctx context.Context, req types.SearchRequest, profile *ranking.RankingProfile) (types.SearchResultPage, *candidateSearchState, error) {
	if profile == nil {
		page, err := q.provider.Search(ctx, req)
		return page, nil, err
	}
	requests := make([]types.SearchRequest, 0, len(req.Indexes))
	for _, index := range req.Indexes {
		one := req
		one.Indexes = []string{index}
		requests = append(requests, one)
	}
	pages, err := searchPages(ctx, q.provider, requests)
	if err != nil {
		return types.SearchResultPage{}, nil, err
	}
	state := &candidateSearchState{profile: profile, indexes: make([]*candidateIndexState, 0, len(pages))}
	retentionCap := profile.Candidates.MaxPerIndex
	if retentionCap < 1 {
		retentionCap = max(1, req.PerPage)
	}
	for i, page := range pages {
		hits := page.Hits
		truncated := false
		if len(hits) > retentionCap {
			hits = hits[:retentionCap]
			truncated = true
		}
		fetched := len(hits)
		providerExact := candidateTotalExact(page.TotalAccuracy)
		state.indexes = append(state.indexes, &candidateIndexState{
			index: req.Indexes[i], hits: append([]types.SearchHit(nil), hits...), total: page.Total,
			fetched: fetched, rounds: 1, providerExact: providerExact,
			exhausted: providerExact && !truncated && (fetched >= page.Total || fetched < req.PerPage),
		})
	}
	return renderCandidateState(req, state), state, nil
}

func (q *Search) refillCandidates(ctx context.Context, req types.SearchRequest, state *candidateSearchState, cfg ranking.CandidateConfig) (types.SearchResultPage, error) {
	if state == nil {
		return types.SearchResultPage{}, errs.ConfigurationError("candidate state is required for profiled refill", nil)
	}
	for round := 2; round <= cfg.MaxRefillRounds; round++ {
		requests := make([]types.SearchRequest, 0, len(state.indexes))
		targets := make([]*candidateIndexState, 0, len(state.indexes))
		for _, index := range state.indexes {
			if index.exhausted || index.fetched >= cfg.MaxPerIndex {
				continue
			}
			next := req
			next.Indexes = []string{index.index}
			next.Page = index.rounds + 1
			// Keep the original page size stable: offset-based providers derive the
			// page offset from it. The retained candidate slice is capped below.
			next.PerPage = req.PerPage
			requests = append(requests, next)
			targets = append(targets, index)
		}
		if len(requests) == 0 {
			break
		}
		pages, err := searchPages(ctx, q.provider, requests)
		if err != nil {
			return types.SearchResultPage{}, err
		}
		for i, page := range pages {
			index := targets[i]
			index.rounds++
			index.total = page.Total
			index.providerExact = index.providerExact && candidateTotalExact(page.TotalAccuracy)
			remaining := cfg.MaxPerIndex - index.fetched
			extra := page.Hits
			if len(extra) > remaining {
				extra = extra[:remaining]
			}
			index.hits = append(index.hits, extra...)
			index.fetched += len(extra)
			index.exhausted = index.providerExact && (len(page.Hits) < requests[i].PerPage || index.fetched >= page.Total)
		}
	}
	return renderCandidateState(req, state), nil
}

func renderCandidateState(req types.SearchRequest, state *candidateSearchState) types.SearchResultPage {
	lists := make([]ranking.RankedList, 0, len(state.indexes))
	exact := true
	maxRounds, candidateCount := 0, 0
	for _, index := range state.indexes {
		grouped := ranking.GroupEntities(index.hits, 1)
		lists = append(lists, ranking.RankedList{Index: index.index, Weight: state.profile.Indexes[index.index].Weight, Hits: grouped})
		exact = exact && index.providerExact && index.exhausted
		maxRounds = max(maxRounds, index.rounds)
		candidateCount += index.fetched
	}
	var hits []types.SearchHit
	if len(lists) == 1 {
		hits = lists[0].Hits
	} else {
		hits = ranking.FuseRRF(lists, 60)
	}
	accuracy := types.TotalAccuracyLowerBound
	if exact {
		accuracy = types.TotalAccuracyExact
	}
	return types.SearchResultPage{Hits: hits, Page: req.Page, PerPage: req.PerPage, Total: len(hits), TotalAccuracy: accuracy, Metadata: map[string]any{"fusion": "rrf", "candidate_rounds": maxRounds, "candidate_count": candidateCount}}
}

func candidateTotalExact(accuracy types.TotalAccuracy) bool {
	// Empty is the legacy provider contract and remains exact unless the provider
	// explicitly declares another accuracy. Unknown future values remain
	// conservative instead of being promoted to exact.
	return accuracy == "" || accuracy == types.TotalAccuracyExact
}

func normalizeLegacyCandidatePage(page types.SearchResultPage, window int) types.SearchResultPage {
	fetched := len(page.Hits)
	exhausted := candidatePageExhausted(page, window)
	page.TotalAccuracy = types.TotalAccuracyLowerBound
	if exhausted {
		page.TotalAccuracy = types.TotalAccuracyExact
	}
	if page.Metadata == nil {
		page.Metadata = map[string]any{}
	}
	page.Metadata["candidate_rounds"] = 1
	page.Metadata["candidate_count"] = fetched
	page.Metadata["candidate_window"] = window
	return page
}

func candidatePageExhausted(page types.SearchResultPage, window int) bool {
	return candidateTotalExact(page.TotalAccuracy) && (len(page.Hits) >= page.Total || len(page.Hits) < window)
}

func defaultCandidateConfig() ranking.CandidateConfig {
	return ranking.CandidateConfig{Multiplier: 5, MaxPerIndex: 250, MaxRefillRounds: 2}
}

func candidateWindow(req types.SearchRequest, cfg ranking.CandidateConfig) int {
	cap := max(1, cfg.MaxPerIndex)
	page := max(1, req.Page)
	perPage := max(1, req.PerPage)
	multiplier := max(1, cfg.Multiplier)
	if perPage >= cap || page > cap/perPage {
		return cap
	}
	window := page * perPage
	if multiplier > cap/window {
		return cap
	}
	return min(window*multiplier, cap)
}

func requiresPostProcessing(req types.SearchRequest, rules []types.EditorialRankRule, guard types.ScopeGuard) bool {
	return req.GroupBy != "" || len(rules) > 0 || scopeGuardRequiresCandidateExpansion(guard)
}

func scopeGuardRequiresCandidateExpansion(guard types.ScopeGuard) bool {
	if guard == nil {
		return false
	}
	if policy, ok := guard.(interface{ RequiresCandidateExpansion() bool }); ok {
		return policy.RequiresCandidateExpansion()
	}
	return true
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
		baseProviderReq.PerPage = candidateWindow(plan.Request, defaultCandidateConfig())
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
		facet := buildGroupedFacet(facetReq, plan.Request.Filters, hits)
		if !candidatePageExhausted(page, baseProviderReq.PerPage) {
			facet.Accuracy = types.FacetCountAccuracyLowerBound
			for j := range facet.Values {
				facet.Values[j].EntityIDsComplete = false
			}
		}
		out = append(out, facet)
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
