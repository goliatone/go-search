package ranking

import (
	"fmt"
	"slices"
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
	page.Page = req.Page
	page.PerPage = req.PerPage
	page.Hits = ApplyRulesToHits(req, page.Hits, rules, now)
	if req.GroupBy != "" {
		allGroups := GroupHitsBy(page.Hits, req.GroupBy)
		sort.SliceStable(allGroups, func(i, j int) bool {
			return compareHits(req, *allGroups[i].TopHit, *allGroups[j].TopHit)
		})
		page.Total = len(allGroups)
		page.Groups = PaginateGroups(allGroups, req.Page, req.PerPage)
		page.Hits = FlattenGroupHits(page.Groups)
		page.Metadata = mergePageRankingMetadata(page.Metadata, page.Hits, page.Groups, rules)
		return page, nil
	}
	page.Total = len(page.Hits)
	page.Hits = PaginateHits(page.Hits, req.Page, req.PerPage)
	page.Metadata = mergePageRankingMetadata(page.Metadata, page.Hits, nil, rules)
	return page, nil
}

func ApplyRulesToHits(req types.SearchRequest, hits []types.SearchHit, rules []types.EditorialRankRule, now time.Time) []types.SearchHit {
	out := make([]types.SearchHit, 0, len(hits))
	for _, hit := range hits {
		evaluation := evaluateHit(req, hit, rules, now)
		if evaluation.hidden {
			continue
		}
		hit.FinalScore = evaluation.score
		hit.Score = evaluation.score
		if hit.Ranking == nil {
			hit.Ranking = &types.AppliedRankingSignals{}
		}
		hit.Ranking.Editorial = evaluation.applied
		if hit.Ranking.Metadata == nil {
			hit.Ranking.Metadata = map[string]any{}
		}
		hit.Ranking.Metadata["base_score"] = hit.BaseScore
		hit.Ranking.Metadata["final_score"] = evaluation.score
		hit.Ranking.Metadata["matched_parent_target"] = evaluation.matchedParentTarget
		hit.Ranking.Metadata["rule_count"] = len(evaluation.applied)
		if localeMatch := localeMatchLabel(hit); localeMatch != "" {
			hit.Ranking.Metadata["locale_match"] = localeMatch
		}
		if exactLocale, ok := exactLocaleFlag(hit); ok {
			hit.Ranking.Metadata["exact_locale"] = exactLocale
		}
		if evaluation.pin != nil {
			if evaluation.pin.Position != nil {
				hit.Ranking.Metadata["pin_position"] = *evaluation.pin.Position
			}
			hit.Ranking.Metadata["pin_rule_id"] = evaluation.pin.ID
			hit.Ranking.Metadata["pin_specificity"] = evaluation.pinSpecificity
		}
		out = append(out, hit)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return compareHits(req, out[i], out[j])
	})
	return out
}

func GroupHits(hits []types.SearchHit) []types.SearchGroup {
	return GroupHitsBy(hits, "parent_id")
}

