package searchdemo

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	runtimex "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers"
	"github.com/goliatone/go-search/providers/memory"
	providertypesense "github.com/goliatone/go-search/providers/typesense"
	"github.com/goliatone/go-search/query"
)

type Config struct {
	Provider                  string
	SeedOnStart               bool
	IndexName                 string
	DefaultLocale             string
	CultureDataPath           string
	TypesenseServerURL        string
	TypesenseAPIKey           string
	TypesenseCollectionPrefix string
	ReindexBatchSize          int
	Logger                    *slog.Logger
}

type SearchQuerier interface {
	Query(context.Context, types.SearchRequest) (types.SearchResultPage, error)
}

type SuggestQuerier interface {
	Query(context.Context, types.SuggestRequest) (types.SuggestResult, error)
}

type HealthQuerier interface {
	Query(context.Context, types.HealthRequest) (types.HealthStatus, error)
}

type StatsQuerier interface {
	Query(context.Context, types.StatsRequest) (types.StatsResult, error)
}

type EnsureCommander interface {
	Execute(context.Context, types.EnsureIndexInput) error
}

type ReindexCommander interface {
	Execute(context.Context, types.ReindexIndexInput) error
}

type Runtime struct {
	provider         providers.Provider
	registry         *indexing.Registry
	planner          *planner.Planner
	localeRuntime    *locale.I18nRuntime
	generationStore  *memoryGenerationStore
	editorialStore   *memoryEditorialStore
	metrics          *runtimeMetricsHook
	activities       *memoryActivityHook
	logger           types.Logger
	ensureIndex      *command.EnsureIndex
	indexer          *indexing.Indexer
	reindex          *command.ReindexIndex
	search           *query.Search
	suggest          *query.Suggest
	health           *query.Health
	stats            *query.Stats
	editorialRules   *query.EditorialRules
	upsertRule       *command.UpsertEditorialRule
	deleteRule       *command.DeleteEditorialRule
	setRuleEnabled   *command.SetEditorialRuleEnabled
	index            types.IndexDefinition
	cultureDataPath  string
	defaultLocale    string
	reindexBatchSize int
	seedRecords      []media.TranscriptRecord
}

type Status struct {
	Provider         string                 `json:"provider"`
	IndexName        string                 `json:"index_name"`
	DefaultLocale    string                 `json:"default_locale"`
	CultureDataPath  string                 `json:"culture_data_path,omitempty"`
	Documents        int                    `json:"documents"`
	Generation       int64                  `json:"generation"`
	EditorialRules   int                    `json:"editorial_rules"`
	Capabilities     types.CapabilitySet    `json:"capabilities"`
	Health           types.HealthStatus     `json:"health"`
	Stats            types.StatsResult      `json:"stats"`
	Metrics          runtimeMetricsSnapshot `json:"metrics"`
	RecentActivities []types.ActivityEvent  `json:"recent_activities,omitempty"`
}

