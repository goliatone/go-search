package types

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

const DefaultFacetPathSeparator = " > "

const FacetCountByResultID = "result_id"

// BuildEntityFacet creates exact unique-entity counts and bounded mergeable
// identity sets. The caller is responsible for including every eligible match.
func BuildEntityFacet(request FacetRequest, identities map[string]map[string]struct{}, selected []string) SearchFacet {
	counts := make(map[string]int, len(identities))
	for value, ids := range identities {
		counts[value] = len(ids)
	}
	facet := BuildFacet(request, counts, selected)
	facet.Accuracy = FacetCountAccuracyExact
	limit := request.IdentityLimit
	for i := range facet.Values {
		ids := make([]string, 0, len(identities[facet.Values[i].Value]))
		for id := range identities[facet.Values[i].Value] {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if limit > 0 && len(ids) > limit {
			ids = ids[:limit]
			facet.Values[i].EntityIDsComplete = false
		} else {
			facet.Values[i].EntityIDsComplete = true
		}
		facet.Values[i].EntityIDs = ids
	}
	return facet
}

// MergeEntityFacets unions per-bucket global identities across indexes.
func MergeEntityFacets(request FacetRequest, facets ...SearchFacet) SearchFacet {
	identities := map[string]map[string]struct{}{}
	selected := []string{}
	complete := true
	for _, facet := range facets {
		for _, value := range facet.Values {
			if value.Selected {
				selected = append(selected, value.Value)
			}
			set := identities[value.Value]
			if set == nil {
				set = map[string]struct{}{}
				identities[value.Value] = set
			}
			for _, id := range value.EntityIDs {
				set[id] = struct{}{}
			}
			complete = complete && value.EntityIDsComplete
		}
	}
	merged := BuildEntityFacet(request, identities, selected)
	if !complete {
		merged.Accuracy = FacetCountAccuracyLowerBound
	}
	return merged
}

func (r FacetRequest) NormalizedKind() FacetKind {
	switch r.Kind {
	case FacetKindHierarchical, FacetKindNumericRange, FacetKindDateRange:
		return r.Kind
	default:
		return FacetKindTerm
	}
}

func (r FacetRequest) PathSeparator() string {
	if separator := strings.TrimSpace(r.Separator); separator != "" {
		return separator
	}
	return DefaultFacetPathSeparator
}

func SplitFacetPath(value, separator string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.TrimSpace(separator) == "" {
		separator = DefaultFacetPathSeparator
	}
	raw := strings.Split(value, separator)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func JoinFacetPath(path []string, separator string) string {
	if strings.TrimSpace(separator) == "" {
		separator = DefaultFacetPathSeparator
	}
	clean := make([]string, 0, len(path))
	for _, item := range path {
		item = strings.TrimSpace(item)
		if item != "" {
			clean = append(clean, item)
		}
	}
	return strings.Join(clean, separator)
}

func SelectedFacetValues(expr FilterExpr, field string) []string {
	field = strings.TrimSpace(field)
	if field == "" || expr == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := selectedFacetValues(expr, field, seen)
	sort.Strings(out)
	return out
}

func selectedFacetValues(expr FilterExpr, field string, seen map[string]struct{}) []string {
	switch v := expr.(type) {
	case AndExpr:
		out := []string{}
		for _, term := range v.Terms {
			out = append(out, selectedFacetValues(term, field, seen)...)
		}
		return out
	case OrExpr:
		out := []string{}
		for _, term := range v.Terms {
			out = append(out, selectedFacetValues(term, field, seen)...)
		}
		return out
	case NotExpr:
		return nil
	case TermExpr:
		if strings.TrimSpace(v.Field) != field {
			return nil
		}
		return uniqueFacetValues(v.Value, seen)
	default:
		return nil
	}
}

func uniqueFacetValues(value any, seen map[string]struct{}) []string {
	out := []string{}
	appendValue := func(item string) {
		item = strings.TrimSpace(item)
		if item == "" {
			return
		}
		if _, ok := seen[item]; ok {
			return
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	switch v := value.(type) {
	case string:
		appendValue(v)
	case []string:
		for _, item := range v {
			appendValue(item)
		}
	case []any:
		for _, item := range v {
			appendValue(stringifyFacetValue(item))
		}
	default:
		appendValue(stringifyFacetValue(v))
	}
	return out
}

func RemoveFacetFilter(expr FilterExpr, field string) FilterExpr {
	field = strings.TrimSpace(field)
	if field == "" || expr == nil {
		return expr
	}
	switch v := expr.(type) {
	case AndExpr:
		terms := make([]FilterExpr, 0, len(v.Terms))
		for _, term := range v.Terms {
			if next := RemoveFacetFilter(term, field); next != nil {
				terms = append(terms, next)
			}
		}
		return collapseTerms(terms, true)
	case OrExpr:
		terms := make([]FilterExpr, 0, len(v.Terms))
		for _, term := range v.Terms {
			if next := RemoveFacetFilter(term, field); next != nil {
				terms = append(terms, next)
			}
		}
		return collapseTerms(terms, false)
	case NotExpr:
		if next := RemoveFacetFilter(v.Term, field); next != nil {
			return NotExpr{Term: next}
		}
		return nil
	case TermExpr:
		if strings.TrimSpace(v.Field) == field {
			return nil
		}
		return v
	case RangeExpr:
		if strings.TrimSpace(v.Field) == field {
			return nil
		}
		return v
	case ExistsExpr:
		if strings.TrimSpace(v.Field) == field {
			return nil
		}
		return v
	default:
		return expr
	}
}

func BuildFacet(request FacetRequest, counts map[string]int, selected []string) SearchFacet {
	facet := SearchFacet{
		Field:       request.Field,
		Kind:        request.NormalizedKind(),
		Disjunctive: request.Disjunctive,
		Metadata:    cloneFacetMetadata(request.Metadata),
	}
	if len(request.Path) > 0 {
		if facet.Metadata == nil {
			facet.Metadata = map[string]any{}
		}
		facet.Metadata["path"] = append([]string(nil), request.Path...)
	}
	if facet.Kind == FacetKindHierarchical {
		if facet.Metadata == nil {
			facet.Metadata = map[string]any{}
		}
		facet.Metadata["separator"] = request.PathSeparator()
	}
	values := make([]SearchFacetValue, 0, len(counts))
	for value, count := range counts {
		item := SearchFacetValue{
			Value:    value,
			Count:    count,
			Selected: slices.Contains(selected, value),
		}
		if facet.Kind == FacetKindHierarchical {
			item.Path = SplitFacetPath(value, request.PathSeparator())
			item.Level = max(0, len(item.Path)-1)
			item.Label = facetLabel(item.Path, value)
			if len(item.Path) > 1 {
				item.ParentValue = JoinFacetPath(item.Path[:len(item.Path)-1], request.PathSeparator())
			}
		}
		values = append(values, item)
	}
	for _, value := range selected {
		if _, ok := counts[value]; ok || strings.TrimSpace(value) == "" {
			continue
		}
		item := SearchFacetValue{Value: value, Selected: true}
		if facet.Kind == FacetKindHierarchical {
			item.Path = SplitFacetPath(value, request.PathSeparator())
			item.Level = max(0, len(item.Path)-1)
			item.Label = facetLabel(item.Path, value)
			if len(item.Path) > 1 {
				item.ParentValue = JoinFacetPath(item.Path[:len(item.Path)-1], request.PathSeparator())
			}
		}
		values = append(values, item)
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Count == values[j].Count {
			if values[i].Level == values[j].Level {
				return values[i].Value < values[j].Value
			}
			return values[i].Level < values[j].Level
		}
		return values[i].Count > values[j].Count
	})
	if request.Limit > 0 && len(values) > request.Limit {
		limited := append([]SearchFacetValue(nil), values[:request.Limit]...)
		for _, value := range values[request.Limit:] {
			if value.Selected && !slices.ContainsFunc(limited, func(item SearchFacetValue) bool { return item.Value == value.Value }) {
				limited = append(limited, value)
			}
		}
		values = limited
	}
	facet.Values = values
	return facet
}

func cloneFacetMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func facetLabel(path []string, fallback string) string {
	if len(path) == 0 {
		return fallback
	}
	return path[len(path)-1]
}

func collapseTerms(terms []FilterExpr, and bool) FilterExpr {
	switch len(terms) {
	case 0:
		return nil
	case 1:
		return terms[0]
	default:
		if and {
			return AndExpr{Terms: terms}
		}
		return OrExpr{Terms: terms}
	}
}

func stringifyFacetValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
