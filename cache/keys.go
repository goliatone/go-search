package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

type generationLookup interface {
	Get(ctx context.Context, index string) (int64, error)
}

type cacheKeyDetails struct {
	Key                   string
	BaseKey               string
	Indexes               []string
	GenerationFingerprint string
}

func searchCacheKey(ctx context.Context, provider string, req types.SearchRequest, generations generationLookup) (string, error) {
	details, err := searchCacheDetails(ctx, provider, req, generations)
	if err != nil {
		return "", err
	}
	return details.Key, nil
}

func searchCacheDetails(ctx context.Context, provider string, req types.SearchRequest, generations generationLookup) (cacheKeyDetails, error) {
	indexes := normalizeIndexes(req.Indexes)
	generationMap, err := indexGenerations(ctx, indexes, generations)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	payload := map[string]any{
		"provider":        strings.TrimSpace(provider),
		"indexes":         indexes,
		"query":           normalizeQuery(req.Query),
		"locale":          locale.Normalize(req.Locale),
		"locales":         locale.NormalizeAndSort(req.Locales),
		"page":            req.Page,
		"per_page":        req.PerPage,
		"sort":            normalizeSorts(req.Sort),
		"filters":         normalizeFilter(req.Filters),
		"facets":          normalizeFacets(req.Facets),
		"group_by":        strings.TrimSpace(req.GroupBy),
		"highlight":       sortStrings(req.Highlight),
		"include_fields":  sortStrings(req.IncludeFields),
		"ranking_profile": strings.TrimSpace(req.RankingProfile),
		"mode":            req.Mode,
		"semantic":        normalizeSemantic(req.Semantic),
		"metadata":        normalizeMap(req.Metadata),
		"actor":           normalizeActor(req.Actor),
		"scope":           normalizeScope(req.Scope),
	}
	baseKey, err := hashPayload("search-base", payload)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	payloadWithGeneration := maps.Clone(payload)
	payloadWithGeneration["generations"] = generationMap
	key, err := hashPayload("search", payloadWithGeneration)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	fingerprint, err := generationFingerprint(generationMap)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	return cacheKeyDetails{
		Key:                   key,
		BaseKey:               baseKey,
		Indexes:               indexes,
		GenerationFingerprint: fingerprint,
	}, nil
}

func suggestCacheKey(ctx context.Context, provider string, req types.SuggestRequest, generations generationLookup) (string, error) {
	details, err := suggestCacheDetails(ctx, provider, req, generations)
	if err != nil {
		return "", err
	}
	return details.Key, nil
}

func suggestCacheDetails(ctx context.Context, provider string, req types.SuggestRequest, generations generationLookup) (cacheKeyDetails, error) {
	indexes := normalizeIndexes(req.Indexes)
	generationMap, err := indexGenerations(ctx, indexes, generations)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	payload := map[string]any{
		"provider":      strings.TrimSpace(provider),
		"indexes":       indexes,
		"query":         normalizeQuery(req.Query),
		"locale":        locale.Normalize(req.Locale),
		"limit":         req.Limit,
		"prefer_parent": req.PreferParent,
		"metadata":      normalizeMap(req.Metadata),
		"actor":         normalizeActor(req.Actor),
		"scope":         normalizeScope(req.Scope),
	}
	baseKey, err := hashPayload("suggest-base", payload)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	payloadWithGeneration := maps.Clone(payload)
	payloadWithGeneration["generations"] = generationMap
	key, err := hashPayload("suggest", payloadWithGeneration)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	fingerprint, err := generationFingerprint(generationMap)
	if err != nil {
		return cacheKeyDetails{}, err
	}
	return cacheKeyDetails{
		Key:                   key,
		BaseKey:               baseKey,
		Indexes:               indexes,
		GenerationFingerprint: fingerprint,
	}, nil
}

func metadataCacheKey(provider, kind string, indexes []string) (string, error) {
	payload := map[string]any{
		"provider": strings.TrimSpace(provider),
		"kind":     strings.TrimSpace(kind),
		"indexes":  normalizeIndexes(indexes),
	}
	return hashPayload("metadata", payload)
}

func hashPayload(prefix string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%s:%s", prefix, hex.EncodeToString(sum[:])), nil
}

func indexGenerations(ctx context.Context, indexes []string, store generationLookup) (map[string]int64, error) {
	if store == nil {
		return nil, nil
	}
	out := make(map[string]int64, len(indexes))
	for _, index := range indexes {
		generation, err := store.Get(ctx, index)
		if err != nil {
			return nil, err
		}
		out[index] = generation
	}
	return out, nil
}

func generationFingerprint(generationMap map[string]int64) (string, error) {
	if len(generationMap) == 0 {
		return "", nil
	}
	return hashPayload("generation", generationMap)
}

func normalizeIndexes(indexes []string) []string {
	return sortStrings(indexes)
}

func sortStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
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
	sort.Strings(out)
	return out
}

