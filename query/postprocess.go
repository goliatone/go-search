package query

import (
	"context"
	"sort"

	"github.com/goliatone/go-search/pkg/types"
)

func filterSearchPage(ctx context.Context, req types.SearchRequest, page types.SearchResultPage, guard types.ScopeGuard) types.SearchResultPage {
	if guard == nil {
		return page
	}
	if len(page.Groups) > 0 {
		page.Groups = filterAuthorizedGroups(ctx, req.Actor, page.Groups, guard)
		page.Hits = flattenGroupHits(page.Groups)
		page.Total = len(page.Groups)
		page.Facets = buildFacets(req.Facets, page.Hits)
		return page
	}
	page.Hits = filterAuthorizedHits(ctx, req.Actor, page.Hits, guard)
	page.Total = len(page.Hits)
	page.Facets = buildFacets(req.Facets, page.Hits)
	return page
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

func buildFacets(requests []types.FacetRequest, hits []types.SearchHit) []types.SearchFacet {
	if len(requests) == 0 {
		return nil
	}
	out := make([]types.SearchFacet, 0, len(requests))
	for _, facetReq := range requests {
		counts := map[string]int{}
		for _, hit := range hits {
			if hit.Document == nil {
				continue
			}
			for _, value := range hit.Document.Facets[facetReq.Field] {
				counts[value]++
			}
		}
		values := make([]types.SearchFacetValue, 0, len(counts))
		for value, count := range counts {
			values = append(values, types.SearchFacetValue{Value: value, Count: count})
		}
		sort.SliceStable(values, func(i, j int) bool {
			if values[i].Count == values[j].Count {
				return values[i].Value < values[j].Value
			}
			return values[i].Count > values[j].Count
		})
		if facetReq.Limit > 0 && len(values) > facetReq.Limit {
			values = values[:facetReq.Limit]
		}
		out = append(out, types.SearchFacet{Field: facetReq.Field, Values: values})
	}
	return out
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
