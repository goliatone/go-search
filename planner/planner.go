package planner

import (
	"context"
	"slices"
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

type IndexRegistry interface {
	GetIndex(name string) (types.IndexDefinition, bool)
	ListIndexes() []types.IndexDefinition
}

type CapabilitySource interface {
	Capabilities(ctx context.Context) (types.CapabilitySet, error)
}

type SearchPlan struct {
	Request types.SearchRequest
	Indexes []types.IndexDefinition
	Locale  LocalePlan
}

type SuggestPlan struct {
	Request types.SuggestRequest
	Indexes []types.IndexDefinition
	Locale  LocalePlan
}

type Defaults struct {
	SearchPage                 int
	SearchPerPage              int
	SuggestLimit               int
	DefaultSearchMode          types.SearchMode
	DisableIndexGroupByDefault bool
}

type LocalePolicy struct {
	MatchStrategy   locale.MatchStrategy
	Scope           locale.Scope
	ExpandParents   bool
	ExpandFallbacks bool
	IncludeDefault  bool
}

type Config struct {
	Registry       IndexRegistry
	LocaleRuntime  locale.Runtime
	LocalePolicy   LocalePolicy
	ScopeGuard     types.ScopeGuard
	CapabilityGate types.CapabilityGate
	Defaults       Defaults
}

type Planner struct {
	registry       IndexRegistry
	localeRuntime  locale.Runtime
	localePolicy   LocalePolicy
	scopeGuard     types.ScopeGuard
	capabilityGate types.CapabilityGate
	defaults       Defaults
}

type LocalePlan struct {
	Requested                  string
	Canonical                  string
	RequestedLocales           []string
	Matched                    string
	Primary                    string
	Chain                      []string
	Fallbacks                  []string
	Origins                    map[string]string
	Resolution                 locale.Resolution
	Metadata                   locale.LocaleSearchMetadata
	SupportedValidationApplied bool
	ActiveValidationApplied    bool
}

func New(cfg Config) (*Planner, error) {
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("planner registry is required", nil)
	}
	cfg.Defaults = normalizeDefaults(cfg.Defaults)
	cfg.LocalePolicy = normalizeLocalePolicy(cfg.LocalePolicy)
	return &Planner{
		registry:       cfg.Registry,
		localeRuntime:  cfg.LocaleRuntime,
		localePolicy:   cfg.LocalePolicy,
		scopeGuard:     cfg.ScopeGuard,
		capabilityGate: cfg.CapabilityGate,
		defaults:       cfg.Defaults,
	}, nil
}

