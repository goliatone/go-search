package typesense

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/ranking"
	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

const unscopedStorageValue = "__go_search_unscoped__"

func compileSearchParams(cfg Config, def types.IndexDefinition, req types.SearchRequest) (*tsapi.SearchCollectionParams, error) {
	cfg = normalizeConfig(cfg)
	weighted := req.QueryFields[def.Name]
	if len(weighted) == 0 {
		weighted = ranking.LegacyQueryFields(def)
	}
	queryFields := make([]string, 0, len(weighted))
	queryWeights := make([]string, 0, len(weighted))
	for _, field := range weighted {
		queryFields = append(queryFields, field.Field)
		queryWeights = append(queryWeights, strconv.Itoa(field.Weight))
	}
	if len(queryFields) == 0 {
		queryFields = searchQueryFields(def)
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = "*"
	}

	params := &tsapi.SearchCollectionParams{
		Q:                    &query,
		QueryBy:              new(strings.Join(queryFields, ",")),
		QueryByWeights:       new(strings.Join(queryWeights, ",")),
		Page:                 new(req.Page),
		PerPage:              new(req.PerPage),
		PrioritizeExactMatch: new(true),
	}
	if req.TextMatch != nil && req.TextMatch.PrioritizeExactMatch != nil {
		params.PrioritizeExactMatch = req.TextMatch.PrioritizeExactMatch
	}
	if req.TextMatch != nil && strings.TrimSpace(req.TextMatch.Mode) != "" {
		mode := strings.TrimSpace(req.TextMatch.Mode)
		if mode != "max_score" && mode != "max_weight" {
			return nil, errs.InvalidInput("invalid text match mode", map[string]any{"mode": mode})
		}
		params.TextMatchType = &mode
	}

	if req.GroupBy != "" {
		params.GroupBy = new(req.GroupBy)
		params.GroupLimit = new(cfg.GroupedEvidenceLimit)
		params.GroupMissingValues = new(false)
	}

	if filter, err := compileCombinedFilter(def, req); err != nil {
		return nil, err
	} else if filter != "" {
		params.FilterBy = &filter
	}

	if len(req.Facets) > 0 {
		fields := make([]string, 0, len(req.Facets))
		maxFacetValues := 0
		for _, facet := range req.Facets {
			if !contains(def.FacetFields, facet.Field) && facet.Field != "topic" && facet.Field != "parent_id" && facet.Field != "locale" && facet.Field != "match_location" {
				return nil, errs.InvalidFilter("facet field is not declared on the index", map[string]any{"field": facet.Field})
			}
			fields = append(fields, facet.Field)
			if facet.Limit > maxFacetValues {
				maxFacetValues = facet.Limit
			}
		}
		params.FacetBy = new(strings.Join(fields, ","))
		if maxFacetValues > 0 {
			params.MaxFacetValues = new(maxFacetValues)
		}
	}

	if sortBy := compileSortBy(req); sortBy != "" {
		params.SortBy = &sortBy
	}

	highlight := req.Highlight
	if len(highlight) == 0 {
		highlight = def.HighlightFields
	}
	if len(highlight) == 0 {
		highlight = queryFields
	}
	params.HighlightFields = new(strings.Join(highlight, ","))

	if len(req.IncludeFields) > 0 {
		params.IncludeFields = new(strings.Join(req.IncludeFields, ","))
	}

	return params, nil
}

func compileSuggestParams(cfg Config, def types.IndexDefinition, req types.SuggestRequest) (*tsapi.SearchCollectionParams, error) {
	cfg = normalizeConfig(cfg)
	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = "*"
	}

	queryFields := append([]string(nil), cfg.SuggestPreferParentFields...)
	queryByWeights := weightStrings(queryFields, cfg.SuggestPreferParentWeights)
	if !req.PreferParent {
		queryFields = append([]string(nil), cfg.SuggestDocumentFields...)
		queryByWeights = weightStrings(queryFields, cfg.SuggestDocumentWeights)
	}

	fetchLimit := max(req.Limit*cfg.SuggestFetchMultiplier, cfg.SuggestMinimumFetchLimit)
	params := &tsapi.SearchCollectionParams{
		Q:                    &query,
		QueryBy:              new(strings.Join(queryFields, ",")),
		QueryByWeights:       new(strings.Join(queryByWeights, ",")),
		PerPage:              new(fetchLimit),
		Page:                 new(1),
		Prefix:               new(strings.TrimRight(strings.Repeat("true,", len(queryFields)), ",")),
		PrioritizeExactMatch: new(true),
		HighlightFields:      new("none"),
	}

	if req.Locale != "" {
		values := localeConstraintValues(req.Locale)
		filter := fmt.Sprintf("locale:=[%s]", strings.Join(values, ","))
		params.FilterBy = &filter
		sortBy := fmt.Sprintf("_eval(locale:=%s):desc,_text_match:desc", encodeFilterValue(req.Locale))
		params.SortBy = &sortBy
	} else {
		sortBy := "_text_match:desc"
		params.SortBy = &sortBy
	}

	return params, nil
}

