package ranking

import (
	"github.com/goliatone/go-search/pkg/types"
	"sort"
	"strings"
)

func ResultID(hit types.SearchHit) string {
	if hit.ResultID != "" {
		return hit.ResultID
	}
	if hit.Document != nil && hit.Document.ResultID != "" {
		return hit.Document.ResultID
	}
	if hit.Parent != nil && hit.Parent.ID != "" {
		kind := hit.Parent.Type
		if kind == "" {
			kind = hit.Type
		}
		if kind == "" {
			kind = "result"
		}
		return kind + ":" + hit.Parent.ID
	}
	kind := hit.Type
	if kind == "" {
		kind = "result"
	}
	return kind + ":" + hit.ID
}
func GroupEntities(hits []types.SearchHit, topK int) []types.SearchHit {
	if topK < 1 {
		topK = 1
	}
	groups := map[string][]types.SearchHit{}
	order := []string{}
	for _, hit := range hits {
		id := ResultID(hit)
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], hit)
	}
	out := make([]types.SearchHit, 0, len(order))
	for _, id := range order {
		units := groups[id]
		sort.SliceStable(units, func(i, j int) bool {
			if units[i].FinalScore == units[j].FinalScore {
				return units[i].ID < units[j].ID
			}
			return units[i].FinalScore > units[j].FinalScore
		})
		rep := units[0]
		rep.ResultID = id
		if rep.ResultType == "" {
			rep.ResultType = strings.SplitN(id, ":", 2)[0]
		}
		score := 0.0
		limit := min(topK, len(units))
		for i := range limit {
			score += units[i].FinalScore / float64(i+1)
		}
		rep.BaseScore = score
		rep.FinalScore = score
		rep.Score = score
		out = append(out, rep)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FinalScore == out[j].FinalScore {
			return out[i].ResultID < out[j].ResultID
		}
		return out[i].FinalScore > out[j].FinalScore
	})
	return out
}
func AggregateEvidence(hits []types.SearchHit, maxSamples int) map[string]*types.MatchEvidenceSummary {
	counts := map[string]map[string]int{}
	samples := map[string]map[string][]types.MatchEvidenceSample{}
	for _, hit := range hits {
		id := ResultID(hit)
		location := "unknown"
		if hit.Document != nil && hit.Document.MatchLocation != "" {
			location = hit.Document.MatchLocation
		}
		if counts[id] == nil {
			counts[id] = map[string]int{}
			samples[id] = map[string][]types.MatchEvidenceSample{}
		}
		counts[id][location]++
		if len(samples[id][location]) < maxSamples && hit.Document != nil {
			samples[id][location] = append(samples[id][location], types.MatchEvidenceSample{
				DocumentID:   hit.Document.ID,
				Field:        hit.Document.MatchField,
				Locale:       hit.Document.Locale,
				Snippet:      types.BoundedSearchSnippet(hit.Snippet),
				ChunkOrdinal: hit.Document.ChunkOrdinal,
				Anchor:       hit.Anchor,
			})
		}
	}
	out := map[string]*types.MatchEvidenceSummary{}
	for id, locations := range counts {
		summary := &types.MatchEvidenceSummary{Exact: true, Status: types.EvidenceStatusComplete}
		keys := []string{}
		for key := range locations {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			summary.Locations = append(summary.Locations, types.MatchEvidenceLocation{Location: key, Count: locations[key], Samples: samples[id][key]})
		}
		out[id] = summary
	}
	return out
}