func (p *Planner) NormalizeSearchRequest(req types.SearchRequest) types.SearchRequest {
	if req.Page < 1 {
		req.Page = p.defaults.SearchPage
	}
	if req.PerPage <= 0 {
		req.PerPage = p.defaults.SearchPerPage
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Locale = p.normalizeLocale(req.Locale)
	req.Locales = p.normalizeLocales(req.Locales)
	if req.GroupBy == "" && !p.defaults.DisableIndexGroupByDefault {
		req.GroupBy = p.defaultGroupBy(req.Indexes)
	}
	if req.Mode == "" {
		req.Mode = p.defaults.DefaultSearchMode
	}
	return req
}

func (p *Planner) NormalizeSuggestRequest(req types.SuggestRequest) types.SuggestRequest {
	if req.Limit <= 0 {
		req.Limit = p.defaults.SuggestLimit
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Locale = p.normalizeLocale(req.Locale)
	return req
}

func normalizeDefaults(cfg Defaults) Defaults {
	out := Defaults{
		SearchPage:        1,
		SearchPerPage:     20,
		SuggestLimit:      5,
		DefaultSearchMode: types.SearchModeLexical,
	}
	if cfg.SearchPage > 0 {
		out.SearchPage = cfg.SearchPage
	}
	if cfg.SearchPerPage > 0 {
		out.SearchPerPage = cfg.SearchPerPage
	}
	if cfg.SuggestLimit > 0 {
		out.SuggestLimit = cfg.SuggestLimit
	}
	if cfg.DefaultSearchMode != "" {
		out.DefaultSearchMode = cfg.DefaultSearchMode
	}
	out.DisableIndexGroupByDefault = cfg.DisableIndexGroupByDefault
	return out
}

func normalizeLocalePolicy(policy LocalePolicy) LocalePolicy {
	if policy.MatchStrategy == 0 {
		policy.MatchStrategy = locale.MatchExactOrParent
	}
	return policy
}

func (p *Planner) BuildSearchPlan(ctx context.Context, req types.SearchRequest) (SearchPlan, error) {
	req = p.NormalizeSearchRequest(req)
	if err := p.ValidateSearchRequest(ctx, req); err != nil {
		return SearchPlan{}, err
	}
	indexes, err := p.resolveIndexes(req.Indexes)
	if err != nil {
		return SearchPlan{}, err
	}
	if err := p.validateGroupedSearch(req, indexes); err != nil {
		return SearchPlan{}, err
	}
	if p.scopeGuard != nil && !p.scopeGuard.AllowSearch(ctx, req.Actor, req) {
		return SearchPlan{}, errs.FeatureDisabled("search denied by scope guard", map[string]any{"indexes": req.Indexes})
	}
	plan := SearchPlan{Request: req, Indexes: indexes, Locale: p.buildSearchLocalePlan(req)}
	p.applySearchLocaleMetadata(&plan.Request, plan.Locale)
	return plan, nil
}

func (p *Planner) BuildSuggestPlan(ctx context.Context, req types.SuggestRequest) (SuggestPlan, error) {
	req = p.NormalizeSuggestRequest(req)
	indexes, err := p.resolveIndexes(req.Indexes)
	if err != nil {
		return SuggestPlan{}, err
	}
	if p.scopeGuard != nil && !p.scopeGuard.AllowSuggest(ctx, req.Actor, req) {
		return SuggestPlan{}, errs.FeatureDisabled("suggest denied by scope guard", map[string]any{"indexes": req.Indexes})
	}
	plan := SuggestPlan{Request: req, Indexes: indexes, Locale: p.buildSuggestLocalePlan(req)}
	p.applySuggestLocaleMetadata(&plan.Request, plan.Locale)
	return plan, nil
}

func (p *Planner) ValidateSearchRequest(_ context.Context, req types.SearchRequest) error {
	if len(req.Indexes) == 0 {
		return errs.UnknownIndex("", map[string]any{"reason": "no indexes requested"})
	}
	for _, sort := range req.Sort {
		if sort.Field == "" {
			return errs.InvalidSort("sort field is required", map[string]any{"sort": sort})
		}
		if sort.Direction != "" && sort.Direction != types.SortAsc && sort.Direction != types.SortDesc {
			return errs.InvalidSort("sort direction is invalid", map[string]any{"sort": sort})
		}
	}
	return ValidateFilter(req.Filters)
}

func (p *Planner) ValidateSearchCapabilities(ctx context.Context, req types.SearchRequest, source CapabilitySource) error {
	if source == nil {
		return nil
	}
	caps, err := source.Capabilities(ctx)
	if err != nil {
		return err
	}
	switch req.Mode {
	case "", types.SearchModeLexical:
	case types.SearchModeSemantic:
		if p.capabilityGate != nil && !p.capabilityGate.Enabled(ctx, "search.semantic") {
			return errs.FeatureDisabled("semantic search is disabled", map[string]any{"mode": req.Mode})
		}
		if !supportsMode(caps, types.SearchModeSemantic) || !caps.SemanticSearch {
			return errs.UnsupportedCapability("semantic_search", map[string]any{"mode": req.Mode})
		}
	case types.SearchModeHybrid:
		if p.capabilityGate != nil && !p.capabilityGate.Enabled(ctx, "search.hybrid") {
			return errs.FeatureDisabled("hybrid search is disabled", map[string]any{"mode": req.Mode})
		}
		if !supportsMode(caps, types.SearchModeHybrid) || !caps.HybridSearch {
			return errs.UnsupportedCapability("hybrid_search", map[string]any{"mode": req.Mode})
		}
	default:
		return errs.UnsupportedCapability("search_mode", map[string]any{"mode": req.Mode})
	}
	if req.GroupBy != "" && !caps.Grouping {
		return errs.UnsupportedCapability("grouping", map[string]any{"group_by": req.GroupBy})
	}
	if len(req.Facets) > 0 && !caps.Facets {
		return errs.UnsupportedCapability("facets", map[string]any{"count": len(req.Facets)})
	}
	for _, facet := range req.Facets {
		switch facet.NormalizedKind() {
		case types.FacetKindHierarchical:
			if !caps.HierarchicalFacets {
				return errs.UnsupportedCapability("hierarchical_facets", map[string]any{"field": facet.Field})
			}
		case types.FacetKindNumericRange, types.FacetKindDateRange:
			if !caps.RangeFacets {
				return errs.UnsupportedCapability("range_facets", map[string]any{"field": facet.Field, "kind": facet.Kind})
			}
		}
		if facet.Disjunctive && !caps.DisjunctiveFacets {
			return errs.UnsupportedCapability("disjunctive_facets", map[string]any{"field": facet.Field})
		}
	}
	if len(req.Highlight) > 0 && !caps.Highlighting {
		return errs.UnsupportedCapability("highlighting", map[string]any{"fields": req.Highlight})
	}
	if req.Semantic != nil {
		switch req.Mode {
		case types.SearchModeSemantic, types.SearchModeHybrid:
		default:
			return errs.UnsupportedCapability("semantic_request", map[string]any{"mode": req.Mode})
		}
		if len(req.Semantic.QueryEmbedding) > 0 && !caps.ExternalEmbeddings {
			return errs.UnsupportedCapability("external_embeddings", nil)
		}
		if req.Semantic.DistanceThreshold != nil && !caps.DistanceThreshold {
			return errs.UnsupportedCapability("distance_threshold", nil)
		}
	}
	return nil
}

func ValidateFilter(expr types.FilterExpr) error {
	if expr == nil {
		return nil
	}
	switch v := expr.(type) {
	case types.AndExpr:
		for _, term := range v.Terms {
			if err := ValidateFilter(term); err != nil {
				return err
			}
		}
	case types.OrExpr:
		for _, term := range v.Terms {
			if err := ValidateFilter(term); err != nil {
				return err
			}
		}
	case types.NotExpr:
		return ValidateFilter(v.Term)
	case types.TermExpr:
		if strings.TrimSpace(v.Field) == "" {
			return errs.InvalidFilter("filter field is required", nil)
		}
	case types.RangeExpr:
		if strings.TrimSpace(v.Field) == "" {
			return errs.InvalidFilter("range field is required", nil)
		}
	case types.ExistsExpr:
		if strings.TrimSpace(v.Field) == "" {
			return errs.InvalidFilter("exists field is required", nil)
		}
	default:
		return errs.InvalidFilter("unsupported filter expression", map[string]any{"type": expr})
	}
	return nil
}

func (p *Planner) resolveIndexes(indexes []string) ([]types.IndexDefinition, error) {
	out := make([]types.IndexDefinition, 0, len(indexes))
	for _, name := range indexes {
		def, ok := p.registry.GetIndex(name)
		if !ok {
			return nil, errs.UnknownIndex(name, nil)
		}
		out = append(out, def)
	}
	return out, nil
}

func (p *Planner) defaultGroupBy(indexes []string) string {
	if len(indexes) == 0 {
		return ""
	}
	groupBy := ""
	for _, index := range indexes {
		def, ok := p.registry.GetIndex(index)
		if !ok {
			continue
		}
		value := strings.TrimSpace(def.GroupByDefault)
		if value == "" {
			return ""
		}
		if groupBy == "" {
			groupBy = value
			continue
		}
		if groupBy != value {
			return ""
		}
	}
	return groupBy
}

func (p *Planner) validateGroupedSearch(req types.SearchRequest, indexes []types.IndexDefinition) error {
	if strings.TrimSpace(req.GroupBy) == "" {
		return nil
	}
	for _, def := range indexes {
		if strings.TrimSpace(def.GroupByDefault) == strings.TrimSpace(req.GroupBy) {
			continue
		}
		return errs.InvalidInput("grouped search is not supported for the selected index set", map[string]any{
			"group_by": req.GroupBy,
			"index":    def.Name,
		})
	}
	return nil
}

func supportsMode(caps types.CapabilitySet, mode types.SearchMode) bool {
	if len(caps.SupportedSearchModes) == 0 {
		return mode == types.SearchModeLexical
	}
	return slices.Contains(caps.SupportedSearchModes, mode)
}

func (p *Planner) ScopeGuard() types.ScopeGuard {
	return p.scopeGuard
}

func (p *Planner) normalizeLocale(input string) string {
	if p.localeRuntime != nil {
		return p.localeRuntime.Normalize(input)
	}
	return locale.Normalize(input)
}

func (p *Planner) normalizeLocales(locales []string) []string {
	if p.localeRuntime != nil {
		return p.localeRuntime.NormalizeMany(locales)
	}
	return locale.NormalizeMany(locales)
}

func (p *Planner) localeResolveOptions() locale.ResolveOptions {
	return locale.ResolveOptions{
		MatchStrategy:   p.localePolicy.MatchStrategy,
		Scope:           p.localePolicy.Scope,
		ExpandParents:   p.localePolicy.ExpandParents,
		ExpandFallbacks: p.localePolicy.ExpandFallbacks,
		IncludeDefault:  p.localePolicy.IncludeDefault,
	}
}

func (p *Planner) decodeLocaleMetadata(code string) (locale.LocaleSearchMetadata, bool) {
	if p.localeRuntime == nil || code == "" {
		return locale.LocaleSearchMetadata{}, false
	}

	var out locale.LocaleSearchMetadata
	if err := p.localeRuntime.DecodeMetadata(code, &out); err != nil {
		return locale.LocaleSearchMetadata{}, false
	}
	return out, true
}

func (p *Planner) filterEnabledLocales(candidates []string) (string, []string, locale.LocaleSearchMetadata) {
	if len(candidates) == 0 {
		return "", nil, locale.LocaleSearchMetadata{}
	}

	fallbacks := make([]string, 0, len(candidates))
	first := candidates[0]
	firstMeta, firstHasMeta := p.decodeLocaleMetadata(first)
	primary := ""
	primaryMeta := locale.LocaleSearchMetadata{}

	for _, candidate := range candidates {
		meta, ok := p.decodeLocaleMetadata(candidate)
		enabled := true
		if ok && meta.SearchEnabled != nil && !*meta.SearchEnabled {
			enabled = false
		}

		if primary == "" && enabled {
			primary = candidate
			primaryMeta = meta
			continue
		}
		if primary != "" && enabled {
			fallbacks = append(fallbacks, candidate)
		}
	}

	if primary != "" {
		return primary, fallbacks, primaryMeta
	}
	if firstHasMeta {
		return first, nil, firstMeta
	}
	return first, nil, locale.LocaleSearchMetadata{}
}

func ensureRequestMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}

func cloneStringMapToAny(input map[string]string) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (p *Planner) applySearchLocaleMetadata(req *types.SearchRequest, localePlan LocalePlan) {
	if req == nil {
		return
	}

	req.Metadata = ensureRequestMetadata(req.Metadata)
	if len(localePlan.Chain) > 0 {
		req.Metadata["locale_chain"] = append([]string(nil), localePlan.Chain...)
	}
	if len(localePlan.Origins) > 0 {
		req.Metadata["locale_origins"] = cloneStringMapToAny(localePlan.Origins)
	}
	if localePlan.Metadata.SearchEnabled != nil {
		req.Metadata["locale_search_enabled"] = *localePlan.Metadata.SearchEnabled
	}
	if localePlan.Metadata.Analyzer != "" {
		req.Metadata["locale_analyzer"] = localePlan.Metadata.Analyzer
	}
	if localePlan.Metadata.EmbeddingStrategy != "" {
		req.Metadata["locale_embedding_strategy"] = localePlan.Metadata.EmbeddingStrategy
	}
	if localePlan.Metadata.SemanticModel != "" {
		req.Metadata["locale_semantic_model"] = localePlan.Metadata.SemanticModel
	}
	if len(localePlan.Metadata.SearchLabels) > 0 {
		req.Metadata["locale_search_labels"] = cloneStringMapToAny(localePlan.Metadata.SearchLabels)
	}
	if req.Semantic != nil && req.Semantic.Model == "" && localePlan.Metadata.SemanticModel != "" {
		req.Semantic.Model = localePlan.Metadata.SemanticModel
	}
}

func (p *Planner) applySuggestLocaleMetadata(req *types.SuggestRequest, localePlan LocalePlan) {
	if req == nil {
		return
	}

	req.Metadata = ensureRequestMetadata(req.Metadata)
	if len(localePlan.Chain) > 0 {
		req.Metadata["locale_chain"] = append([]string(nil), localePlan.Chain...)
	}
	if len(localePlan.Origins) > 0 {
		req.Metadata["locale_origins"] = cloneStringMapToAny(localePlan.Origins)
	}
	if len(localePlan.Metadata.SearchLabels) > 0 {
		req.Metadata["locale_search_labels"] = cloneStringMapToAny(localePlan.Metadata.SearchLabels)
	}
}