func weightStrings(fields []string, weights []int) []string {
	out := make([]string, 0, len(fields))
	for i := range fields {
		weight := 1
		if i < len(weights) && weights[i] > 0 {
			weight = weights[i]
		}
		out = append(out, strconv.Itoa(weight))
	}
	return out
}

func compileCombinedFilter(def types.IndexDefinition, req types.SearchRequest) (string, error) {
	allowed := allowedFilterFields(def)
	var parts []string
	if req.Filters != nil {
		filter, err := compileFilterExpr(req.Filters, allowed)
		if err != nil {
			return "", err
		}
		if filter != "" {
			parts = append(parts, filter)
		}
	}

	locales := localeConstraintValues(localeCandidates(req.Locale, req.Locales)...)
	if len(locales) > 0 {
		parts = append(parts, fmt.Sprintf("locale:=[%s]", strings.Join(locales, ",")))
	}
	if scopeFilter := compileScopeFilter(req.Scope); scopeFilter != "" {
		parts = append(parts, scopeFilter)
	}

	return strings.Join(parts, " && "), nil
}

func compileFilterExpr(expr types.FilterExpr, allowed map[string]struct{}) (string, error) {
	switch v := expr.(type) {
	case nil:
		return "", nil
	case types.AndExpr:
		parts := make([]string, 0, len(v.Terms))
		for _, term := range v.Terms {
			part, err := compileFilterExpr(term, allowed)
			if err != nil {
				return "", err
			}
			if part != "" {
				parts = append(parts, part)
			}
		}
		return joinWrapped(parts, " && "), nil
	case types.OrExpr:
		parts := make([]string, 0, len(v.Terms))
		for _, term := range v.Terms {
			part, err := compileFilterExpr(term, allowed)
			if err != nil {
				return "", err
			}
			if part != "" {
				parts = append(parts, part)
			}
		}
		return joinWrapped(parts, " || "), nil
	case types.NotExpr:
		part, err := compileFilterExpr(v.Term, allowed)
		if err != nil {
			return "", err
		}
		if part == "" {
			return "", nil
		}
		return fmt.Sprintf("!(%s)", part), nil
	case types.TermExpr:
		if err := validateFilterField(v.Field, allowed); err != nil {
			return "", err
		}
		return compileTermExpr(v)
	case types.RangeExpr:
		if err := validateFilterField(v.Field, allowed); err != nil {
			return "", err
		}
		field := storageFilterField(v.Field)
		switch {
		case v.GTE != nil && v.LTE != nil:
			return fmt.Sprintf("%s:[%s..%s]", field, encodeScalar(v.GTE), encodeScalar(v.LTE)), nil
		case v.GTE != nil:
			return fmt.Sprintf("%s:>=%s", field, encodeScalar(v.GTE)), nil
		case v.LTE != nil:
			return fmt.Sprintf("%s:<=%s", field, encodeScalar(v.LTE)), nil
		default:
			return "", nil
		}
	case types.ExistsExpr:
		if err := validateFilterField(v.Field, allowed); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s:=%t", existsFieldName(storageFilterField(v.Field)), v.Exists), nil
	default:
		return "", errs.InvalidFilter("unsupported filter expression", map[string]any{"type": expr})
	}
}

func compileTermExpr(expr types.TermExpr) (string, error) {
	field := storageFilterField(expr.Field)
	switch expr.Op {
	case types.FilterOpEQ:
		return fmt.Sprintf("%s:=%s", field, encodeFilterValue(expr.Value)), nil
	case types.FilterOpNEQ:
		return fmt.Sprintf("%s:!=%s", field, encodeFilterValue(expr.Value)), nil
	case types.FilterOpIn:
		values, err := encodeList(expr.Value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s:=[%s]", field, strings.Join(values, ",")), nil
	case types.FilterOpContains:
		return fmt.Sprintf("%s:%s", field, encodeFilterValue(expr.Value)), nil
	default:
		return "", errs.InvalidFilter("unsupported filter operator", map[string]any{"field": expr.Field, "op": expr.Op})
	}
}

func compileSortBy(req types.SearchRequest) string {
	parts := []string{}
	if req.Locale != "" {
		parts = append(parts, fmt.Sprintf("_eval(locale:=%s):desc", encodeFilterValue(req.Locale)))
	}
	for _, sortField := range req.Sort {
		if strings.TrimSpace(sortField.Field) == "" {
			continue
		}
		direction := strings.ToLower(string(sortField.Direction))
		if direction == "" {
			direction = "desc"
		}
		parts = append(parts, fmt.Sprintf("%s:%s", sortField.Field, direction))
	}
	if len(req.Sort) == 0 {
		parts = append(parts, "_text_match:desc")
	}
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ",")
}

