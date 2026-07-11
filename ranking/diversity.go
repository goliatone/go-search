package ranking

import (
	"github.com/goliatone/go-search/pkg/types"
	"net/url"
	"sort"
	"strings"
)

type DiversityConfig struct {
	FamilyField     string
	MaxPerFamily    int
	RepeatedPenalty float64
}
type DiversityResult struct {
	Hits       []types.SearchHit
	Suppressed []types.DiversityEvidence
}

func ApplyDiversity(hits []types.SearchHit, cfg DiversityConfig) []types.SearchHit {
	return ApplyDiversityDetailed(hits, cfg).Hits
}
func ApplyDiversityDetailed(hits []types.SearchHit, cfg DiversityConfig) DiversityResult {
	seenURL := map[string]bool{}
	families := map[string]int{}
	out := make([]types.SearchHit, 0, len(hits))
	suppressed := []types.DiversityEvidence{}
	for _, hit := range hits {
		canonical := canonicalURL(hit.URL)
		if canonical != "" && seenURL[canonical] {
			suppressed = append(suppressed, types.DiversityEvidence{Kind: "canonical_duplicate", Key: canonical, Suppressed: true})
			continue
		}
		seenURL[canonical] = true
		family := ""
		if hit.Fields != nil {
			family, _ = hit.Fields[cfg.FamilyField].(string)
			family = strings.TrimSpace(family)
		}
		families[family]++
		occ := families[family]
		if family != "" && cfg.MaxPerFamily > 0 && occ > cfg.MaxPerFamily {
			suppressed = append(suppressed, types.DiversityEvidence{Kind: "family_cap", Key: family, Occurrence: occ, Suppressed: true})
			continue
		}
		if hit.Ranking == nil {
			hit.Ranking = &types.AppliedRankingSignals{}
		}
		if family != "" && occ > 1 {
			penalty := float64(occ-1) * cfg.RepeatedPenalty
			hit.FinalScore -= penalty
			hit.Score = hit.FinalScore
			hit.Ranking.Diversity = append(hit.Ranking.Diversity, types.DiversityEvidence{Kind: "family_penalty", Key: family, Occurrence: occ, Penalty: penalty})
		}
		out = append(out, hit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FinalScore == out[j].FinalScore {
			return out[i].ID < out[j].ID
		}
		return out[i].FinalScore > out[j].FinalScore
	})
	return DiversityResult{Hits: out, Suppressed: suppressed}
}
func canonicalURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	u.Fragment = ""
	query := u.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if lower == "utm" || strings.HasPrefix(lower, "utm_") || lower == "gclid" || lower == "fbclid" {
			query.Del(key)
		}
	}
	u.RawQuery = query.Encode()
	u.Path = strings.TrimSuffix(u.Path, "/")
	return strings.ToLower(u.String())
}
