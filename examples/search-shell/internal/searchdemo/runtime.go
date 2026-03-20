package searchdemo

import (
	"context"
	"fmt"
	"path/filepath"
	runtimex "runtime"
	"strings"

	"github.com/goliatone/go-search/command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers/memory"
	"github.com/goliatone/go-search/query"
)

type Config struct {
	Provider        string
	SeedOnStart     bool
	IndexName       string
	DefaultLocale   string
	CultureDataPath string
}

type Runtime struct {
	provider        *memory.Provider
	registry        *indexing.Registry
	planner         *planner.Planner
	localeRuntime   *locale.I18nRuntime
	ensureIndex     *command.EnsureIndex
	upsert          *command.UpsertDocuments
	search          *query.Search
	suggest         *query.Suggest
	health          *query.Health
	index           types.IndexDefinition
	cultureDataPath string
	defaultLocale   string
	seedDocuments   []types.Document
}

type Status struct {
	Provider        string              `json:"provider"`
	IndexName       string              `json:"index_name"`
	DefaultLocale   string              `json:"default_locale"`
	CultureDataPath string              `json:"culture_data_path,omitempty"`
	Documents       int                 `json:"documents"`
	Capabilities    types.CapabilitySet `json:"capabilities"`
	Health          types.HealthStatus  `json:"health"`
}

type SearchRequest struct {
	Query           string `json:"query"`
	Locale          string `json:"locale"`
	AcceptLanguage  string `json:"accept_language,omitempty"`
	LocaleSource    string `json:"locale_source,omitempty"`
	LocaleSupported bool   `json:"locale_supported,omitempty"`
	Topic           string `json:"topic,omitempty"`
	Group           bool   `json:"group"`
	Page            int    `json:"page"`
	PerPage         int    `json:"per_page"`
}

type SuggestRequest struct {
	Query           string `json:"query"`
	Locale          string `json:"locale"`
	AcceptLanguage  string `json:"accept_language,omitempty"`
	LocaleSource    string `json:"locale_source,omitempty"`
	LocaleSupported bool   `json:"locale_supported,omitempty"`
	Limit           int    `json:"limit"`
}

func New(cfg Config) (*Runtime, error) {
	cfg = normalizeConfig(cfg)
	if cfg.Provider != "memory" {
		return nil, fmt.Errorf("bootstrap runtime only supports memory provider right now")
	}

	localeRuntime, err := locale.NewI18nRuntimeFromCultureData(cfg.CultureDataPath, cfg.DefaultLocale)
	if err != nil {
		return nil, err
	}

	registry := indexing.NewRegistry()
	provider := memory.New(memory.Config{})
	pln, err := planner.New(planner.Config{
		Registry:      registry,
		LocaleRuntime: localeRuntime,
		LocalePolicy: planner.LocalePolicy{
			MatchStrategy:   locale.MatchExactOrParent,
			Scope:           locale.ScopeActiveOnly,
			ExpandParents:   true,
			ExpandFallbacks: true,
			IncludeDefault:  true,
		},
	})
	if err != nil {
		return nil, err
	}
	ensureIndex, err := command.NewEnsureIndex(command.EnsureIndexConfig{
		Provider: provider,
		Registry: registry,
	})
	if err != nil {
		return nil, err
	}
	upsert, err := command.NewUpsertDocuments(command.UpsertDocumentsConfig{
		Provider: provider,
	})
	if err != nil {
		return nil, err
	}
	searchQuery, err := query.NewSearch(query.SearchConfig{
		Planner:  pln,
		Provider: provider,
	})
	if err != nil {
		return nil, err
	}
	suggestQuery, err := query.NewSuggest(query.SuggestConfig{
		Planner:  pln,
		Provider: provider,
	})
	if err != nil {
		return nil, err
	}
	healthQuery, err := query.NewHealth(query.HealthConfig{Provider: provider})
	if err != nil {
		return nil, err
	}

	index := types.IndexDefinition{
		Name:               cfg.IndexName,
		Label:              "Media transcripts",
		DefaultQueryFields: []string{"title", "summary", "body"},
		SearchableFields:   []string{"title", "summary", "body"},
		FacetFields:        []string{"topic", "locale"},
		FilterableFields:   []string{"topic", "locale"},
		HighlightFields:    []string{"body"},
		GroupByDefault:     "parent_id",
	}

	runtime := &Runtime{
		provider:        provider,
		registry:        registry,
		planner:         pln,
		localeRuntime:   localeRuntime,
		ensureIndex:     ensureIndex,
		upsert:          upsert,
		search:          searchQuery,
		suggest:         suggestQuery,
		health:          healthQuery,
		index:           index,
		cultureDataPath: cfg.CultureDataPath,
		defaultLocale:   cfg.DefaultLocale,
		seedDocuments:   seedDocuments(cfg.IndexName, cfg.DefaultLocale),
	}

	if err := runtime.ensureIndex.Execute(context.Background(), types.EnsureIndexInput{Definition: index}); err != nil {
		return nil, err
	}
	if cfg.SeedOnStart {
		if err := runtime.upsert.Execute(context.Background(), types.UpsertDocumentsInput{
			Index:     index.Name,
			Documents: runtime.seedDocuments,
		}); err != nil {
			return nil, err
		}
	}

	return runtime, nil
}

