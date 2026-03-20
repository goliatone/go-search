package planner

import (
	"slices"
	"strings"

	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

func (p *Planner) buildSearchLocalePlan(req types.SearchRequest) LocalePlan {
	plan := p.baseLocalePlan(req.Locale, req.Locales)
	if plan.Primary == "" {
		plan.Chain = append([]string(nil), plan.Fallbacks...)
		return plan
	}
	plan.Chain = append([]string{plan.Primary}, plan.Fallbacks...)
	return plan
}

func (p *Planner) buildSuggestLocalePlan(req types.SuggestRequest) LocalePlan {
	plan := p.baseLocalePlan(req.Locale, nil)
	if plan.Primary == "" {
		return plan
	}
	plan.Chain = []string{plan.Primary}
	plan.Fallbacks = nil
	return plan
}

func (p *Planner) baseLocalePlan(requested string, requestedLocales []string) LocalePlan {
	canonical := p.normalizeLocale(requested)
	explicit := p.normalizeLocales(requestedLocales)
	plan := LocalePlan{
		Requested:        requested,
		Canonical:        canonical,
		RequestedLocales: explicit,
		Origins:          map[string]string{},
	}
	if canonical == "" {
		plan.Fallbacks = append([]string(nil), explicit...)
		return plan
	}
	plan.Primary = canonical
	if p.localeRuntime != nil {
		plan.SupportedValidationApplied = true
		plan.ActiveValidationApplied = p.localePolicy.Scope == locale.ScopeActiveOnly
		plan.Resolution = p.localeRuntime.Resolve(canonical, p.localeResolveOptions())
		plan.Origins = locale.ResolutionOrigins(plan.Resolution)
		plan.Canonical = plan.Resolution.Canonical
		plan.Matched = plan.Resolution.Matched

		resolutionChain := append([]string(nil), plan.Resolution.Chain...)
		if len(resolutionChain) == 0 {
			primary := plan.Canonical
			if plan.Matched != "" {
				primary = plan.Matched
			}
			if primary != "" {
				resolutionChain = append(resolutionChain, primary)
			}
		}
		if primary, resolvedFallbacks, metadata := p.filterEnabledLocales(resolutionChain); primary != "" {
			plan.Primary = primary
			plan.Fallbacks = append(plan.Fallbacks, resolvedFallbacks...)
			plan.Metadata = metadata
		}
	}
	for _, extra := range filterLocales(explicit, plan.Primary) {
		if addLocale(&plan.Fallbacks, extra) {
			if _, exists := plan.Origins[extra]; !exists {
				plan.Origins[extra] = "explicit"
			}
		}
	}
	return plan
}

func (p SearchPlan) ProviderRequest() types.SearchRequest {
	req := p.Request
	req.Locale = p.Locale.Primary
	if p.Locale.Primary == "" {
		req.Locales = append([]string(nil), p.Locale.RequestedLocales...)
		return req
	}
	req.Locales = append([]string(nil), p.Locale.Fallbacks...)
	return req
}

func (p SuggestPlan) ProviderRequest() types.SuggestRequest {
	req := p.Request
	req.Locale = p.Locale.Primary
	return req
}

func (p LocalePlan) IsExact(got string) bool {
	return p.Primary != "" && strings.EqualFold(strings.TrimSpace(got), p.Primary)
}

func (p LocalePlan) MatchLabel(got string) string {
	got = locale.Normalize(got)
	switch {
	case got == "":
		return "any"
	case p.IsExact(got):
		return "exact"
	case p.Origins[got] == "parent" || p.Origins[got] == "fallback" || p.Origins[got] == "default" || p.Origins[got] == "explicit":
		return "fallback"
	case slices.Contains(p.Fallbacks, got):
		return "fallback"
	default:
		return "none"
	}
}

func (p LocalePlan) Origin(got string) string {
	return p.Origins[locale.Normalize(got)]
}

func filterLocales(locales []string, excluded string) []string {
	if len(locales) == 0 {
		return nil
	}
	out := make([]string, 0, len(locales))
	for _, locale := range locales {
		if excluded != "" && strings.EqualFold(locale, excluded) {
			continue
		}
		out = append(out, locale)
	}
	return out
}

func addLocale(dst *[]string, candidate string) bool {
	candidate = locale.Normalize(candidate)
	if candidate == "" {
		return false
	}
	if slices.Contains(*dst, candidate) {
		return false
	}
	*dst = append(*dst, candidate)
	return true
}
