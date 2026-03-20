package ranking

import (
	"sort"
	"strings"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

type Policy interface {
	Apply(req types.SearchRequest, page types.SearchResultPage, rules []types.EditorialRankRule, now time.Time) (types.SearchResultPage, error)
}

type DefaultPolicy struct{}

func NewDefaultPolicy() *DefaultPolicy {
	return &DefaultPolicy{}
}

func (p *DefaultPolicy) Apply(req types.SearchRequest, page types.SearchResultPage, rules []types.EditorialRankRule, now time.Time) (types.SearchResultPage, error) {
	page.Hits = ApplyRulesToHits(req, page.Hits, rules, now)
	if len(page.Groups) > 0 {
		page.Groups = GroupHits(page.Hits)
		page.Total = len(page.Groups)
	}
	return page, nil
}

func ApplyRulesToHits(req types.SearchRequest, hits []types.SearchHit, rules []types.EditorialRankRule, now time.Time) []types.SearchHit {
	out := make([]types.SearchHit, 0, len(hits))
	for _, hit := range hits {
		applied := []types.AppliedEditorialSignal{}
		hidden := false
		score := hit.BaseScore
		parentID := hit.ID
		if hit.Parent != nil && hit.Parent.ID != "" {
			parentID = hit.Parent.ID
		}
		for _, rule := range rules {
			if !matchesRule(req, parentID, rule, now) {
				continue
			}
			switch rule.Action {
			case types.EditorialActionHide:
				hidden = true
			case types.EditorialActionPin:
				score += 10_000 + rule.Weight
			case types.EditorialActionBoost:
				score += rule.Weight
			case types.EditorialActionBury:
				score -= rule.Weight
			}
			applied = append(applied, types.AppliedEditorialSignal{
				RuleID: rule.ID,
				Action: rule.Action,
				Weight: rule.Weight,
				Scope:  rule.Scope.Locale,
				Reason: rule.Reason,
			})
		}
		if hidden {
			continue
		}
		hit.FinalScore = score
		hit.Score = score
		if len(applied) > 0 {
			if hit.Ranking == nil {
				hit.Ranking = &types.AppliedRankingSignals{}
			}
			hit.Ranking.Editorial = applied
		}
		out = append(out, hit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FinalScore == out[j].FinalScore {
			if out[i].Parent != nil && out[j].Parent != nil && out[i].Parent.Title != out[j].Parent.Title {
				return out[i].Parent.Title < out[j].Parent.Title
			}
			return out[i].ID < out[j].ID
		}
		return out[i].FinalScore > out[j].FinalScore
	})
	return out
}

func GroupHits(hits []types.SearchHit) []types.SearchGroup {
	byParent := map[string][]types.SearchHit{}
	order := []string{}
	for _, hit := range hits {
		key := hit.ID
		parent := hit.Parent
		if parent != nil && parent.ID != "" {
			key = parent.ID
		}
		if _, ok := byParent[key]; !ok {
			order = append(order, key)
		}
		byParent[key] = append(byParent[key], hit)
	}
	groups := make([]types.SearchGroup, 0, len(order))
	for _, key := range order {
		groupHits := byParent[key]
		sort.SliceStable(groupHits, func(i, j int) bool {
			if groupHits[i].FinalScore == groupHits[j].FinalScore {
				return groupHits[i].ID < groupHits[j].ID
			}
			return groupHits[i].FinalScore > groupHits[j].FinalScore
		})
		top := groupHits[0]
		group := types.SearchGroup{
			Key:    key,
			Parent: top.Parent,
			TopHit: &top,
			Hits:   groupHits,
			Count:  len(groupHits),
		}
		if group.Parent == nil {
			group.Parent = &types.SearchParent{
				ID:    top.ID,
				Type:  top.Type,
				Title: top.Title,
				URL:   top.URL,
			}
		}
		groups = append(groups, group)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].TopHit.FinalScore == groups[j].TopHit.FinalScore {
			return groups[i].Key < groups[j].Key
		}
		return groups[i].TopHit.FinalScore > groups[j].TopHit.FinalScore
	})
	return groups
}

func matchesRule(req types.SearchRequest, targetID string, rule types.EditorialRankRule, now time.Time) bool {
	if !rule.Enabled {
		return false
	}
	if rule.TargetID != "" && rule.TargetID != targetID && rule.ParentTargetID != targetID {
		return false
	}
	if rule.StartsAt != nil && now.Before(*rule.StartsAt) {
		return false
	}
	if rule.EndsAt != nil && now.After(*rule.EndsAt) {
		return false
	}
	if len(rule.Scope.Indexes) > 0 && !contains(rule.Scope.Indexes, req.Indexes) {
		return false
	}
	if rule.Scope.Locale != "" && rule.Scope.Locale != req.Locale {
		return false
	}
	if rule.Scope.TenantID != "" && rule.Scope.TenantID != req.Scope.TenantID {
		return false
	}
	if rule.Scope.OrgID != "" && rule.Scope.OrgID != req.Scope.OrgID {
		return false
	}
	if rule.Scope.RankingProfile != "" && rule.Scope.RankingProfile != req.RankingProfile {
		return false
	}
	if rule.Scope.Query != "" && !strings.EqualFold(strings.TrimSpace(rule.Scope.Query), strings.TrimSpace(req.Query)) {
		return false
	}
	return true
}

func contains(needles []string, haystack []string) bool {
	for _, n := range needles {
		for _, h := range haystack {
			if n == h {
				return true
			}
		}
	}
	return len(needles) == 0
}