type SearchRequest struct {
	Query              string              `json:"query"`
	Locale             string              `json:"locale"`
	AcceptLanguage     string              `json:"accept_language,omitempty"`
	LocaleSource       string              `json:"locale_source,omitempty"`
	LocaleSupported    bool                `json:"locale_supported,omitempty"`
	Topic              string              `json:"topic,omitempty"`
	Topics             []string            `json:"topics,omitempty"`
	FacetFilters       map[string][]string `json:"facet_filters,omitempty"`
	LandingSlug        string              `json:"landing_slug,omitempty"`
	PublishedYearGTE   *int                `json:"published_year_gte,omitempty"`
	PublishedYearLTE   *int                `json:"published_year_lte,omitempty"`
	DurationSecondsGTE *int                `json:"duration_seconds_gte,omitempty"`
	DurationSecondsLTE *int                `json:"duration_seconds_lte,omitempty"`
	Group              bool                `json:"group"`
	Page               int                 `json:"page"`
	PerPage            int                 `json:"per_page"`
	SortField          string              `json:"sort_field,omitempty"`
	SortDir            string              `json:"sort_dir,omitempty"`
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

	localeRuntime, err := locale.NewI18nRuntimeFromCultureData(cfg.CultureDataPath, cfg.DefaultLocale)
	if err != nil {
		return nil, err
	}

	provider, err := newProvider(cfg)
	if err != nil {
		return nil, err
	}

	logger := newSlogSearchLogger(cfg.Logger)
	metrics := newRuntimeMetricsHook()
	activities := newMemoryActivityHook(32)
	generationStore := newMemoryGenerationStore()
	editorialStore := newMemoryEditorialStore()
	registry := indexing.NewRegistry()

	index := media.DefaultArchiveIndexDefinition(cfg.IndexName)
	source := media.NewTranscriptSource(seedTranscriptRecords(cfg.DefaultLocale))
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:      cfg.IndexName,
		SourceType: "transcript",
	})
	registration := indexing.NewRegistration(
		index.Name,
		index,
		"transcript",
		source,
		projector,
		func(record media.TranscriptRecord) string { return record.ID },
	)
	if err := registry.Register(index, registration); err != nil {
		return nil, err
	}

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
		Defaults: planner.Defaults{
			DisableIndexGroupByDefault: true,
		},
	})
	if err != nil {
		return nil, err
	}

	metricHooks := []types.MetricsHook{metrics}
	activityHooks := []types.ActivityHook{activities}

	ensureIndex, err := command.NewEnsureIndex(command.EnsureIndexConfig{
		Provider:   provider,
		Registry:   registry,
		Activities: activityHooks,
		Metrics:    metricHooks,
		Logger:     logger,
	})
	if err != nil {
		return nil, err
	}

	indexer, err := indexing.NewIndexer(indexing.IndexerConfig{
		Registry:        registry,
		Provider:        provider,
		GenerationStore: generationStore,
		Activities:      activityHooks,
		Metrics:         metricHooks,
		Logger:          logger,
	})
	if err != nil {
		return nil, err
	}

	reindexCmd, err := command.NewReindexIndex(command.ReindexIndexConfig{Indexer: indexer})
	if err != nil {
		return nil, err
	}

	searchQuery, err := query.NewSearch(query.SearchConfig{
		Planner:   pln,
		Provider:  provider,
		Editorial: editorialStore,
		Logger:    logger,
		Metrics:   metricHooks,
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

	statsQuery, err := query.NewStats(query.StatsConfig{
		Provider:        provider,
		Registry:        registry,
		GenerationStore: generationStore,
	})
	if err != nil {
		return nil, err
	}

	editorialRulesQuery, err := query.NewEditorialRules(query.EditorialRulesConfig{Store: editorialStore})
	if err != nil {
		return nil, err
	}

	upsertRuleCmd, err := command.NewUpsertEditorialRule(command.UpsertEditorialRuleConfig{
		Store:      editorialStore,
		Activities: activityHooks,
		Metrics:    metricHooks,
		Logger:     logger,
	})
	if err != nil {
		return nil, err
	}

	deleteRuleCmd, err := command.NewDeleteEditorialRule(command.DeleteEditorialRuleConfig{
		Store:      editorialStore,
		Activities: activityHooks,
		Metrics:    metricHooks,
		Logger:     logger,
	})
	if err != nil {
		return nil, err
	}

	setRuleEnabledCmd, err := command.NewSetEditorialRuleEnabled(command.SetEditorialRuleEnabledConfig{
		Store:      editorialStore,
		Activities: activityHooks,
		Metrics:    metricHooks,
		Logger:     logger,
	})
	if err != nil {
		return nil, err
	}

	runtime := &Runtime{
		provider:         provider,
		registry:         registry,
		planner:          pln,
		localeRuntime:    localeRuntime,
		generationStore:  generationStore,
		editorialStore:   editorialStore,
		metrics:          metrics,
		activities:       activities,
		logger:           logger,
		ensureIndex:      ensureIndex,
		indexer:          indexer,
		reindex:          reindexCmd,
		search:           searchQuery,
		suggest:          suggestQuery,
		health:           healthQuery,
		stats:            statsQuery,
		editorialRules:   editorialRulesQuery,
		upsertRule:       upsertRuleCmd,
		deleteRule:       deleteRuleCmd,
		setRuleEnabled:   setRuleEnabledCmd,
		index:            index,
		cultureDataPath:  cfg.CultureDataPath,
		defaultLocale:    cfg.DefaultLocale,
		reindexBatchSize: cfg.ReindexBatchSize,
		seedRecords:      seedTranscriptRecords(cfg.DefaultLocale),
	}

	if err := runtime.ensureIndex.Execute(context.Background(), types.EnsureIndexInput{Definition: index}); err != nil {
		return nil, err
	}
	if cfg.SeedOnStart {
		if err := runtime.Reindex(context.Background(), cfg.ReindexBatchSize); err != nil {
			return nil, err
		}
		if err := runtime.seedEditorialRules(context.Background()); err != nil {
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

func (r *Runtime) IndexName() string {
	if r == nil {
		return ""
	}
	return r.index.Name
}

func (r *Runtime) IndexDefinition() types.IndexDefinition {
	if r == nil {
		return types.IndexDefinition{}
	}
	return r.index
}

func (r *Runtime) SearchQuery() SearchQuerier   { return r.search }
func (r *Runtime) SuggestQuery() SuggestQuerier { return r.suggest }
func (r *Runtime) HealthQuery() HealthQuerier   { return r.health }
func (r *Runtime) StatsQuery() StatsQuerier     { return r.stats }
func (r *Runtime) EnsureCommand() EnsureCommander {
	return r.ensureIndex
}
func (r *Runtime) ReindexCommand() ReindexCommander {
	return r.reindex
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
	stats, err := r.stats.Query(ctx, types.StatsRequest{Indexes: []string{r.index.Name}})
	if err != nil {
		return Status{}, err
	}
	rules, err := r.editorialRules.Query(ctx, types.EditorialRuleListRequest{Indexes: []string{r.index.Name}})
	if err != nil {
		return Status{}, err
	}
	documents := 0
	generation := int64(0)
	for _, item := range health.Indexes {
		if item.Name == r.index.Name {
			documents = item.Documents
			break
		}
	}
	for _, item := range stats.Indexes {
		if item.Name == r.index.Name {
			generation = item.Generation
			break
		}
	}
	return Status{
		Provider:         r.provider.Name(),
		IndexName:        r.index.Name,
		DefaultLocale:    r.defaultLocale,
		CultureDataPath:  r.cultureDataPath,
		Documents:        documents,
		Generation:       generation,
		EditorialRules:   len(rules),
		Capabilities:     caps,
		Health:           health,
		Stats:            stats,
		Metrics:          r.metrics.Snapshot(),
		RecentActivities: r.activities.Snapshot(),
	}, nil
}

func (r *Runtime) Stats(ctx context.Context) (types.StatsResult, error) {
	if r == nil || r.stats == nil {
		return types.StatsResult{}, fmt.Errorf("search runtime is not initialized")
	}
	return r.stats.Query(ctx, types.StatsRequest{Indexes: []string{r.index.Name}})
}

func (r *Runtime) Ensure(ctx context.Context) error {
	if r == nil || r.ensureIndex == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	return r.ensureIndex.Execute(ctx, types.EnsureIndexInput{Definition: r.index})
}

func (r *Runtime) Reindex(ctx context.Context, batchSize int) error {
	if r == nil || r.reindex == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	if batchSize <= 0 {
		batchSize = r.reindexBatchSize
	}
	return r.reindex.Execute(ctx, types.ReindexIndexInput{
		Index:     r.index.Name,
		BatchSize: batchSize,
	})
}

func (r *Runtime) ListEditorialRules(ctx context.Context, enabled *bool) ([]types.EditorialRankRule, error) {
	if r == nil || r.editorialRules == nil {
		return nil, fmt.Errorf("search runtime is not initialized")
	}
	return r.editorialRules.Query(ctx, types.EditorialRuleListRequest{
		Indexes: []string{r.index.Name},
		Enabled: enabled,
	})
}

func (r *Runtime) UpsertEditorialRule(ctx context.Context, rule types.EditorialRankRule) error {
	if r == nil || r.upsertRule == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	if len(rule.Scope.Indexes) == 0 {
		rule.Scope.Indexes = []string{r.index.Name}
	}
	return r.upsertRule.Execute(ctx, types.UpsertEditorialRuleInput{Rule: rule})
}

func (r *Runtime) DeleteEditorialRule(ctx context.Context, id string) error {
	if r == nil || r.deleteRule == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	return r.deleteRule.Execute(ctx, types.DeleteEditorialRuleInput{ID: id})
}

func (r *Runtime) EnableEditorialRule(ctx context.Context, id string) error {
	if r == nil || r.setRuleEnabled == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	return r.setRuleEnabled.Execute(ctx, types.SetEditorialRuleEnabledInput{ID: id, Enabled: true})
}

func (r *Runtime) DisableEditorialRule(ctx context.Context, id string) error {
	if r == nil || r.setRuleEnabled == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	return r.setRuleEnabled.Execute(ctx, types.SetEditorialRuleEnabledInput{ID: id, Enabled: false})
}

func (r *Runtime) BindSearchRequest(req SearchRequest) SearchRequest {
	bound := locale.BindSearchRequest(r.localeRuntime, types.SearchRequest{
		Locale: req.Locale,
	}, req.AcceptLanguage, r.defaultLocale, locale.MatchOptions{
		Scope: locale.ScopeActiveOnly,
	})
	req.Locale = firstNonEmpty(bound.Request.Locale, req.Locale)
	req.LocaleSource = bound.Locale.Source
	req.LocaleSupported = bound.Locale.Supported
	return req
}

func (r *Runtime) BindSuggestRequest(req SuggestRequest) SuggestRequest {
	bound := locale.BindSuggestRequest(r.localeRuntime, types.SuggestRequest{
		Locale: req.Locale,
	}, req.AcceptLanguage, r.defaultLocale, locale.MatchOptions{
		Scope: locale.ScopeActiveOnly,
	})
	req.Locale = firstNonEmpty(bound.Request.Locale, req.Locale)
	req.LocaleSource = bound.Locale.Source
	req.LocaleSupported = bound.Locale.Supported
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
	sortField, sortDir := normalizeSearchSort(req.SortField, req.SortDir)
	request := types.SearchRequest{
		Indexes: []string{r.index.Name},
		Query:   strings.TrimSpace(req.Query),
		Locale:  firstNonEmpty(strings.TrimSpace(req.Locale), r.defaultLocale),
		Page:    positiveOr(req.Page, 1),
		PerPage: positiveOr(req.PerPage, 10),
		Facets:  media.DefaultArchiveFacetRequests(),
	}
	if req.Group {
		request.GroupBy = "parent_id"
	}

	// Handle sorting
	if sortField != "" {
		dir := types.SortAsc
		if sortDir == "desc" {
			dir = types.SortDesc
		}
		request.Sort = []types.Sort{{Field: sortField, Direction: dir}}
	}

	// Handle topic filtering (single or multiple)
	facetFilters := cloneFacetFilters(req.FacetFilters)
	topics := req.Topics
	if topic := strings.TrimSpace(req.Topic); topic != "" && len(topics) == 0 {
		topics = []string{topic}
	}
	if len(topics) > 0 {
		facetFilters[media.FacetFieldTopic] = append([]string(nil), topics...)
	}
	if slug := strings.TrimSpace(req.LandingSlug); slug != "" {
		if preset, ok := media.TopicLandingPreset(slug); ok {
			for field, values := range preset.FacetFilter {
				facetFilters[field] = append([]string(nil), values...)
			}
		}
	}
	request.Filters = searchFiltersExpr(
		facetFilters,
		req.PublishedYearGTE,
		req.PublishedYearLTE,
		req.DurationSecondsGTE,
		req.DurationSecondsLTE,
	)

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

func (r *Runtime) seedEditorialRules(ctx context.Context) error {
	position := 1
	rules := []types.EditorialRankRule{
		{
			ID:             "demo-pin-blueprint",
			TargetType:     types.DocumentTypeTranscriptSegment,
			ParentTargetID: "media-1",
			Action:         types.EditorialActionPin,
			Position:       &position,
			Enabled:        true,
			Scope: types.EditorialScope{
				Indexes: []string{r.index.Name},
				Query:   "blueprint",
				Locale:  r.defaultLocale,
			},
			Reason: "Keep the blueprint walkthrough first for blueprint-oriented queries.",
		},
		{
			ID:             "demo-hide-localization-disabled",
			TargetType:     types.DocumentTypeTranscriptSegment,
			ParentTargetID: "media-2",
			Action:         types.EditorialActionHide,
			Enabled:        false,
			Scope: types.EditorialScope{
				Indexes: []string{r.index.Name},
				Query:   "localization",
				Locale:  r.defaultLocale,
			},
			Reason: "Disabled demo rule for exercising hide toggles.",
		},
	}
	for _, rule := range rules {
		if err := r.UpsertEditorialRule(ctx, rule); err != nil {
			return err
		}
	}
	return nil
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
	if cfg.ReindexBatchSize <= 0 {
		cfg.ReindexBatchSize = 25
	}
	return cfg
}

func newProvider(cfg Config) (providers.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "memory":
		return memory.New(memory.Config{}), nil
	case "typesense":
		return providertypesense.New(providertypesense.Config{
			ServerURL:        strings.TrimSpace(cfg.TypesenseServerURL),
			APIKey:           strings.TrimSpace(cfg.TypesenseAPIKey),
			CollectionPrefix: strings.TrimSpace(cfg.TypesenseCollectionPrefix),
		})
	default:
		return nil, fmt.Errorf("unsupported search demo provider %q", cfg.Provider)
	}
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

func cloneFacetFilters(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(in))
	for field, values := range in {
		out[field] = append([]string(nil), values...)
	}
	return out
}

func searchFiltersExpr(filters map[string][]string, publishedYearGTE, publishedYearLTE, durationSecondsGTE, durationSecondsLTE *int) types.FilterExpr {
	terms := make([]types.FilterExpr, 0, len(filters))
	for field, values := range filters {
		values = compact(values)
		switch len(values) {
		case 0:
			continue
		case 1:
			terms = append(terms, types.TermExpr{Field: field, Op: types.FilterOpEQ, Value: values[0]})
		default:
			terms = append(terms, types.TermExpr{Field: field, Op: types.FilterOpIn, Value: values})
		}
	}
	if publishedYearGTE != nil || publishedYearLTE != nil {
		terms = append(terms, types.RangeExpr{
			Field: media.FieldPublishedYear,
			GTE:   optionalIntValue(publishedYearGTE),
			LTE:   optionalIntValue(publishedYearLTE),
		})
	}
	if durationSecondsGTE != nil || durationSecondsLTE != nil {
		terms = append(terms, types.RangeExpr{
			Field: media.FieldDurationSeconds,
			GTE:   optionalIntValue(durationSecondsGTE),
			LTE:   optionalIntValue(durationSecondsLTE),
		})
	}
	return collapseFilterTerms(terms)
}

func compact(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func collapseFilterTerms(terms []types.FilterExpr) types.FilterExpr {
	switch len(terms) {
	case 0:
		return nil
	case 1:
		return terms[0]
	default:
		return types.AndExpr{Terms: terms}
	}
}

func optionalIntValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func defaultCultureDataPath() string {
	_, file, _, ok := runtimex.Caller(0)
	if !ok {
		return filepath.Join("..", "..", "..", "..", "testdata", "locale_search_culture.json")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "locale_search_culture.json")
}

func seedTranscriptRecords(locale string) []media.TranscriptRecord {
	alternateLocale := "es"
	if strings.EqualFold(locale, alternateLocale) {
		alternateLocale = "en"
	}
	return []media.TranscriptRecord{
		// Media 1: Search Blueprint (architecture topic)
		{
			ID: "track-media-1-en-part-1",
			Media: media.MediaRecord{
				ID:        "media-1",
				Title:     "Search Blueprint Walkthrough",
				Summary:   "Comprehensive guide to building search interfaces with grouped transcript ranking.",
				URL:       "/media/search-blueprint",
				Thumbnail: "/static/search-blueprint.jpg",
				Topic:     "architecture",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-1",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:25.000
This transcript explains how grouped transcript search keeps the parent media result stable while highlighting matching evidence windows.
`),
		},
		{
			ID: "track-media-1-en-part-2",
			Media: media.MediaRecord{
				ID:        "media-1",
				Title:     "Search Blueprint Walkthrough",
				Summary:   "Memory and Typesense provider parity in the search shell.",
				URL:       "/media/search-blueprint",
				Thumbnail: "/static/search-blueprint.jpg",
				Topic:     "architecture",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-1",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:25.000 --> 00:00:50.000
Typesense is the first production realism check, but the shell keeps the memory provider path available so ranking behavior remains deterministic during local development.
`),
		},
		{
			ID: "track-media-1-en-part-3",
			Media: media.MediaRecord{
				ID:        "media-1",
				Title:     "Search Blueprint Walkthrough",
				Summary:   "Faceted navigation and filter expressions.",
				URL:       "/media/search-blueprint",
				Thumbnail: "/static/search-blueprint.jpg",
				Topic:     "architecture",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-1",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:50.000 --> 00:01:15.000
Faceted navigation allows users to refine search results by selecting filter values. The system supports AND, OR, and NOT expressions for complex filtering scenarios.
`),
		},

		// Media 2: Locale Planning (localization topic)
		{
			ID: "track-media-2-en-part-1",
			Media: media.MediaRecord{
				ID:        "media-2",
				Title:     "Locale Planning Deep Dive",
				Summary:   "Understanding locale matching strategies for multilingual search.",
				URL:       "/media/locale-planning",
				Thumbnail: "/static/locale-planning.jpg",
				Topic:     "localization",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-2",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:30.000
Locale planning should prefer the exact locale before broader fallback matches so multilingual search behavior remains deterministic and inspectable.
`),
		},
		{
			ID: "track-media-2-en-part-2",
			Media: media.MediaRecord{
				ID:        "media-2",
				Title:     "Locale Planning Deep Dive",
				Summary:   "Culture data and internationalization runtime configuration.",
				URL:       "/media/locale-planning",
				Thumbnail: "/static/locale-planning.jpg",
				Topic:     "localization",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-2",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:30.000 --> 00:01:00.000
Culture data defines active locales, fallback chains, and display names. The I18N runtime loads this configuration to power locale-aware search queries.
`),
		},

		// Media 3: Editorial Rules (ranking topic)
		{
			ID: "track-media-3-en-part-1",
			Media: media.MediaRecord{
				ID:        "media-3",
				Title:     "Editorial Ranking Rules",
				Summary:   "How to boost, bury, pin, and hide search results using editorial rules.",
				URL:       "/media/editorial-rules",
				Thumbnail: "/static/editorial-rules.jpg",
				Topic:     "ranking",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-3",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:25.000
Editorial rules allow content curators to manually influence search rankings. You can boost important results, bury less relevant ones, pin items to specific positions, or hide them entirely.
`),
		},
		{
			ID: "track-media-3-en-part-2",
			Media: media.MediaRecord{
				ID:        "media-3",
				Title:     "Editorial Ranking Rules",
				Summary:   "Scoping rules by query, locale, and time window.",
				URL:       "/media/editorial-rules",
				Thumbnail: "/static/editorial-rules.jpg",
				Topic:     "ranking",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-3",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:25.000 --> 00:00:50.000
Rules can be scoped by query terms, locale, topic filters, and even time windows. This allows seasonal promotions or locale-specific curations without affecting global search behavior.
`),
		},

		// Media 4: Indexing Pipeline (indexing topic)
		{
			ID: "track-media-4-en-part-1",
			Media: media.MediaRecord{
				ID:        "media-4",
				Title:     "Building the Indexing Pipeline",
				Summary:   "Source projectors and document transformation for search indexing.",
				URL:       "/media/indexing-pipeline",
				Thumbnail: "/static/indexing-pipeline.jpg",
				Topic:     "indexing",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-4",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:30.000
The indexing pipeline transforms source records into searchable documents. Projectors map your domain models to the document schema, handling field extraction, facet generation, and metadata enrichment.
`),
		},
		{
			ID: "track-media-4-en-part-2",
			Media: media.MediaRecord{
				ID:        "media-4",
				Title:     "Building the Indexing Pipeline",
				Summary:   "Batch processing and incremental reindexing strategies.",
				URL:       "/media/indexing-pipeline",
				Thumbnail: "/static/indexing-pipeline.jpg",
				Topic:     "indexing",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-4",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:30.000 --> 00:01:00.000
Batch processing improves throughput for full reindex operations. Incremental updates use generation tracking to efficiently sync changes without rebuilding the entire index.
`),
		},

		// Media 5: Autocomplete (ui topic)
		{
			ID: "track-media-5-en-part-1",
			Media: media.MediaRecord{
				ID:        "media-5",
				Title:     "Implementing Autocomplete",
				Summary:   "Real-time search suggestions and typeahead functionality.",
				URL:       "/media/autocomplete",
				Thumbnail: "/static/autocomplete.jpg",
				Topic:     "ui",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-5",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:25.000
Autocomplete suggestions help users discover content as they type. The suggest API returns matching titles with locale-aware filtering and relevance scoring.
`),
		},
		{
			ID: "track-media-5-en-part-2",
			Media: media.MediaRecord{
				ID:        "media-5",
				Title:     "Implementing Autocomplete",
				Summary:   "Debouncing requests and handling suggestion selection.",
				URL:       "/media/autocomplete",
				Thumbnail: "/static/autocomplete.jpg",
				Topic:     "ui",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-5",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:25.000 --> 00:00:50.000
Debouncing prevents excessive API calls while users type. When a suggestion is selected, the search form submits with the full title for precise matching.
`),
		},

		// Media 6: Semantic Search (semantic topic)
		{
			ID: "track-media-6-en-part-1",
			Media: media.MediaRecord{
				ID:        "media-6",
				Title:     "Introduction to Semantic Search",
				Summary:   "Beyond keyword matching with vector embeddings.",
				URL:       "/media/semantic-search",
				Thumbnail: "/static/semantic-search.jpg",
				Topic:     "semantic",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-6",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:30.000
Semantic search uses vector embeddings to find conceptually similar content, even when exact keywords don't match. This enables more natural language queries and improved recall.
`),
		},
		{
			ID: "track-media-6-en-part-2",
			Media: media.MediaRecord{
				ID:        "media-6",
				Title:     "Introduction to Semantic Search",
				Summary:   "Hybrid search combining lexical and semantic signals.",
				URL:       "/media/semantic-search",
				Thumbnail: "/static/semantic-search.jpg",
				Topic:     "semantic",
				Locale:    locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-6",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:30.000 --> 00:01:00.000
Hybrid search combines the precision of lexical matching with the understanding of semantic similarity. This gives users the best of both worlds for finding relevant content.
`),
		},

		// Media 7: Architecture Case Studies (architecture topic, different archive facets)
		{
			ID: "track-media-7-en-part-1",
			Media: media.MediaRecord{
				ID:              "media-7",
				Title:           "Architecture Case Studies",
				Summary:         "Applying grouped search architecture to real archive workflows.",
				URL:             "/media/architecture-case-studies",
				Thumbnail:       "/static/architecture-case-studies.jpg",
				Topic:           "architecture",
				People:          []string{"Archive Research Team"},
				Subjects:        []string{"Architecture Review"},
				Texts:           []string{"Field Notes"},
				Location:        "Mexico City",
				Sangha:          "Field Research Sangha",
				Format:          "Workshop",
				Series:          "Search Case Studies",
				DurationSeconds: 2100,
				PublishedAt:     seededPublishedAt(2025, time.February, 14),
				Badge:           "Case Study",
				Locale:          locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-7",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:24.000
Architecture reviews compare grouped search flows across archive surfaces so teams can verify that series, format, and location filters narrow parent results in visible ways.
`),
		},
		{
			ID: "track-media-7-en-part-2",
			Media: media.MediaRecord{
				ID:              "media-7",
				Title:           "Architecture Case Studies",
				Summary:         "Testing grouped result filters against realistic archive metadata.",
				URL:             "/media/architecture-case-studies",
				Thumbnail:       "/static/architecture-case-studies.jpg",
				Topic:           "architecture",
				People:          []string{"Archive Research Team"},
				Subjects:        []string{"Architecture Review"},
				Texts:           []string{"Field Notes"},
				Location:        "Mexico City",
				Sangha:          "Field Research Sangha",
				Format:          "Workshop",
				Series:          "Search Case Studies",
				DurationSeconds: 2100,
				PublishedAt:     seededPublishedAt(2025, time.February, 14),
				Badge:           "Case Study",
				Locale:          locale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-7",
				Locale:       locale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: locale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:24.000 --> 00:00:48.000
This case study keeps the architecture topic but changes the archive facets, making it easier to prove that grouped search filters affect some parent results without affecting all of them.
`),
		},

		// Media 8: Arquitectura de Busqueda (architecture topic, alternate locale)
		{
			ID: "track-media-8-alt-part-1",
			Media: media.MediaRecord{
				ID:              "media-8",
				Title:           "Arquitectura de Busqueda",
				Summary:         "Version localizada del recorrido de arquitectura de busqueda.",
				URL:             "/media/arquitectura-busqueda",
				Thumbnail:       "/static/arquitectura-busqueda.jpg",
				Topic:           "architecture",
				People:          []string{"Equipo de Arquitectura"},
				Subjects:        []string{"Arquitectura de Busqueda"},
				Texts:           []string{"Plano de Busqueda"},
				Location:        "Bogota",
				Sangha:          "Sangha de Traduccion",
				Format:          "Seminar",
				Series:          "Busqueda Global",
				DurationSeconds: 1950,
				PublishedAt:     seededPublishedAt(2024, time.September, 3),
				Badge:           "Espanol",
				Locale:          alternateLocale,
			},
			Track: types.TranscriptTrack{
				MediaID:      "media-8",
				Locale:       alternateLocale,
				SourceFormat: "vtt",
				TrackKind:    "captions",
				SourceLocale: alternateLocale,
			},
			Format: "vtt",
			Content: strings.TrimSpace(`
WEBVTT

00:00:00.000 --> 00:00:26.000
La arquitectura de busqueda agrupa segmentos por recurso padre para que los filtros de locale, formato y serie sigan siendo faciles de inspeccionar.
`),
		},
	}
}

func seededPublishedAt(year int, month time.Month, day int) *time.Time {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &value
}

type memoryGenerationStore struct {
	mu    sync.RWMutex
	items map[string]int64
}

func newMemoryGenerationStore() *memoryGenerationStore {
	return &memoryGenerationStore{items: map[string]int64{}}
}

func (s *memoryGenerationStore) Get(_ context.Context, index string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items[index], nil
}

func (s *memoryGenerationStore) Bump(_ context.Context, index string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[index]++
	return s.items[index], nil
}

type memoryEditorialStore struct {
	mu    sync.RWMutex
	items map[string]types.EditorialRankRule
}

func newMemoryEditorialStore() *memoryEditorialStore {
	return &memoryEditorialStore{items: map[string]types.EditorialRankRule{}}
}

func (s *memoryEditorialStore) Upsert(_ context.Context, rule types.EditorialRankRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[rule.ID] = cloneEditorialRule(rule)
	return nil
}

func (s *memoryEditorialStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	return nil
}

func (s *memoryEditorialStore) SetEnabled(_ context.Context, id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rule, ok := s.items[id]
	if !ok {
		return nil
	}
	rule.Enabled = enabled
	s.items[id] = rule
	return nil
}

func (s *memoryEditorialStore) List(_ context.Context, req types.EditorialRuleListRequest) ([]types.EditorialRankRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.EditorialRankRule, 0, len(s.items))
	for _, rule := range s.items {
		if req.Enabled != nil && rule.Enabled != *req.Enabled {
			continue
		}
		if len(req.Indexes) == 1 && len(rule.Scope.Indexes) > 0 && !containsString(rule.Scope.Indexes, req.Indexes[0]) {
			continue
		}
		if req.Locale != "" && rule.Scope.Locale != "" && !strings.EqualFold(rule.Scope.Locale, req.Locale) {
			continue
		}
		out = append(out, cloneEditorialRule(rule))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *memoryEditorialStore) ListApplicable(_ context.Context, req types.SearchRequest) ([]types.EditorialRankRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.EditorialRankRule, 0, len(s.items))
	for _, rule := range s.items {
		if !rule.Enabled {
			continue
		}
		if len(req.Indexes) == 1 && len(rule.Scope.Indexes) > 0 && !containsString(rule.Scope.Indexes, req.Indexes[0]) {
			continue
		}
		if rule.Scope.TenantID != "" && rule.Scope.TenantID != req.Scope.TenantID {
			continue
		}
		if rule.Scope.OrgID != "" && rule.Scope.OrgID != req.Scope.OrgID {
			continue
		}
		if rule.Scope.Locale != "" && !strings.EqualFold(rule.Scope.Locale, req.Locale) {
			continue
		}
		if rule.Scope.RankingProfile != "" && !strings.EqualFold(rule.Scope.RankingProfile, req.RankingProfile) {
			continue
		}
		out = append(out, cloneEditorialRule(rule))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

type runtimeMetricsSnapshot struct {
	Observed map[string]float64 `json:"observed,omitempty"`
	Counts   map[string]int64   `json:"counts,omitempty"`
}

type runtimeMetricsHook struct {
	mu       sync.RWMutex
	observed map[string]float64
	counts   map[string]int64
}

func newRuntimeMetricsHook() *runtimeMetricsHook {
	return &runtimeMetricsHook{
		observed: map[string]float64{},
		counts:   map[string]int64{},
	}
}

func (h *runtimeMetricsHook) Observe(_ context.Context, metric string, value float64, _ map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observed[metric] = value
}

func (h *runtimeMetricsHook) Count(_ context.Context, metric string, delta int64, _ map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.counts[metric] += delta
}

func (h *runtimeMetricsHook) Snapshot() runtimeMetricsSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return runtimeMetricsSnapshot{
		Observed: cloneFloatMap(h.observed),
		Counts:   cloneInt64Map(h.counts),
	}
}

type memoryActivityHook struct {
	mu     sync.RWMutex
	limit  int
	events []types.ActivityEvent
}

func newMemoryActivityHook(limit int) *memoryActivityHook {
	if limit <= 0 {
		limit = 16
	}
	return &memoryActivityHook{limit: limit}
}

func (h *memoryActivityHook) Notify(_ context.Context, event types.ActivityEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	event.Metadata = cloneAnyMap(event.Metadata)
	h.events = append(h.events, event)
	if len(h.events) > h.limit {
		h.events = append([]types.ActivityEvent(nil), h.events[len(h.events)-h.limit:]...)
	}
}

func (h *memoryActivityHook) Snapshot() []types.ActivityEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]types.ActivityEvent, 0, len(h.events))
	for _, event := range h.events {
		copy := event
		copy.Metadata = cloneAnyMap(event.Metadata)
		out = append(out, copy)
	}
	return out
}

type slogSearchLogger struct {
	logger *slog.Logger
}

func newSlogSearchLogger(logger *slog.Logger) types.Logger {
	if logger == nil {
		return nil
	}
	return &slogSearchLogger{logger: logger}
}

func (l *slogSearchLogger) Debug(msg string, metadata map[string]any) {
	l.log(slog.LevelDebug, msg, metadata)
}
func (l *slogSearchLogger) Info(msg string, metadata map[string]any) {
	l.log(slog.LevelInfo, msg, metadata)
}
func (l *slogSearchLogger) Warn(msg string, metadata map[string]any) {
	l.log(slog.LevelWarn, msg, metadata)
}
func (l *slogSearchLogger) Error(msg string, metadata map[string]any) {
	l.log(slog.LevelError, msg, metadata)
}

func (l *slogSearchLogger) log(level slog.Level, msg string, metadata map[string]any) {
	if l == nil || l.logger == nil {
		return
	}
	attrs := make([]any, 0, len(metadata)*2)
	for key, value := range metadata {
		attrs = append(attrs, key, value)
	}
	l.logger.Log(context.Background(), level, msg, attrs...)
}

func cloneEditorialRule(rule types.EditorialRankRule) types.EditorialRankRule {
	out := rule
	out.Scope.Indexes = append([]string(nil), rule.Scope.Indexes...)
	if len(rule.Scope.Filters) > 0 {
		out.Scope.Filters = make(map[string][]string, len(rule.Scope.Filters))
		for key, values := range rule.Scope.Filters {
			out.Scope.Filters[key] = append([]string(nil), values...)
		}
	}
	out.Metadata = cloneAnyMap(rule.Metadata)
	return out
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneInt64Map(in map[string]int64) map[string]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func normalizeSearchSort(field, dir string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "title", media.FieldPublishedYear, media.FieldDurationSeconds:
	default:
		return "", ""
	}
	field = strings.ToLower(strings.TrimSpace(field))
	if strings.EqualFold(strings.TrimSpace(dir), "desc") {
		return field, "desc"
	}
	return field, "asc"
}
