package locale

import (
	"fmt"
	"strings"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-search/pkg/types"
)

type MatchStrategy int

const (
	MatchExact MatchStrategy = iota
	MatchExactOrParent
	MatchBestFit
)

type Scope int

const (
	ScopeAll Scope = iota
	ScopeActiveOnly
)

type MatchOptions struct {
	Scope Scope
}

type ResolveOptions struct {
	DefaultLocale   string
	MatchStrategy   MatchStrategy
	Scope           Scope
	ExpandParents   bool
	ExpandFallbacks bool
	IncludeDefault  bool
}

type Resolution struct {
	Requested string
	Canonical string
	Matched   string
	Parents   []string
	Fallbacks []string
	Default   string
	Chain     []string
}

type LocaleSearchMetadata struct {
	SearchEnabled     *bool             `json:"search_enabled,omitempty"`
	SearchFallbacks   []string          `json:"search_fallbacks,omitempty"`
	Analyzer          string            `json:"analyzer,omitempty"`
	SemanticModel     string            `json:"semantic_model,omitempty"`
	EmbeddingStrategy string            `json:"embedding_strategy,omitempty"`
	SemanticEnabled   *bool             `json:"semantic_enabled,omitempty"`
	SearchLabels      map[string]string `json:"search_labels,omitempty"`
}

type SearchMetadata = LocaleSearchMetadata

type BoundLocale struct {
	RequestedLocale string `json:"requested_locale,omitempty"`
	CanonicalLocale string `json:"canonical_locale,omitempty"`
	Locale          string `json:"locale,omitempty"`
	Source          string `json:"source,omitempty"`
	Supported       bool   `json:"supported"`
}

type BoundSearchRequest struct {
	Request types.SearchRequest `json:"request"`
	Locale  BoundLocale         `json:"locale"`
}

type BoundSuggestRequest struct {
	Request types.SuggestRequest `json:"request"`
	Locale  BoundLocale          `json:"locale"`
}

type Runtime interface {
	Normalize(locale string) string
	NormalizeMany(locales []string) []string
	NormalizeAndSort(locales []string) []string
	Match(locale string) (string, bool)
	MatchAcceptLanguage(header string) (string, bool)
	MatchAcceptLanguageWithOptions(header string, opts MatchOptions) (string, bool)
	Resolve(locale string, opts ResolveOptions) Resolution
	DecodeMetadata(locale string, out any) error
}

type I18nRuntime struct {
	Catalog       *i18n.LocaleCatalog
	Resolver      i18n.FallbackResolver
	DefaultLocale string
}

func NewI18nRuntime(catalog *i18n.LocaleCatalog, resolver i18n.FallbackResolver, defaultLocale string) *I18nRuntime {
	return &I18nRuntime{
		Catalog:       catalog,
		Resolver:      resolver,
		DefaultLocale: defaultLocale,
	}
}

func NewI18nRuntimeFromCultureData(cultureDataPath, defaultLocale string) (*I18nRuntime, error) {
	cfg, err := i18n.NewConfig(
		i18n.WithCultureData(cultureDataPath),
		i18n.WithDefaultLocale(defaultLocale),
	)
	if err != nil {
		return nil, err
	}
	return NewI18nRuntime(cfg.LocaleCatalog(), cfg.Resolver, cfg.DefaultLocale), nil
}

func Normalize(locale string) string {
	return i18n.NormalizeLocale(locale)
}

func NormalizeMany(locales []string) []string {
	return i18n.NormalizeLocales(locales)
}

func NormalizeAndSort(locales []string) []string {
	return i18n.NormalizeAndSortLocales(locales)
}

func (r *I18nRuntime) Normalize(locale string) string {
	return Normalize(locale)
}

func (r *I18nRuntime) NormalizeMany(locales []string) []string {
	return NormalizeMany(locales)
}

func (r *I18nRuntime) NormalizeAndSort(locales []string) []string {
	return NormalizeAndSort(locales)
}

func (r *I18nRuntime) Match(locale string) (string, bool) {
	if r == nil || r.Catalog == nil {
		return "", false
	}
	meta, ok := r.Catalog.Match(locale)
	if !ok {
		return "", false
	}
	return Normalize(meta.Code), true
}

func (r *I18nRuntime) MatchAcceptLanguage(header string) (string, bool) {
	if r == nil || r.Catalog == nil {
		return "", false
	}
	meta, ok := r.Catalog.MatchAcceptLanguage(header)
	if !ok {
		return "", false
	}
	return Normalize(meta.Code), true
}

func (r *I18nRuntime) MatchAcceptLanguageWithOptions(header string, opts MatchOptions) (string, bool) {
	if r == nil || r.Catalog == nil {
		return "", false
	}
	meta, ok := r.Catalog.MatchAcceptLanguageWithOptions(header, i18n.MatchOptions{
		Scope: toI18nScope(opts.Scope),
	})
	if !ok {
		return "", false
	}
	return Normalize(meta.Code), true
}

