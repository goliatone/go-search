package locale

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

type consumerFixture struct {
	Normalization []struct {
		Name             string   `json:"name"`
		Locale           string   `json:"locale"`
		Locales          []string `json:"locales"`
		CanonicalLocale  string   `json:"canonical_locale"`
		CanonicalLocales []string `json:"canonical_locales"`
		CacheKeyLocales  []string `json:"cache_key_locales"`
	} `json:"normalization"`
	RequestBinding []struct {
		Name            string `json:"name"`
		RequestedLocale string `json:"requested_locale"`
		AcceptLanguage  string `json:"accept_language"`
		DefaultLocale   string `json:"default_locale"`
		Scope           string `json:"scope"`
		BoundLocale     string `json:"bound_locale"`
		Source          string `json:"source"`
		Supported       bool   `json:"supported"`
	} `json:"request_binding"`
	MetadataDecode []struct {
		Name     string               `json:"name"`
		Locale   string               `json:"locale"`
		Expected LocaleSearchMetadata `json:"expected"`
	} `json:"metadata_decode"`
	Resolution []struct {
		Name    string `json:"name"`
		Locale  string `json:"locale"`
		Options struct {
			DefaultLocale   string `json:"default_locale"`
			MatchStrategy   string `json:"match_strategy"`
			Scope           string `json:"scope"`
			ExpandParents   bool   `json:"expand_parents"`
			ExpandFallbacks bool   `json:"expand_fallbacks"`
			IncludeDefault  bool   `json:"include_default"`
		} `json:"options"`
		Expected struct {
			Canonical string   `json:"canonical"`
			Matched   string   `json:"matched"`
			Parents   []string `json:"parents"`
			Fallbacks []string `json:"fallbacks"`
			Default   string   `json:"default"`
			Chain     []string `json:"chain"`
		} `json:"expected"`
		Origins map[string]string `json:"origins"`
	} `json:"resolution"`
}

func TestNormalizeHelpers(t *testing.T) {
	if got := Normalize(" ES_mx "); got != "es-MX" {
		t.Fatalf("Normalize = %q, want %q", got, "es-MX")
	}
	gotMany := NormalizeMany([]string{" es_mx ", "ES-MX", "es"})
	if len(gotMany) != 2 || gotMany[0] != "es-MX" || gotMany[1] != "es" {
		t.Fatalf("NormalizeMany = %#v", gotMany)
	}
	gotSorted := NormalizeAndSort([]string{"es", " es_mx ", "ES-MX"})
	if len(gotSorted) != 2 || gotSorted[0] != "es" || gotSorted[1] != "es-MX" {
		t.Fatalf("NormalizeAndSort = %#v", gotSorted)
	}
}

func TestI18nRuntimeResolveWithoutCatalogStillCanonicalizes(t *testing.T) {
	runtime := NewI18nRuntime(nil, nil, "")
	resolved := runtime.Resolve("ES_mx", ResolveOptions{})
	if resolved.Canonical != "es-MX" {
		t.Fatalf("Resolve canonical = %q", resolved.Canonical)
	}
	if resolved.Matched != "" {
		t.Fatalf("Resolve matched = %q", resolved.Matched)
	}
	if len(resolved.Chain) != 1 || resolved.Chain[0] != "es-MX" {
		t.Fatalf("Resolve chain = %#v", resolved.Chain)
	}
}

func TestLocaleConsumerFixture_NormalizationAndCacheKeys(t *testing.T) {
	fixture := loadConsumerFixture(t)

	for _, tc := range fixture.Normalization {
		t.Run(tc.Name, func(t *testing.T) {
			if got := Normalize(tc.Locale); got != tc.CanonicalLocale {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.Locale, got, tc.CanonicalLocale)
			}
			if got := NormalizeMany(tc.Locales); !reflect.DeepEqual(got, tc.CanonicalLocales) {
				t.Fatalf("NormalizeMany(%#v) = %#v, want %#v", tc.Locales, got, tc.CanonicalLocales)
			}
			if got := NormalizeAndSort(tc.Locales); !reflect.DeepEqual(got, tc.CacheKeyLocales) {
				t.Fatalf("NormalizeAndSort(%#v) = %#v, want %#v", tc.Locales, got, tc.CacheKeyLocales)
			}
		})
	}
}