func normalizeQuery(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func normalizeSorts(sorts []types.Sort) []map[string]any {
	if len(sorts) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(sorts))
	for _, item := range sorts {
		out = append(out, map[string]any{
			"field":     strings.TrimSpace(item.Field),
			"direction": item.Direction,
		})
	}
	return out
}

func normalizeSemantic(semantic *types.SemanticRequest) map[string]any {
	if semantic == nil {
		return nil
	}
	out := map[string]any{
		"field":              strings.TrimSpace(semantic.Field),
		"query_text":         strings.TrimSpace(semantic.QueryText),
		"query_embedding":    semantic.QueryEmbedding,
		"k":                  semantic.K,
		"distance_threshold": semantic.DistanceThreshold,
		"alpha":              semantic.Alpha,
		"rerank":             semantic.Rerank,
		"locale_strategy":    strings.TrimSpace(semantic.LocaleStrategy),
		"model":              strings.TrimSpace(semantic.Model),
		"metadata":           normalizeMap(semantic.Metadata),
	}
	return out
}

func normalizeFilter(expr types.FilterExpr) any {
	switch v := expr.(type) {
	case nil:
		return nil
	case types.AndExpr:
		items := make([]any, 0, len(v.Terms))
		for _, term := range v.Terms {
			items = append(items, normalizeFilter(term))
		}
		return map[string]any{"kind": "and", "terms": items}
	case types.OrExpr:
		items := make([]any, 0, len(v.Terms))
		for _, term := range v.Terms {
			items = append(items, normalizeFilter(term))
		}
		return map[string]any{"kind": "or", "terms": items}
	case types.NotExpr:
		return map[string]any{"kind": "not", "term": normalizeFilter(v.Term)}
	case types.TermExpr:
		value := normalizeValue(v.Value)
		if v.Op == types.FilterOpIn {
			value = normalizeInValues(v.Value)
		}
		return map[string]any{
			"kind":  "term",
			"field": strings.TrimSpace(v.Field),
			"op":    v.Op,
			"value": value,
		}
	case types.RangeExpr:
		return map[string]any{
			"kind":  "range",
			"field": strings.TrimSpace(v.Field),
			"gte":   normalizeValue(v.GTE),
			"lte":   normalizeValue(v.LTE),
		}
	case types.ExistsExpr:
		return map[string]any{
			"kind":   "exists",
			"field":  strings.TrimSpace(v.Field),
			"exists": v.Exists,
		}
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func normalizeFacets(facets []types.FacetRequest) []map[string]any {
	if len(facets) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(facets))
	for _, facet := range facets {
		out = append(out, map[string]any{
			"field":       strings.TrimSpace(facet.Field),
			"limit":       facet.Limit,
			"kind":        facet.Kind,
			"disjunctive": facet.Disjunctive,
			"separator":   strings.TrimSpace(facet.Separator),
			"path":        append([]string(nil), facet.Path...),
			"metadata":    normalizeMap(facet.Metadata),
		})
	}
	return out
}

func normalizeActor(actor types.ActorRef) map[string]any {
	if strings.TrimSpace(actor.UserID) == "" &&
		strings.TrimSpace(actor.TenantID) == "" &&
		strings.TrimSpace(actor.OrgID) == "" &&
		len(actor.Metadata) == 0 {
		return nil
	}
	return map[string]any{
		"user_id":   strings.TrimSpace(actor.UserID),
		"tenant_id": strings.TrimSpace(actor.TenantID),
		"org_id":    strings.TrimSpace(actor.OrgID),
		"metadata":  normalizeMap(actor.Metadata),
	}
}

func normalizeScope(scope types.Scope) map[string]any {
	if strings.TrimSpace(scope.TenantID) == "" &&
		strings.TrimSpace(scope.OrgID) == "" &&
		len(scope.Labels) == 0 {
		return nil
	}
	return map[string]any{
		"tenant_id": strings.TrimSpace(scope.TenantID),
		"org_id":    strings.TrimSpace(scope.OrgID),
		"labels":    normalizeStringMap(scope.Labels),
	}
}

func normalizeMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = normalizeValue(value)
	}
	return out
}

func normalizeStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case []string:
		out := append([]string(nil), v...)
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, normalizeValue(item))
		}
		return out
	case map[string]any:
		return normalizeMap(v)
	case map[string]string:
		return normalizeStringMap(v)
	default:
		return v
	}
}

func normalizeInValues(value any) any {
	switch v := value.(type) {
	case []string:
		return sortStrings(v)
	case []any:
		if len(v) == 0 {
			return nil
		}
		type item struct {
			key   string
			value any
		}
		items := make([]item, 0, len(v))
		seen := map[string]struct{}{}
		for _, raw := range v {
			normalized := normalizeValue(raw)
			key := strings.TrimSpace(fmt.Sprint(normalized))
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, item{key: key, value: normalized})
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].key < items[j].key
		})
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, item.value)
		}
		return out
	default:
		return normalizeValue(value)
	}
}
