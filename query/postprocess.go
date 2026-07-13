package query

import (
	"context"

	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/ranking"
)

func filterSearchPage(ctx context.Context, req types.SearchRequest, page types.SearchResultPage, guard types.ScopeGuard) types.SearchResultPage {
	if guard == nil {
		return page
	}
	if len(page.Groups) > 0 {
		originalHitCount := groupedHitCount(page.Groups)
		page.Groups = filterAuthorizedGroups(ctx, req.Actor, page.Groups, guard)
		page.Hits = flattenGroupHits(page.Groups)
		if len(page.Hits) == originalHitCount {
			// Provider facets and totals describe the full filtered result set, while
			// the returned hits are only a bounded page. When the guard removes
			// nothing, retain that complete information (including entity identity
			// sets and accuracy) instead of rebuilding it from the page window.
			return page
		}
		page.Total = len(page.Groups)
		page.Facets = buildFacets(req.Facets, req.Filters, page.Hits)
		page.TotalAccuracy = types.TotalAccuracyLowerBound
		return page
	}
	originalHitCount := len(page.Hits)
	page.Hits = filterAuthorizedHits(ctx, req.Actor, page.Hits, guard)
	if len(page.Hits) == originalHitCount {
		return page
	}
	page.Total = len(page.Hits)
	page.Facets = buildFacets(req.Facets, req.Filters, page.Hits)
	page.TotalAccuracy = types.TotalAccuracyLowerBound
	return page
}

func groupedHitCount(groups []types.SearchGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Hits)
	}
	return total
}

func filterSuggestResult(ctx context.Context, actor types.ActorRef, result types.SuggestResult, guard types.ScopeGuard, limit int) types.SuggestResult {
	if guard == nil {
		return limitSuggests(result, limit)
	}
	items := make([]types.SuggestHit, 0, len(result.Items))
	for _, item := range result.Items {
		if !canAccessDocument(ctx, actor, item.Document, guard) {
			continue
		}
		items = append(items, item)
	}
	result.Items = items
	return limitSuggests(result, limit)
}

func filterAuthorizedGroups(ctx context.Context, actor types.ActorRef, groups []types.SearchGroup, guard types.ScopeGuard) []types.SearchGroup {
	out := make([]types.SearchGroup, 0, len(groups))
	for _, group := range groups {
		group.Hits = filterAuthorizedHits(ctx, actor, group.Hits, guard)
		if len(group.Hits) == 0 {
			continue
		}
		top := group.Hits[0]
		group.TopHit = &top
		group.Count = len(group.Hits)
		out = append(out, group)
	}
	return out
}

func filterAuthorizedHits(ctx context.Context, actor types.ActorRef, hits []types.SearchHit, guard types.ScopeGuard) []types.SearchHit {
	out := make([]types.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if !canAccessDocument(ctx, actor, hit.Document, guard) {
			continue
		}
		out = append(out, hit)
	}
	return out
}

func canAccessDocument(ctx context.Context, actor types.ActorRef, doc *types.Document, guard types.ScopeGuard) bool {
	if guard == nil {
		return true
	}
	if doc == nil {
		return false
	}
	return guard.AllowDocument(ctx, actor, doc.Clone())
}

func buildFacets(requests []types.FacetRequest, filters types.FilterExpr, hits []types.SearchHit) []types.SearchFacet {
	if len(requests) == 0 {
		return nil
	}
	out := make([]types.SearchFacet, 0, len(requests))
	for _, facetReq := range requests {
		if facetReq.CountBy == types.FacetCountByResultID {
			facet := types.BuildEntityFacet(facetReq, entityFacetIdentities(facetReq.Field, hits), types.SelectedFacetValues(filters, facetReq.Field))
			// Guard filtering only sees the bounded candidate window. Its rebuilt
			// identity set is therefore a safe lower bound, never an exact global
			// count.
			facet.Accuracy = types.FacetCountAccuracyLowerBound
			out = append(out, facet)
			continue
		}
		counts := map[string]int{}
		for _, hit := range hits {
			if hit.Document == nil {
				continue
			}
			for _, value := range hit.Document.Facets[facetReq.Field] {
				counts[value]++
			}
		}
		out = append(out, types.BuildFacet(facetReq, counts, types.SelectedFacetValues(filters, facetReq.Field)))
	}
	return out
}

func entityFacetIdentities(field string, hits []types.SearchHit) map[string]map[string]struct{} {
	identities := map[string]map[string]struct{}{}
	for _, hit := range hits {
		if hit.Document == nil {
			continue
		}
		resultID := ranking.ResultID(hit)
		if resultID == "" {
			resultID = hit.Document.ID
		}
		if resultID == "" {
			continue
		}
		for _, value := range hit.Document.Facets[field] {
			set := identities[value]
			if set == nil {
				set = map[string]struct{}{}
				identities[value] = set
			}
			set[resultID] = struct{}{}
		}
	}
	return identities
}

func buildGroupedFacet(request types.FacetRequest, filters types.FilterExpr, hits []types.SearchHit) types.SearchFacet {
	return types.BuildFacet(request, groupedFacetCounts(request.Field, hits), types.SelectedFacetValues(filters, request.Field))
}

func groupedFacetCounts(field string, hits []types.SearchHit) map[string]int {
	counts := map[string]int{}
	for _, group := range ranking.GroupHits(hits) {
		seen := map[string]struct{}{}
		for _, hit := range group.Hits {
			if hit.Document == nil {
				continue
			}
			for _, value := range hit.Document.Facets[field] {
				if _, ok := seen[value]; ok {
					continue
				}
				seen[value] = struct{}{}
				counts[value]++
			}
		}
	}
	return counts
}

func flattenGroupHits(groups []types.SearchGroup) []types.SearchHit {
	out := make([]types.SearchHit, 0)
	for _, group := range groups {
		out = append(out, group.Hits...)
	}
	return out
}

func limitSuggests(result types.SuggestResult, limit int) types.SuggestResult {
	if limit > 0 && len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	return result
}

func annotateSearchPageLocales(page types.SearchResultPage, localePlan planner.LocalePlan) types.SearchResultPage {
	for i := range page.Hits {
		annotateHitLocale(&page.Hits[i], localePlan)
	}
	for i := range page.Groups {
		for j := range page.Groups[i].Hits {
			annotateHitLocale(&page.Groups[i].Hits[j], localePlan)
		}
		if page.Groups[i].TopHit != nil {
			top := *page.Groups[i].TopHit
			annotateHitLocale(&top, localePlan)
			page.Groups[i].TopHit = &top
		}
	}
	return page
}

func annotateHitLocale(hit *types.SearchHit, localePlan planner.LocalePlan) {
	if hit == nil {
		return
	}
	if hit.Retrieval == nil {
		hit.Retrieval = &types.AppliedRetrievalSignals{Metadata: map[string]any{}}
	}
	if hit.Retrieval.Metadata == nil {
		hit.Retrieval.Metadata = map[string]any{}
	}
	if _, ok := hit.Retrieval.Metadata["locale_match"]; !ok {
		hit.Retrieval.Metadata["locale_match"] = localePlan.MatchLabel(hit.Locale)
	}
	if _, ok := hit.Retrieval.Metadata["exact_locale"]; !ok {
		hit.Retrieval.Metadata["exact_locale"] = localePlan.IsExact(hit.Locale)
	}
	if _, ok := hit.Retrieval.Metadata["locale_origin"]; !ok {
		if origin := localePlan.Origin(hit.Locale); origin != "" {
			hit.Retrieval.Metadata["locale_origin"] = origin
		}
	}
}