func (r *I18nRuntime) Resolve(locale string, opts ResolveOptions) Resolution {
	if r == nil {
		return Resolution{
			Requested: locale,
			Canonical: Normalize(locale),
			Parents:   []string{},
			Fallbacks: []string{},
		}
	}
	resolved := i18n.ResolveLocale(locale, i18n.ResolveLocaleOptions{
		Catalog:         r.Catalog,
		Resolver:        r.Resolver,
		DefaultLocale:   firstNonEmpty(opts.DefaultLocale, r.DefaultLocale),
		MatchStrategy:   toI18nMatchStrategy(opts.MatchStrategy),
		Scope:           toI18nScope(opts.Scope),
		ExpandParents:   opts.ExpandParents,
		ExpandFallbacks: opts.ExpandFallbacks,
		IncludeDefault:  opts.IncludeDefault,
	})
	return Resolution{
		Requested: resolved.Requested,
		Canonical: resolved.Canonical,
		Matched:   resolved.Matched,
		Parents:   append([]string(nil), resolved.Parents...),
		Fallbacks: append([]string(nil), resolved.Fallbacks...),
		Default:   resolved.Default,
		Chain:     append([]string(nil), resolved.Chain...),
	}
}

func (r *I18nRuntime) DecodeMetadata(locale string, out any) error {
	if r == nil || r.Catalog == nil {
		return fmt.Errorf("locale runtime: locale catalog is unavailable")
	}
	return r.Catalog.DecodeMetadata(locale, out)
}

func toI18nMatchStrategy(strategy MatchStrategy) i18n.MatchStrategy {
	switch strategy {
	case MatchExact:
		return i18n.MatchExact
	case MatchBestFit:
		return i18n.MatchBestFit
	default:
		return i18n.MatchExactOrParent
	}
}

func toI18nScope(scope Scope) i18n.LocaleScope {
	switch scope {
	case ScopeActiveOnly:
		return i18n.ScopeActiveOnly
	default:
		return i18n.ScopeAll
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if normalized := Normalize(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func BindLocale(runtime Runtime, requestedLocale, acceptLanguage, defaultLocale string, opts MatchOptions) BoundLocale {
	bound := BoundLocale{
		RequestedLocale: strings.TrimSpace(requestedLocale),
		CanonicalLocale: Normalize(requestedLocale),
		Source:          "empty",
	}

	if bound.CanonicalLocale != "" {
		bound.Locale = bound.CanonicalLocale
		bound.Source = "explicit"
		if runtime != nil {
			if matched, ok := runtime.Match(bound.CanonicalLocale); ok {
				bound.Locale = matched
				bound.Supported = true
			}
		}
		return bound
	}

	if runtime != nil {
		header := strings.TrimSpace(acceptLanguage)
		var (
			matched string
			ok      bool
		)
		if opts.Scope == ScopeAll {
			matched, ok = runtime.MatchAcceptLanguage(header)
		} else {
			matched, ok = runtime.MatchAcceptLanguageWithOptions(header, opts)
		}
		if ok {
			bound.Locale = matched
			bound.Source = "accept_language"
			bound.Supported = true
			return bound
		}
	}

	if normalizedDefault := Normalize(defaultLocale); normalizedDefault != "" {
		bound.Locale = normalizedDefault
		bound.Source = "default"
	}

	return bound
}

func BindSearchRequest(runtime Runtime, req types.SearchRequest, acceptLanguage, defaultLocale string, opts MatchOptions) BoundSearchRequest {
	bound := BindLocale(runtime, req.Locale, acceptLanguage, defaultLocale, opts)
	out := req
	if normalized := firstNonEmpty(bound.Locale, out.Locale); normalized != "" {
		out.Locale = normalized
	}
	return BoundSearchRequest{
		Request: out,
		Locale:  bound,
	}
}

func BindSuggestRequest(runtime Runtime, req types.SuggestRequest, acceptLanguage, defaultLocale string, opts MatchOptions) BoundSuggestRequest {
	bound := BindLocale(runtime, req.Locale, acceptLanguage, defaultLocale, opts)
	out := req
	if normalized := firstNonEmpty(bound.Locale, out.Locale); normalized != "" {
		out.Locale = normalized
	}
	return BoundSuggestRequest{
		Request: out,
		Locale:  bound,
	}
}

func ResolutionOrigins(res Resolution) map[string]string {
	origins := make(map[string]string, len(res.Chain))

	primary := res.Matched
	label := "matched"
	if primary == "" {
		primary = res.Canonical
		label = "requested"
	}
	if primary != "" {
		origins[primary] = label
	}

	for _, parent := range res.Parents {
		parent = Normalize(parent)
		if parent == "" {
			continue
		}
		if _, exists := origins[parent]; !exists {
			origins[parent] = "parent"
		}
	}

	for _, fallback := range res.Fallbacks {
		fallback = Normalize(fallback)
		if fallback == "" {
			continue
		}
		if _, exists := origins[fallback]; !exists {
			origins[fallback] = "fallback"
		}
	}

	if normalizedDefault := Normalize(res.Default); normalizedDefault != "" {
		if _, exists := origins[normalizedDefault]; !exists {
			origins[normalizedDefault] = "default"
		}
	}

	return origins
}