func searchQueryFields(def types.IndexDefinition) []string {
	for _, fields := range [][]string{
		def.DefaultQueryFields,
		def.SearchableFields,
	} {
		if len(fields) > 0 {
			return dedupe(fields)
		}
	}
	return []string{"title", "body", "parent_title", "parent_summary"}
}

func allowedFilterFields(def types.IndexDefinition) map[string]struct{} {
	out := map[string]struct{}{
		"id":                     {},
		"index":                  {},
		"type":                   {},
		"parent_id":              {},
		"result_id":              {},
		"match_location":         {},
		"source_type":            {},
		"source_id":              {},
		"locale":                 {},
		"start_ms":               {},
		"end_ms":                 {},
		"track_kind":             {},
		"source_format":          {},
		"topic":                  {},
		"scope_tenant_id":        {},
		"scope_org_id":           {},
		"scope_labels":           {},
		"visibility_public":      {},
		"visibility_roles":       {},
		"visibility_permissions": {},
		"visibility_status":      {},
	}
	for _, field := range def.FilterableFields {
		out[field] = struct{}{}
	}
	for _, field := range def.FacetFields {
		out[field] = struct{}{}
	}
	if def.GroupByDefault != "" {
		out[def.GroupByDefault] = struct{}{}
	}
	return out
}

func storageFilterField(field string) string {
	field = strings.TrimSpace(field)
	if field == "id" {
		return "document_id"
	}
	return field
}

func compileScopeFilter(scope types.Scope) string {
	parts := make([]string, 0, 3)
	if tenantID := strings.TrimSpace(scope.TenantID); tenantID != "" {
		parts = append(parts, fmt.Sprintf("(scope_tenant_id:=%s || scope_tenant_id:=%s)", encodeStringValue(unscopedStorageValue), encodeStringValue(tenantID)))
	} else {
		parts = append(parts, fmt.Sprintf("scope_tenant_id:=%s", encodeStringValue(unscopedStorageValue)))
	}
	if orgID := strings.TrimSpace(scope.OrgID); orgID != "" {
		parts = append(parts, fmt.Sprintf("(scope_org_id:=%s || scope_org_id:=%s)", encodeStringValue(unscopedStorageValue), encodeStringValue(orgID)))
	} else {
		parts = append(parts, fmt.Sprintf("scope_org_id:=%s", encodeStringValue(unscopedStorageValue)))
	}
	if fingerprint := scopeLabelsFingerprint(scope.Labels); fingerprint != "" {
		parts = append(parts, fmt.Sprintf("(scope_labels_fingerprint:=%s || scope_labels_fingerprint:=%s)", encodeStringValue(unscopedStorageValue), encodeStringValue(fingerprint)))
	} else {
		parts = append(parts, fmt.Sprintf("scope_labels_fingerprint:=%s", encodeStringValue(unscopedStorageValue)))
	}
	return strings.Join(parts, " && ")
}

func scopeStorageValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return unscopedStorageValue
	}
	return value
}

func scopeDomainValue(value string) string {
	value = strings.TrimSpace(value)
	if value == unscopedStorageValue {
		return ""
	}
	return value
}

func scopeLabelsFingerprint(labels map[string]string) string {
	tokens := scopeLabelTokens(labels)
	if len(tokens) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(tokens, "\x00")))
	return fmt.Sprintf("%x", sum[:])
}

func scopeLabelTokens(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for key, value := range labels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func validateFilterField(field string, allowed map[string]struct{}) error {
	if _, ok := allowed[field]; ok {
		return nil
	}
	return errs.InvalidFilter("filter field is not declared on the index", map[string]any{"field": field})
}

func localeCandidates(locale string, fallbacks []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, item := range append([]string{locale}, fallbacks...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func localeConstraintValues(locales ...string) []string {
	values := make([]string, 0, len(locales)+1)
	for _, locale := range locales {
		locale = strings.TrimSpace(locale)
		if locale == "" {
			continue
		}
		values = append(values, encodeFilterValue(locale))
	}
	if len(values) == 0 {
		return nil
	}
	values = append(values, encodeStringValue(""))
	return values
}

func encodeFilterValue(value any) string {
	switch v := value.(type) {
	case string:
		return encodeStringValue(v)
	default:
		return encodeScalar(value)
	}
}

func encodeList(value any) ([]string, error) {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, encodeStringValue(item))
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, encodeFilterValue(item))
		}
		return out, nil
	default:
		return nil, errs.InvalidFilter("filter in operator expects a list value", map[string]any{"value": value})
	}
}

func encodeScalar(value any) string {
	switch v := value.(type) {
	case string:
		return encodeStringValue(v)
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func encodeStringValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "``"
	}
	if strings.ContainsAny(value, ", []()&|!`") {
		return "`" + strings.ReplaceAll(value, "`", "\\`") + "`"
	}
	return value
}

func joinWrapped(parts []string, sep string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return "(" + strings.Join(parts, sep) + ")"
	}
}

func dedupe(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