func TestLocaleConsumerFixture_RequestBinding(t *testing.T) {
	runtime := mustBuildRuntime(t)
	fixture := loadConsumerFixture(t)

	for _, tc := range fixture.RequestBinding {
		t.Run(tc.Name, func(t *testing.T) {
			bound := BindLocale(runtime, tc.RequestedLocale, tc.AcceptLanguage, tc.DefaultLocale, MatchOptions{
				Scope: parseScope(tc.Scope),
			})
			if bound.Locale != tc.BoundLocale {
				t.Fatalf("bound locale = %q, want %q", bound.Locale, tc.BoundLocale)
			}
			if bound.Source != tc.Source {
				t.Fatalf("binding source = %q, want %q", bound.Source, tc.Source)
			}
			if bound.Supported != tc.Supported {
				t.Fatalf("supported = %v, want %v", bound.Supported, tc.Supported)
			}
		})
	}
}

func TestBindSearchRequestAppliesTransportLocaleWithoutMutatingOtherFields(t *testing.T) {
	runtime := mustBuildRuntime(t)

	bound := BindSearchRequest(runtime, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Page:    2,
		PerPage: 15,
	}, "fr-CA,fr;q=0.9,en;q=0.8", "en", MatchOptions{Scope: ScopeActiveOnly})

	if bound.Request.Locale != "en" {
		t.Fatalf("request locale = %q", bound.Request.Locale)
	}
	if bound.Locale.Source != "accept_language" {
		t.Fatalf("binding source = %q", bound.Locale.Source)
	}
	if bound.Request.Page != 2 || bound.Request.PerPage != 15 {
		t.Fatalf("unexpected request mutation: %+v", bound.Request)
	}
}

func TestBindSuggestRequestPrefersExplicitLocaleAndPreservesLimit(t *testing.T) {
	runtime := mustBuildRuntime(t)

	bound := BindSuggestRequest(runtime, types.SuggestRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "ES_mx",
		Limit:   7,
	}, "fr-CA,fr;q=0.9,en;q=0.8", "en", MatchOptions{Scope: ScopeActiveOnly})

	if bound.Request.Locale != "es-419" {
		t.Fatalf("request locale = %q", bound.Request.Locale)
	}
	if bound.Locale.Source != "explicit" {
		t.Fatalf("binding source = %q", bound.Locale.Source)
	}
	if !bound.Locale.Supported {
		t.Fatalf("expected explicit locale to be supported")
	}
	if bound.Request.Limit != 7 {
		t.Fatalf("limit = %d", bound.Request.Limit)
	}
}

func TestLocaleConsumerFixture_MetadataDecode(t *testing.T) {
	runtime := mustBuildRuntime(t)
	fixture := loadConsumerFixture(t)

	for _, tc := range fixture.MetadataDecode {
		t.Run(tc.Name, func(t *testing.T) {
			var out LocaleSearchMetadata
			if err := runtime.DecodeMetadata(tc.Locale, &out); err != nil {
				t.Fatalf("DecodeMetadata(%q): %v", tc.Locale, err)
			}
			assertLocaleSearchMetadataEqual(t, out, tc.Expected)
		})
	}
}

