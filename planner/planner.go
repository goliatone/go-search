package planner

import (
	"context"
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type IndexRegistry interface {
	GetIndex(name string) (types.IndexDefinition, bool)
	ListIndexes() []types.IndexDefinition
}

type SearchPlan struct {
	Request types.SearchRequest
	Indexes []types.IndexDefinition
}

type SuggestPlan struct {
	Request types.SuggestRequest
	Indexes []types.IndexDefinition
}

type Config struct {
	Registry       IndexRegistry
	LocalePolicy   types.LocalePolicy
	ScopeGuard     types.ScopeGuard
	CapabilityGate types.CapabilityGate
}

type Planner struct {
	registry       IndexRegistry
	localePolicy   types.LocalePolicy
	scopeGuard     types.ScopeGuard
	capabilityGate types.CapabilityGate
}

func New(cfg Config) (*Planner, error) {
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("planner registry is required", nil)
	}
	return &Planner{
		registry:       cfg.Registry,
		localePolicy:   cfg.LocalePolicy,
		scopeGuard:     cfg.ScopeGuard,
		capabilityGate: cfg.CapabilityGate,
	}, nil
}

func (p *Planner) NormalizeSearchRequest(req types.SearchRequest) types.SearchRequest {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PerPage <= 0 {
		req.PerPage = 20
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Locale = p.normalizeLocale(req.Locale)
	req.Locales = p.normalizeLocales(req.Locales)
	if req.GroupBy == "" {
		for _, index := range req.Indexes {
			if def, ok := p.registry.GetIndex(index); ok && def.GroupByDefault != "" {
				req.GroupBy = def.GroupByDefault
				break
			}
		}
	}
	if req.Mode == "" {
		req.Mode = types.SearchModeLexical
	}
	return req
}

func (p *Planner) NormalizeSuggestRequest(req types.SuggestRequest) types.SuggestRequest {
	if req.Limit <= 0 {
		req.Limit = 5
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Locale = p.normalizeLocale(req.Locale)
	return req
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
	if p.scopeGuard != nil && !p.scopeGuard.AllowSearch(ctx, req.Actor, req) {
		return SearchPlan{}, errs.FeatureDisabled("search denied by scope guard", map[string]any{"indexes": req.Indexes})
	}
	return SearchPlan{Request: req, Indexes: indexes}, nil
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
	return SuggestPlan{Request: req, Indexes: indexes}, nil
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

func (p *Planner) normalizeLocale(locale string) string {
	if p.localePolicy != nil {
		return p.localePolicy.Normalize(locale)
	}
	return strings.TrimSpace(locale)
}

func (p *Planner) normalizeLocales(locales []string) []string {
	if p.localePolicy != nil {
		return p.localePolicy.NormalizeMany(locales)
	}
	out := make([]string, 0, len(locales))
	seen := map[string]struct{}{}
	for _, locale := range locales {
		locale = strings.TrimSpace(locale)
		if locale == "" {
			continue
		}
		if _, ok := seen[locale]; ok {
			continue
		}
		seen[locale] = struct{}{}
		out = append(out, locale)
	}
	return out
}
