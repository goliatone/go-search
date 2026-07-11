package ranking

import (
	"github.com/goliatone/go-search/pkg/types"
	"sort"
)

type RankedList struct {
	Index  string
	Weight float64
	Hits   []types.SearchHit
}

func FuseRRF(lists []RankedList, k float64) []types.SearchHit {
	if k <= 0 {
		k = 60
	}
	byID := map[string]types.SearchHit{}
	for _, list := range lists {
		w := list.Weight
		if w <= 0 {
			w = 1
		}
		for i, hit := range list.Hits {
			id := hit.ID
			if hit.Document != nil && hit.Document.Index != "" {
				list.Index = hit.Document.Index
			}
			contribution := w / (k + float64(i+1))
			merged, ok := byID[id]
			if !ok {
				merged = hit
				merged.BaseScore = 0
				merged.FinalScore = 0
				merged.Score = 0
				merged.Retrieval = &types.AppliedRetrievalSignals{Mode: types.SearchModeLexical}
			}
			var providerScore *float64
			if hit.Retrieval != nil {
				providerScore = hit.Retrieval.ProviderScore
			}
			merged.Retrieval.Contributions = append(merged.Retrieval.Contributions, types.RetrievalContribution{Index: list.Index, ProviderRank: i + 1, ProviderScore: providerScore, Contribution: contribution})
			merged.BaseScore += contribution
			merged.FinalScore = merged.BaseScore
			merged.Score = merged.BaseScore
			byID[id] = merged
		}
	}
	out := make([]types.SearchHit, 0, len(byID))
	for _, hit := range byID {
		out = append(out, hit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FinalScore == out[j].FinalScore {
			return out[i].ID < out[j].ID
		}
		return out[i].FinalScore > out[j].FinalScore
	})
	return out
}