func TestLocaleConsumerFixture_Resolution(t *testing.T) {
	runtime := mustBuildRuntime(t)
	fixture := loadConsumerFixture(t)

	for _, tc := range fixture.Resolution {
		t.Run(tc.Name, func(t *testing.T) {
			resolved := runtime.Resolve(tc.Locale, ResolveOptions{
				DefaultLocale:   tc.Options.DefaultLocale,
				MatchStrategy:   parseMatchStrategy(tc.Options.MatchStrategy),
				Scope:           parseScope(tc.Options.Scope),
				ExpandParents:   tc.Options.ExpandParents,
				ExpandFallbacks: tc.Options.ExpandFallbacks,
				IncludeDefault:  tc.Options.IncludeDefault,
			})
			if resolved.Canonical != tc.Expected.Canonical {
				t.Fatalf("Canonical = %q, want %q", resolved.Canonical, tc.Expected.Canonical)
			}
			if resolved.Matched != tc.Expected.Matched {
				t.Fatalf("Matched = %q, want %q", resolved.Matched, tc.Expected.Matched)
			}
			if !reflect.DeepEqual(resolved.Parents, tc.Expected.Parents) {
				t.Fatalf("Parents = %#v, want %#v", resolved.Parents, tc.Expected.Parents)
			}
			if !reflect.DeepEqual(resolved.Fallbacks, tc.Expected.Fallbacks) {
				t.Fatalf("Fallbacks = %#v, want %#v", resolved.Fallbacks, tc.Expected.Fallbacks)
			}
			if resolved.Default != tc.Expected.Default {
				t.Fatalf("Default = %q, want %q", resolved.Default, tc.Expected.Default)
			}
			if !reflect.DeepEqual(resolved.Chain, tc.Expected.Chain) {
				t.Fatalf("Chain = %#v, want %#v", resolved.Chain, tc.Expected.Chain)
			}
			if origins := ResolutionOrigins(resolved); !reflect.DeepEqual(origins, tc.Origins) {
				t.Fatalf("ResolutionOrigins = %#v, want %#v", origins, tc.Origins)
			}
		})
	}
}

func mustBuildRuntime(t *testing.T) *I18nRuntime {
	t.Helper()

	runtime, err := NewI18nRuntimeFromCultureData(filepath.Join("..", "testdata", "locale_search_culture.json"), "en")
	if err != nil {
		t.Fatalf("NewI18nRuntimeFromCultureData: %v", err)
	}

	return runtime
}

func loadConsumerFixture(t *testing.T) consumerFixture {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "testdata", "locale_consumer_matrix.json"))
	if err != nil {
		t.Fatalf("read consumer fixture: %v", err)
	}

	var fixture consumerFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode consumer fixture: %v", err)
	}
	return fixture
}

func parseScope(value string) Scope {
	if value == "active_only" {
		return ScopeActiveOnly
	}
	return ScopeAll
}

func parseMatchStrategy(value string) MatchStrategy {
	switch value {
	case "exact":
		return MatchExact
	case "best_fit":
		return MatchBestFit
	default:
		return MatchExactOrParent
	}
}

func assertLocaleSearchMetadataEqual(t *testing.T, got, want LocaleSearchMetadata) {
	t.Helper()

	if (got.SearchEnabled == nil) != (want.SearchEnabled == nil) {
		t.Fatalf("SearchEnabled presence mismatch: got %#v want %#v", got.SearchEnabled, want.SearchEnabled)
	}
	if got.SearchEnabled != nil && want.SearchEnabled != nil && *got.SearchEnabled != *want.SearchEnabled {
		t.Fatalf("SearchEnabled = %v, want %v", *got.SearchEnabled, *want.SearchEnabled)
	}
	if !reflect.DeepEqual(got.SearchFallbacks, want.SearchFallbacks) {
		t.Fatalf("SearchFallbacks = %#v, want %#v", got.SearchFallbacks, want.SearchFallbacks)
	}
	if got.Analyzer != want.Analyzer {
		t.Fatalf("Analyzer = %q, want %q", got.Analyzer, want.Analyzer)
	}
	if got.SemanticModel != want.SemanticModel {
		t.Fatalf("SemanticModel = %q, want %q", got.SemanticModel, want.SemanticModel)
	}
	if got.EmbeddingStrategy != want.EmbeddingStrategy {
		t.Fatalf("EmbeddingStrategy = %q, want %q", got.EmbeddingStrategy, want.EmbeddingStrategy)
	}
	if (got.SemanticEnabled == nil) != (want.SemanticEnabled == nil) {
		t.Fatalf("SemanticEnabled presence mismatch: got %#v want %#v", got.SemanticEnabled, want.SemanticEnabled)
	}
	if got.SemanticEnabled != nil && want.SemanticEnabled != nil && *got.SemanticEnabled != *want.SemanticEnabled {
		t.Fatalf("SemanticEnabled = %v, want %v", *got.SemanticEnabled, *want.SemanticEnabled)
	}
	if !reflect.DeepEqual(got.SearchLabels, want.SearchLabels) {
		t.Fatalf("SearchLabels = %#v, want %#v", got.SearchLabels, want.SearchLabels)
	}
}