func (r *Runtime) ProviderName() string {
	if r == nil || r.provider == nil {
		return ""
	}
	return r.provider.Name()
}

func (r *Runtime) Status(ctx context.Context) (Status, error) {
	if r == nil || r.provider == nil {
		return Status{}, fmt.Errorf("search runtime is not initialized")
	}
	caps, err := r.provider.Capabilities(ctx)
	if err != nil {
		return Status{}, err
	}
	health, err := r.health.Query(ctx, types.HealthRequest{Indexes: []string{r.index.Name}})
	if err != nil {
		return Status{}, err
	}
	documents := 0
	for _, item := range health.Indexes {
		if item.Name == r.index.Name {
			documents = item.Documents
			break
		}
	}
	return Status{
		Provider:        r.provider.Name(),
		IndexName:       r.index.Name,
		DefaultLocale:   r.defaultLocale,
		CultureDataPath: r.cultureDataPath,
		Documents:       documents,
		Capabilities:    caps,
		Health:          health,
	}, nil
}

func (r *Runtime) BindSearchRequest(req SearchRequest) SearchRequest {
	bound := r.BindLocale(req.Locale, req.AcceptLanguage)
	req.Locale = firstNonEmpty(bound.Locale, req.Locale)
	req.LocaleSource = bound.Source
	req.LocaleSupported = bound.Supported
	return req
}

func (r *Runtime) BindSuggestRequest(req SuggestRequest) SuggestRequest {
	bound := r.BindLocale(req.Locale, req.AcceptLanguage)
	req.Locale = firstNonEmpty(bound.Locale, req.Locale)
	req.LocaleSource = bound.Source
	req.LocaleSupported = bound.Supported
	return req
}

func (r *Runtime) BindLocale(requestedLocale, acceptLanguage string) locale.BoundLocale {
	return locale.BindLocale(r.localeRuntime, requestedLocale, acceptLanguage, r.defaultLocale, locale.MatchOptions{
		Scope: locale.ScopeActiveOnly,
	})
}

func (r *Runtime) Search(ctx context.Context, req SearchRequest) (types.SearchResultPage, error) {
	if r == nil || r.search == nil {
		return types.SearchResultPage{}, fmt.Errorf("search runtime is not initialized")
	}
	req = r.BindSearchRequest(req)
	request := types.SearchRequest{
		Indexes: []string{r.index.Name},
		Query:   strings.TrimSpace(req.Query),
		Locale:  firstNonEmpty(strings.TrimSpace(req.Locale), r.defaultLocale),
		Page:    positiveOr(req.Page, 1),
		PerPage: positiveOr(req.PerPage, 10),
		Facets: []types.FacetRequest{
			{Field: "topic", Limit: 5},
			{Field: "locale", Limit: 5},
		},
	}
	if req.Group {
		request.GroupBy = "parent_id"
	}
	if topic := strings.TrimSpace(req.Topic); topic != "" {
		request.Filters = types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: topic}
	}
	return r.search.Query(ctx, request)
}