func GroupHitsBy(hits []types.SearchHit, field string) []types.SearchGroup {
	byParent := map[string][]types.SearchHit{}
	order := []string{}
	for _, hit := range hits {
		internalKey, externalKey := groupingKeysByField(hit, field)
		if _, ok := byParent[internalKey]; !ok {
			order = append(order, internalKey)
		}
		byParent[internalKey] = append(byParent[internalKey], hit)
		_ = externalKey
	}
	groups := make([]types.SearchGroup, 0, len(order))
	for _, internalKey := range order {
		groupHits := byParent[internalKey]
		sort.SliceStable(groupHits, func(i, j int) bool {
			if groupHits[i].FinalScore == groupHits[j].FinalScore {
				return groupHits[i].ID < groupHits[j].ID
			}
			return groupHits[i].FinalScore > groupHits[j].FinalScore
		})
		top := groupHits[0]
		_, externalKey := groupingKeysByField(top, field)
		group := types.SearchGroup{
			Key:    externalKey,
			Parent: top.Parent,
			TopHit: &top,
			Hits:   groupHits,
			Count:  len(groupHits),
			Metadata: map[string]any{
				"top_hit_id":   top.ID,
				"final_score":  top.FinalScore,
				"locale_match": localeMatchLabel(top),
				"group_key":    internalKey,
			},
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

func groupingKeysByField(hit types.SearchHit, field string) (string, string) {
	field = strings.TrimSpace(field)
	groupID := ""
	switch field {
	case "result_id":
		groupID = strings.TrimSpace(ResultID(hit))
	case "parent_id", "":
		if hit.Parent != nil {
			groupID = strings.TrimSpace(hit.Parent.ID)
		}
		if groupID == "" && hit.Document != nil {
			groupID = strings.TrimSpace(hit.Document.ParentID)
		}
	default:
		if hit.Document != nil && hit.Document.Fields != nil {
			groupID = strings.TrimSpace(fmt.Sprint(hit.Document.Fields[field]))
		}
		if groupID == "" && hit.Fields != nil {
			groupID = strings.TrimSpace(fmt.Sprint(hit.Fields[field]))
		}
	}
	if groupID == "" || groupID == "<nil>" {
		groupID = strings.TrimSpace(hit.ID)
	}
	if field == "result_id" {
		// Result IDs are the provider-neutral public entity identity and must
		// collapse across indexes. Generic parent IDs remain index-scoped below
		// because different document families can legitimately reuse them.
		return groupID, groupID
	}
	index := ""
	if hit.Document != nil {
		index = strings.TrimSpace(hit.Document.Index)
	}
	if index == "" {
		index = "_default"
	}
	return index + "\x00" + groupID, groupID
}

func matchesRule(req types.SearchRequest, targetID string, parentTargetID string, rule types.EditorialRankRule, now time.Time) bool {
	if !rule.Enabled {
		return false
	}
	if rule.TargetID != "" && rule.TargetID != targetID {
		return false
	}
	if rule.ParentTargetID != "" && rule.ParentTargetID != parentTargetID {
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
	if rule.Scope.Topic != "" && !requestHasFilterValue(req.Filters, "topic", rule.Scope.Topic) {
		return false
	}
	for field, values := range rule.Scope.Filters {
		for _, value := range values {
			if !requestHasFilterValue(req.Filters, field, value) {
				return false
			}
		}
	}
	return true
}

func contains(needles []string, haystack []string) bool {
	for _, n := range needles {
		if slices.Contains(haystack, n) {
			return true
		}
	}
	return len(needles) == 0
}

type hitEvaluation struct {
	applied             []types.AppliedEditorialSignal
	hidden              bool
	matchedParentTarget string
	score               float64
	pin                 *types.EditorialRankRule
	pinSpecificity      int
}

func evaluateHit(req types.SearchRequest, hit types.SearchHit, rules []types.EditorialRankRule, now time.Time) hitEvaluation {
	evaluation := hitEvaluation{score: hit.BaseScore}
	targetID := hit.ID
	parentTargetID := hit.ID
	if hit.Parent != nil && hit.Parent.ID != "" {
		parentTargetID = hit.Parent.ID
	}
	evaluation.matchedParentTarget = parentTargetID
	for _, rule := range rules {
		if !matchesRule(req, targetID, parentTargetID, rule, now) {
			continue
		}
		evaluation.applied = append(evaluation.applied, types.AppliedEditorialSignal{
			RuleID: rule.ID,
			Action: rule.Action,
			Weight: rule.Weight,
			Scope:  rule.Scope.Locale,
			Reason: rule.Reason,
		})
		switch rule.Action {
		case types.EditorialActionHide:
			evaluation.hidden = true
		case types.EditorialActionPin:
			evaluation.pin, evaluation.pinSpecificity = choosePin(evaluation.pin, evaluation.pinSpecificity, rule)
		case types.EditorialActionBoost:
			evaluation.score += rule.Weight
		case types.EditorialActionBury:
			evaluation.score -= rule.Weight
		}
	}
	return evaluation
}

func choosePin(current *types.EditorialRankRule, currentSpecificity int, candidate types.EditorialRankRule) (*types.EditorialRankRule, int) {
	candidateSpecificity := ruleSpecificity(candidate)
	if current == nil {
		copy := candidate
		return &copy, candidateSpecificity
	}
	currentPos, candidatePos := pinPosition(*current), pinPosition(candidate)
	if candidatePos != currentPos {
		if candidatePos < currentPos {
			copy := candidate
			return &copy, candidateSpecificity
		}
		return current, currentSpecificity
	}
	if candidateSpecificity != currentSpecificity {
		if candidateSpecificity > currentSpecificity {
			copy := candidate
			return &copy, candidateSpecificity
		}
		return current, currentSpecificity
	}
	if candidate.Weight != current.Weight {
		if candidate.Weight > current.Weight {
			copy := candidate
			return &copy, candidateSpecificity
		}
		return current, currentSpecificity
	}
	if candidate.ID < current.ID {
		copy := candidate
		return &copy, candidateSpecificity
	}
	return current, currentSpecificity
}

func compareHits(req types.SearchRequest, left, right types.SearchHit) bool {
	leftPinPosition, leftPinned := hitPinPosition(left)
	rightPinPosition, rightPinned := hitPinPosition(right)
	if leftPinned || rightPinned {
		if leftPinned != rightPinned {
			return leftPinned
		}
		if leftPinPosition != rightPinPosition {
			return leftPinPosition < rightPinPosition
		}
		leftSpecificity := hitPinSpecificity(left)
		rightSpecificity := hitPinSpecificity(right)
		if leftSpecificity != rightSpecificity {
			return leftSpecificity > rightSpecificity
		}
	}
	leftLocaleRank := localeRank(req, left)
	rightLocaleRank := localeRank(req, right)
	if len(req.Sort) > 0 {
		if leftLocaleRank != rightLocaleRank {
			return leftLocaleRank < rightLocaleRank
		}
		if ordered, ok := compareRequestedSorts(req.Sort, left, right); ok {
			return ordered
		}
	}
	if left.FinalScore != right.FinalScore {
		return left.FinalScore > right.FinalScore
	}
	if len(req.Sort) == 0 && leftLocaleRank != rightLocaleRank {
		return leftLocaleRank < rightLocaleRank
	}
	if left.Parent != nil && right.Parent != nil && left.Parent.Title != right.Parent.Title {
		return left.Parent.Title < right.Parent.Title
	}
	return left.ID < right.ID
}

func compareRequestedSorts(sorts []types.Sort, left, right types.SearchHit) (bool, bool) {
	for _, sortField := range sorts {
		if strings.TrimSpace(sortField.Field) == "" {
			continue
		}
		leftNum, rightNum, numeric := sortableNumbers(left, right, sortField.Field)
		if numeric {
			if leftNum == rightNum {
				continue
			}
			if sortField.Direction == types.SortAsc {
				return leftNum < rightNum, true
			}
			return leftNum > rightNum, true
		}
		leftText := sortableText(left, sortField.Field)
		rightText := sortableText(right, sortField.Field)
		if leftText == rightText {
			continue
		}
		if sortField.Direction == types.SortAsc {
			return leftText < rightText, true
		}
		return leftText > rightText, true
	}
	return false, false
}

func sortableNumbers(left, right types.SearchHit, field string) (float64, float64, bool) {
	leftValue, leftOK := sortableNumber(left, field)
	rightValue, rightOK := sortableNumber(right, field)
	if !leftOK && !rightOK {
		return 0, 0, false
	}
	return leftValue, rightValue, true
}

func sortableNumber(hit types.SearchHit, field string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "start_ms":
		if hit.Anchor != nil {
			return float64(hit.Anchor.StartMS), true
		}
	case "end_ms":
		if hit.Anchor != nil {
			return float64(hit.Anchor.EndMS), true
		}
	}
	for _, source := range []map[string]any{hit.Fields, documentFields(hit.Document)} {
		if len(source) == 0 {
			continue
		}
		if raw, ok := source[field]; ok {
			if value, ok := anyFloat(raw); ok {
				return value, true
			}
		}
	}
	if hit.Document != nil && hit.Document.Numeric != nil {
		if value, ok := hit.Document.Numeric[field]; ok {
			return value, true
		}
	}
	return 0, false
}

func sortableText(hit types.SearchHit, field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "title":
		return normalizeSortableText(hit.Title)
	case "locale":
		return normalizeSortableText(hit.Locale)
	}
	for _, source := range []map[string]any{hit.Fields, documentFields(hit.Document)} {
		if len(source) == 0 {
			continue
		}
		if raw, ok := source[field]; ok {
			return normalizeSortableText(fmt.Sprint(raw))
		}
	}
	return ""
}

func documentFields(doc *types.Document) map[string]any {
	if doc == nil {
		return nil
	}
	return doc.Fields
}

func anyFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func normalizeSortableText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func localeRank(req types.SearchRequest, hit types.SearchHit) int {
	if exact, ok := exactLocaleFlag(hit); ok {
		if exact {
			return 0
		}
		return 1
	}
	if req.Locale == "" || hit.Locale == "" {
		return 1
	}
	if strings.EqualFold(req.Locale, hit.Locale) {
		return 0
	}
	return 1
}

func exactLocaleFlag(hit types.SearchHit) (bool, bool) {
	if hit.Retrieval == nil || hit.Retrieval.Metadata == nil {
		return false, false
	}
	raw, ok := hit.Retrieval.Metadata["exact_locale"]
	if !ok {
		return false, false
	}
	value, ok := raw.(bool)
	return value, ok
}

func localeMatchLabel(hit types.SearchHit) string {
	if hit.Retrieval == nil || hit.Retrieval.Metadata == nil {
		return ""
	}
	raw, ok := hit.Retrieval.Metadata["locale_match"]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return value
}

func hitPinPosition(hit types.SearchHit) (int, bool) {
	if hit.Ranking == nil || hit.Ranking.Metadata == nil {
		return 0, false
	}
	if raw, ok := hit.Ranking.Metadata["pin_position"]; ok {
		switch value := raw.(type) {
		case int:
			return value, true
		case int32:
			return int(value), true
		case int64:
			return int(value), true
		case float64:
			return int(value), true
		}
	}
	if _, ok := hit.Ranking.Metadata["pin_rule_id"]; ok {
		return 0, true
	}
	return 0, false
}

func hitPinSpecificity(hit types.SearchHit) int {
	if hit.Ranking == nil || hit.Ranking.Metadata == nil {
		return 0
	}
	raw, ok := hit.Ranking.Metadata["pin_specificity"]
	if !ok {
		return 0
	}
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func pinPosition(rule types.EditorialRankRule) int {
	if rule.Position != nil {
		return *rule.Position
	}
	return 0
}

func ruleSpecificity(rule types.EditorialRankRule) int {
	score := 0
	if rule.ParentTargetID != "" {
		score += 1000
	}
	if rule.TargetID != "" {
		score += 1000
	}
	score += len(rule.Scope.Indexes) * 10
	if rule.Scope.TenantID != "" {
		score += 40
	}
	if rule.Scope.OrgID != "" {
		score += 40
	}
	if rule.Scope.Locale != "" {
		score += 50
	}
	if rule.Scope.Topic != "" {
		score += 20
	}
	if rule.Scope.Query != "" {
		score += 60
	}
	if rule.Scope.RankingProfile != "" {
		score += 30
	}
	for _, values := range rule.Scope.Filters {
		score += len(values) * 15
	}
	return score
}

func requestHasFilterValue(expr types.FilterExpr, field, want string) bool {
	if expr == nil {
		return false
	}
	want = strings.TrimSpace(strings.ToLower(want))
	switch value := expr.(type) {
	case types.AndExpr:
		for _, term := range value.Terms {
			if requestHasFilterValue(term, field, want) {
				return true
			}
		}
	case types.OrExpr:
		for _, term := range value.Terms {
			if requestHasFilterValue(term, field, want) {
				return true
			}
		}
	case types.NotExpr:
		return false
	case types.TermExpr:
		if value.Field != field {
			return false
		}
		switch value.Op {
		case types.FilterOpEQ:
			return strings.TrimSpace(strings.ToLower(asString(value.Value))) == want
		case types.FilterOpContains:
			return strings.Contains(strings.TrimSpace(strings.ToLower(asString(value.Value))), want)
		case types.FilterOpIn:
			for _, candidate := range asStringSlice(value.Value) {
				if strings.TrimSpace(strings.ToLower(candidate)) == want {
					return true
				}
			}
		}
	}
	return false
}

func asString(value any) string {
	switch raw := value.(type) {
	case string:
		return raw
	default:
		return fmt.Sprint(raw)
	}
}

func asStringSlice(value any) []string {
	switch raw := value.(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			out = append(out, asString(item))
		}
		return out
	default:
		s := asString(value)
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

func PaginateHits(hits []types.SearchHit, page, perPage int) []types.SearchHit {
	if perPage <= 0 {
		return hits
	}
	start := (page - 1) * perPage
	if start >= len(hits) || start < 0 {
		return nil
	}
	end := min(start+perPage, len(hits))
	return hits[start:end]
}

func PaginateGroups(groups []types.SearchGroup, page, perPage int) []types.SearchGroup {
	if perPage <= 0 {
		return groups
	}
	start := (page - 1) * perPage
	if start >= len(groups) || start < 0 {
		return nil
	}
	end := min(start+perPage, len(groups))
	return groups[start:end]
}

func FlattenGroupHits(groups []types.SearchGroup) []types.SearchHit {
	out := make([]types.SearchHit, 0)
	for _, group := range groups {
		out = append(out, group.Hits...)
	}
	return out
}

func mergePageRankingMetadata(metadata map[string]any, hits []types.SearchHit, groups []types.SearchGroup, rules []types.EditorialRankRule) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	applied := 0
	pinned := 0
	for _, hit := range hits {
		if hit.Ranking != nil {
			applied += len(hit.Ranking.Editorial)
		}
		if _, ok := hitPinPosition(hit); ok {
			pinned++
		}
	}
	metadata["ranking"] = map[string]any{
		"rule_count":         len(rules),
		"applied_rule_count": applied,
		"pinned_hit_count":   pinned,
		"group_count":        len(groups),
		"hit_count":          len(hits),
	}
	return metadata
}