func (r *Runtime) Suggest(ctx context.Context, req SuggestRequest) (types.SuggestResult, error) {
	if r == nil || r.suggest == nil {
		return types.SuggestResult{}, fmt.Errorf("search runtime is not initialized")
	}
	req = r.BindSuggestRequest(req)
	return r.suggest.Query(ctx, types.SuggestRequest{
		Indexes: []string{r.index.Name},
		Query:   strings.TrimSpace(req.Query),
		Locale:  firstNonEmpty(strings.TrimSpace(req.Locale), r.defaultLocale),
		Limit:   positiveOr(req.Limit, 5),
	})
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = "memory"
	}
	if strings.TrimSpace(cfg.IndexName) == "" {
		cfg.IndexName = "media_transcripts"
	}
	if strings.TrimSpace(cfg.DefaultLocale) == "" {
		cfg.DefaultLocale = "en"
	}
	if strings.TrimSpace(cfg.CultureDataPath) == "" {
		cfg.CultureDataPath = defaultCultureDataPath()
	}
	return cfg
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultCultureDataPath() string {
	_, file, _, ok := runtimex.Caller(0)
	if !ok {
		return filepath.Join("..", "..", "..", "..", "testdata", "locale_search_culture.json")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "locale_search_culture.json")
}

func seedDocuments(indexName, locale string) []types.Document {
	start0 := int64(0)
	end25 := int64(25000)
	start25 := int64(25000)
	end50 := int64(50000)
	start60 := int64(60000)
	end90 := int64(90000)

	return []types.Document{
		{
			ID:         "media-1:segment-1:" + locale,
			Index:      indexName,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "media-1",
			SourceType: "transcript",
			SourceID:   "track-media-1-" + locale,
			Title:      "Search Blueprint Walkthrough",
			Summary:    "Transcript segment about grouped transcript evidence and playback anchors.",
			Body:       "This transcript explains how grouped transcript search keeps the parent media result stable while highlighting matching evidence windows.",
			URL:        "/media/search-blueprint",
			AnchorURL:  "/media/search-blueprint?t=0",
			Locale:     locale,
			StartMS:    &start0,
			EndMS:      &end25,
			Fields: map[string]any{
				"parent_title":     "Search Blueprint Walkthrough",
				"parent_url":       "/media/search-blueprint",
				"parent_thumbnail": "/static/search-blueprint.jpg",
				"topic":            "architecture",
			},
			Facets: map[string][]string{
				"topic":  []string{"architecture"},
				"locale": []string{locale},
			},
		},
		{
			ID:         "media-1:segment-2:" + locale,
			Index:      indexName,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "media-1",
			SourceType: "transcript",
			SourceID:   "track-media-1-" + locale,
			Title:      "Search Blueprint Walkthrough",
			Summary:    "Transcript segment about Typesense as the first production realism check.",
			Body:       "Typesense is the first production realism check, but the bootstrap example starts with the memory provider so the workflow stays deterministic.",
			URL:        "/media/search-blueprint",
			AnchorURL:  "/media/search-blueprint?t=25",
			Locale:     locale,
			StartMS:    &start25,
			EndMS:      &end50,
			Fields: map[string]any{
				"parent_title":     "Search Blueprint Walkthrough",
				"parent_url":       "/media/search-blueprint",
				"parent_thumbnail": "/static/search-blueprint.jpg",
				"topic":            "architecture",
			},
			Facets: map[string][]string{
				"topic":  []string{"architecture"},
				"locale": []string{locale},
			},
		},
		{
			ID:         "media-2:segment-1:" + locale,
			Index:      indexName,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "media-2",
			SourceType: "transcript",
			SourceID:   "track-media-2-" + locale,
			Title:      "Locale Planning Deep Dive",
			Summary:    "Transcript segment about exact locale matching before fallback.",
			Body:       "Locale planning should prefer the exact locale before broader fallback matches so multilingual search behavior remains deterministic and inspectable.",
			URL:        "/media/locale-planning",
			AnchorURL:  "/media/locale-planning?t=60",
			Locale:     locale,
			StartMS:    &start60,
			EndMS:      &end90,
			Fields: map[string]any{
				"parent_title":     "Locale Planning Deep Dive",
				"parent_url":       "/media/locale-planning",
				"parent_thumbnail": "/static/locale-planning.jpg",
				"topic":            "localization",
			},
			Facets: map[string][]string{
				"topic":  []string{"localization"},
				"locale": []string{locale},
			},
		},
	}
}
