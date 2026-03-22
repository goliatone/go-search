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

	"github.com/goliatone/go-search/adapters/content"
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
	mediaIndex       types.IndexDefinition
	contentShared    types.IndexDefinition
	contentVideo     types.IndexDefinition
	contentDocument  types.IndexDefinition
	contentBlog      types.IndexDefinition
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
	Surface            string              `json:"surface,omitempty"`
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
	Surface         string `json:"surface,omitempty"`
	Locale          string `json:"locale"`
	AcceptLanguage  string `json:"accept_language,omitempty"`
	LocaleSource    string `json:"locale_source,omitempty"`
	LocaleSupported bool   `json:"locale_supported,omitempty"`
	Limit           int    `json:"limit"`
}

const (
	SurfaceMediaGrouped  = "media_grouped"
	SurfaceContentShared = "content_shared"
	SurfaceContentSplit  = "content_split"
)

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

	mediaIndex := media.DefaultArchiveIndexDefinition(cfg.IndexName)
	sharedContentIndex := content.DefaultIndexDefinition(contentSharedIndexName(cfg.IndexName))
	videoContentIndex := content.DefaultIndexDefinition(contentVideoIndexName(cfg.IndexName))
	documentContentIndex := content.DefaultIndexDefinition(contentDocumentIndexName(cfg.IndexName))
	blogContentIndex := content.DefaultIndexDefinition(contentBlogIndexName(cfg.IndexName))

	transcriptRecords := seedTranscriptRecords(cfg.DefaultLocale)
	videoRecords, documentRecords, blogRecords := seedContentRecords(cfg.DefaultLocale)

	source := media.NewTranscriptSource(transcriptRecords)
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:      cfg.IndexName,
		SourceType: "transcript",
	})
	registration := indexing.NewRegistration(
		mediaIndex.Name,
		mediaIndex,
		"transcript",
		source,
		projector,
		func(record media.TranscriptRecord) string { return record.ID },
	)
	if err := registry.Register(mediaIndex, registration); err != nil {
		return nil, err
	}

	sharedVideoRegistration := indexing.NewRegistrationWithKey(
		sharedContentIndex.Name,
		sharedContentIndex,
		"video",
		"video",
		content.NewSource(videoRecords),
		content.NewProjector(content.ProjectorConfig{Index: sharedContentIndex.Name, SourceType: "video"}),
		func(record content.Record) string { return record.ID },
	)
	if err := registry.Register(sharedContentIndex, sharedVideoRegistration); err != nil {
		return nil, err
	}
	sharedDocumentRegistration := indexing.NewRegistrationWithKey(
		sharedContentIndex.Name,
		sharedContentIndex,
		"document",
		"document",
		content.NewSource(documentRecords),
		content.NewProjector(content.ProjectorConfig{Index: sharedContentIndex.Name, SourceType: "document"}),
		func(record content.Record) string { return record.ID },
	)
	if err := registry.Register(sharedContentIndex, sharedDocumentRegistration); err != nil {
		return nil, err
	}
	sharedBlogRegistration := indexing.NewRegistrationWithKey(
		sharedContentIndex.Name,
		sharedContentIndex,
		"blog_article",
		"blog_article",
		content.NewSource(blogRecords),
		content.NewProjector(content.ProjectorConfig{Index: sharedContentIndex.Name, SourceType: "blog_article"}),
		func(record content.Record) string { return record.ID },
	)
	if err := registry.Register(sharedContentIndex, sharedBlogRegistration); err != nil {
		return nil, err
	}

	if err := registry.Register(videoContentIndex, indexing.NewRegistration(
		videoContentIndex.Name,
		videoContentIndex,
		"video",
		content.NewSource(videoRecords),
		content.NewProjector(content.ProjectorConfig{Index: videoContentIndex.Name, SourceType: "video"}),
		func(record content.Record) string { return record.ID },
	)); err != nil {
		return nil, err
	}
	if err := registry.Register(documentContentIndex, indexing.NewRegistration(
		documentContentIndex.Name,
		documentContentIndex,
		"document",
		content.NewSource(documentRecords),
		content.NewProjector(content.ProjectorConfig{Index: documentContentIndex.Name, SourceType: "document"}),
		func(record content.Record) string { return record.ID },
	)); err != nil {
		return nil, err
	}
	if err := registry.Register(blogContentIndex, indexing.NewRegistration(
		blogContentIndex.Name,
		blogContentIndex,
		"blog_article",
		content.NewSource(blogRecords),
		content.NewProjector(content.ProjectorConfig{Index: blogContentIndex.Name, SourceType: "blog_article"}),
		func(record content.Record) string { return record.ID },
	)); err != nil {
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
		mediaIndex:       mediaIndex,
		contentShared:    sharedContentIndex,
		contentVideo:     videoContentIndex,
		contentDocument:  documentContentIndex,
		contentBlog:      blogContentIndex,
		cultureDataPath:  cfg.CultureDataPath,
		defaultLocale:    cfg.DefaultLocale,
		reindexBatchSize: cfg.ReindexBatchSize,
		seedRecords:      transcriptRecords,
	}

	if err := runtime.Ensure(context.Background()); err != nil {
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
	return r.contentShared.Name
}

func (r *Runtime) IndexDefinition() types.IndexDefinition {
	if r == nil {
		return types.IndexDefinition{}
	}
	return r.contentShared
}

func (r *Runtime) IndexNames() []string {
	if r == nil {
		return nil
	}
	return []string{
		r.mediaIndex.Name,
		r.contentShared.Name,
		r.contentVideo.Name,
		r.contentDocument.Name,
		r.contentBlog.Name,
	}
}

func (r *Runtime) SurfaceIndexes(surface string) []string {
	if r == nil {
		return nil
	}
	switch normalizeSurface(surface, false) {
	case SurfaceMediaGrouped:
		return []string{r.mediaIndex.Name}
	case SurfaceContentSplit:
		return []string{r.contentVideo.Name, r.contentDocument.Name, r.contentBlog.Name}
	default:
		return []string{r.contentShared.Name}
	}
}

func (r *Runtime) DefaultSurface() string {
	return SurfaceContentShared
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
	indexes := r.IndexNames()
	health, err := r.health.Query(ctx, types.HealthRequest{Indexes: indexes})
	if err != nil {
		return Status{}, err
	}
	stats, err := r.stats.Query(ctx, types.StatsRequest{Indexes: indexes})
	if err != nil {
		return Status{}, err
	}
	rules, err := r.editorialRules.Query(ctx, types.EditorialRuleListRequest{Indexes: []string{r.mediaIndex.Name}})
	if err != nil {
		return Status{}, err
	}
	documents := 0
	generation := int64(0)
	for _, item := range health.Indexes {
		documents += item.Documents
	}
	for _, item := range stats.Indexes {
		if item.Name == r.contentShared.Name {
			generation = item.Generation
			break
		}
	}
	return Status{
		Provider:         r.provider.Name(),
		IndexName:        r.contentShared.Name,
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
	return r.stats.Query(ctx, types.StatsRequest{Indexes: r.IndexNames()})
}

func (r *Runtime) Ensure(ctx context.Context) error {
	if r == nil || r.ensureIndex == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	for _, def := range []types.IndexDefinition{r.mediaIndex, r.contentShared, r.contentVideo, r.contentDocument, r.contentBlog} {
		if err := r.ensureIndex.Execute(ctx, types.EnsureIndexInput{Definition: def}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) Reindex(ctx context.Context, batchSize int) error {
	if r == nil || r.reindex == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	if batchSize <= 0 {
		batchSize = r.reindexBatchSize
	}
	for _, indexName := range r.IndexNames() {
		if err := r.reindex.Execute(ctx, types.ReindexIndexInput{
			Index:     indexName,
			BatchSize: batchSize,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) ListEditorialRules(ctx context.Context, enabled *bool) ([]types.EditorialRankRule, error) {
	if r == nil || r.editorialRules == nil {
		return nil, fmt.Errorf("search runtime is not initialized")
	}
	return r.editorialRules.Query(ctx, types.EditorialRuleListRequest{
		Indexes: []string{r.mediaIndex.Name},
		Enabled: enabled,
	})
}

func (r *Runtime) UpsertEditorialRule(ctx context.Context, rule types.EditorialRankRule) error {
	if r == nil || r.upsertRule == nil {
		return fmt.Errorf("search runtime is not initialized")
	}
	if len(rule.Scope.Indexes) == 0 {
		rule.Scope.Indexes = []string{r.mediaIndex.Name}
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
	req.Surface = normalizeSurface(req.Surface, req.Group)
	req.Group = req.Surface == SurfaceMediaGrouped
	sortField, sortDir := normalizeSearchSort(req.SortField, req.SortDir)
	request := types.SearchRequest{
		Indexes: r.SurfaceIndexes(req.Surface),
		Query:   strings.TrimSpace(req.Query),
		Locale:  firstNonEmpty(strings.TrimSpace(req.Locale), r.defaultLocale),
		Page:    positiveOr(req.Page, 1),
		PerPage: positiveOr(req.PerPage, 10),
		Facets:  surfaceFacetRequests(req.Surface),
	}
	if req.Surface == SurfaceMediaGrouped {
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
	req.Surface = normalizeSurface(req.Surface, false)
	return r.suggest.Query(ctx, types.SuggestRequest{
		Indexes: r.SurfaceIndexes(req.Surface),
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
				Indexes: []string{r.mediaIndex.Name},
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
				Indexes: []string{r.mediaIndex.Name},
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

func normalizeSurface(surface string, grouped bool) string {
	switch strings.TrimSpace(surface) {
	case SurfaceMediaGrouped, SurfaceContentShared, SurfaceContentSplit:
		return strings.TrimSpace(surface)
	}
	if grouped {
		return SurfaceMediaGrouped
	}
	return SurfaceContentShared
}

func surfaceFacetRequests(surface string) []types.FacetRequest {
	requests := media.DefaultArchiveFacetRequests()
	if normalizeSurface(surface, false) == SurfaceMediaGrouped {
		return requests
	}
	return append([]types.FacetRequest{{Field: "entity_type", Limit: 10, Disjunctive: true}}, requests...)
}

func contentSharedIndexName(mediaIndex string) string {
	return strings.TrimSpace(mediaIndex) + "_content_shared"
}
func contentVideoIndexName(mediaIndex string) string {
	return strings.TrimSpace(mediaIndex) + "_videos"
}
func contentDocumentIndexName(mediaIndex string) string {
	return strings.TrimSpace(mediaIndex) + "_documents"
}
func contentBlogIndexName(mediaIndex string) string {
	return strings.TrimSpace(mediaIndex) + "_blog_articles"
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

func compactStrings(values ...string) []string {
	return compact(values)
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

type seededMediaFixture struct {
	Media       media.MediaRecord
	TrackLocale string
	Parts       []string
}

func seedTranscriptRecords(locale string) []media.TranscriptRecord {
	return buildSeedTranscriptRecords(seedMediaFixtures(locale))
}

func seedContentRecords(locale string) ([]content.Record, []content.Record, []content.Record) {
	fixtures := seedMediaFixtures(locale)
	videoRecords := []content.Record{
		buildWholeEntityRecord(types.DocumentTypeVideo, "video", fixtures[0].Media, "Whole-item archive view for blueprint architecture search and archive facet exploration."),
		buildWholeEntityRecord(types.DocumentTypeVideo, "video", fixtures[1].Media, "Whole-item locale planning view covering catalog normalization, locale fallback rules, and multilingual site search."),
		buildWholeEntityRecord(types.DocumentTypeVideo, "video", fixtures[6].Media, "Whole-item case study showing how architecture searches narrow by location, series, and workshop format."),
		buildWholeEntityRecord(types.DocumentTypeVideo, "video", fixtures[8].Media, "Whole-item Tara practice result carrying deity, practice, and long-duration filters."),
	}

	documents := []content.Record{
		buildWholeEntityRecord(types.DocumentTypeDocument, "document", media.MediaRecord{
			ID:              "document-1",
			Title:           "Blueprint Faceting Checklist",
			Summary:         "Operational checklist for validating architecture facets and mixed-content search surfaces.",
			URL:             "/documents/blueprint-faceting-checklist",
			Topic:           "architecture",
			TopicPath:       []string{"Teaching Topics", "Architecture"},
			CategoryPath:    []string{"Teaching Categories", "Operations", "Documentation"},
			People:          []string{"Maya Lin", "Archive Research Team"},
			Subjects:        []string{"Facet Validation"},
			Texts:           []string{"Search Blueprint"},
			Location:        "Boulder",
			Sangha:          "Archive Engineering",
			Format:          "Guide",
			Series:          "Search Foundations",
			DurationSeconds: 900,
			PublishedAt:     seededPublishedAt(2025, time.January, 20),
			Badge:           "Checklist",
			Locale:          locale,
		}, "Checklist for validating grouped media search separately from flat video, document, and blog surfaces."),
		buildWholeEntityRecord(types.DocumentTypeDocument, "document", media.MediaRecord{
			ID:              "document-2",
			Title:           "Localization Rollout Workbook",
			Summary:         "Planning workbook for locale-aware search rollout and fallback inspection.",
			URL:             "/documents/localization-rollout-workbook",
			Topic:           "localization",
			TopicPath:       []string{"Teaching Topics", "Localization"},
			CategoryPath:    []string{"Teaching Categories", "Operations", "Documentation"},
			People:          []string{"Elena Ruiz"},
			Subjects:        []string{"Locale Fallback"},
			Texts:           []string{"Culture Catalog"},
			Location:        "Madrid",
			Sangha:          "Translation Sangha",
			Format:          "Workbook",
			Series:          "Locale Planning Lab",
			DurationSeconds: 1200,
			PublishedAt:     seededPublishedAt(2024, time.September, 12),
			Badge:           "Workbook",
			Locale:          locale,
		}, "Workbook for auditing locale filters, fallback ordering, and exact-locale ranking across site-search content."),
		buildWholeEntityRecord(types.DocumentTypeDocument, "document", media.MediaRecord{
			ID:              "document-3",
			Title:           "Catalog Cleanup Runbook",
			Summary:         "Maintenance guide for metadata cleanup and zero-drift index refreshes.",
			URL:             "/documents/catalog-cleanup-runbook",
			Topic:           "indexing",
			TopicPath:       []string{"Teaching Topics", "Indexing"},
			CategoryPath:    []string{"Teaching Categories", "Operations", "Maintenance"},
			People:          []string{"Omar Patel"},
			Subjects:        []string{"Metadata Maintenance"},
			Texts:           []string{"Catalog Checklist"},
			Location:        "Chicago",
			Sangha:          "Archive Engineering",
			Format:          "Runbook",
			Series:          "Search Operations",
			DurationSeconds: 600,
			PublishedAt:     seededPublishedAt(2023, time.May, 3),
			Badge:           "Runbook",
			Locale:          locale,
		}, "Runbook for document projection validation, delete-by-source checks, and reindex safety."),
		buildWholeEntityRecord(types.DocumentTypeDocument, "document", media.MediaRecord{
			ID:              "document-4",
			Title:           "Tara Retreat Handout",
			Summary:         "Handout connecting Tara practice metadata to search filters and archive labels.",
			URL:             "/documents/tara-retreat-handout",
			Topic:           "tara",
			TopicPath:       []string{"Teaching Topics", "Tara"},
			CategoryPath:    []string{"Teaching Categories", "Practice", "Handout"},
			People:          []string{"Tenzin Rocha"},
			Subjects:        []string{"Tara Practice"},
			Texts:           []string{"Praise to the 21 Taras"},
			Deities:         []string{"Tara"},
			Location:        "Santa Fe",
			Sangha:          "Practice Sangha",
			Format:          "Handout",
			Series:          "Bodhisattva Cycle",
			DurationSeconds: 1500,
			PublishedAt:     seededPublishedAt(2019, time.October, 1),
			Badge:           "Practice",
			Locale:          locale,
		}, "Practice handout for validating deity facets, retreat-style durations, and mixed-content result rendering."),
	}

	alternateLocale := "es"
	if strings.EqualFold(locale, alternateLocale) {
		alternateLocale = "en"
	}
	blogs := []content.Record{
		buildWholeEntityRecord(types.DocumentTypeBlogArticle, "blog_article", media.MediaRecord{
			ID:              "blog-1",
			Title:           "Why Mixed-Entity Search Needs Explicit Modes",
			Summary:         "Notes on keeping grouped transcript retrieval separate from site-wide content search.",
			URL:             "/blog/mixed-entity-search-modes",
			Topic:           "architecture",
			TopicPath:       []string{"Teaching Topics", "Architecture"},
			CategoryPath:    []string{"Teaching Categories", "Commentary", "Design Notes"},
			People:          []string{"Jon Alvarez"},
			Subjects:        []string{"Search Modes"},
			Texts:           []string{"Search Blueprint"},
			Location:        "Boulder",
			Sangha:          "Archive Engineering",
			Format:          "Blog",
			Series:          "Search Notes",
			DurationSeconds: 420,
			PublishedAt:     seededPublishedAt(2025, time.March, 1),
			Badge:           "Blog",
			Locale:          locale,
		}, "Grouped transcript search is valuable, but whole-entity site search should stay flat when indexes mix videos, documents, and articles."),
		buildWholeEntityRecord(types.DocumentTypeBlogArticle, "blog_article", media.MediaRecord{
			ID:              "blog-2",
			Title:           "Locale Fallbacks for Site Search",
			Summary:         "Field notes from rolling out locale-aware content search across admin and demo surfaces.",
			URL:             "/blog/locale-fallbacks-site-search",
			Topic:           "localization",
			TopicPath:       []string{"Teaching Topics", "Localization"},
			CategoryPath:    []string{"Teaching Categories", "Commentary", "Field Notes"},
			People:          []string{"Elena Ruiz", "Martin Cole"},
			Subjects:        []string{"Locale Policy"},
			Texts:           []string{"Culture Catalog"},
			Location:        "Mexico City",
			Sangha:          "Translation Sangha",
			Format:          "Blog",
			Series:          "Search Notes",
			DurationSeconds: 360,
			PublishedAt:     seededPublishedAt(2024, time.June, 18),
			Badge:           "Locale",
			Locale:          alternateLocale,
		}, "This article compares exact locale matches and fallback content when the same site search surface spans multiple entity types."),
		buildWholeEntityRecord(types.DocumentTypeBlogArticle, "blog_article", media.MediaRecord{
			ID:              "blog-3",
			Title:           "Catalog Cleanup in Short Bursts",
			Summary:         "Short operational notes for quick metadata maintenance sessions.",
			URL:             "/blog/catalog-cleanup-short-bursts",
			Topic:           "indexing",
			TopicPath:       []string{"Teaching Topics", "Indexing"},
			CategoryPath:    []string{"Teaching Categories", "Operations", "Notes"},
			People:          []string{"Omar Patel"},
			Subjects:        []string{"Quick Maintenance"},
			Texts:           []string{"Catalog Checklist"},
			Location:        "Chicago",
			Sangha:          "Archive Engineering",
			Format:          "Blog",
			Series:          "Search Operations",
			DurationSeconds: 180,
			PublishedAt:     seededPublishedAt(2022, time.February, 8),
			Badge:           "Ops",
			Locale:          locale,
		}, "Short-form operational content keeps low-duration filters populated outside the media archive."),
		buildWholeEntityRecord(types.DocumentTypeBlogArticle, "blog_article", media.MediaRecord{
			ID:              "blog-4",
			Title:           "Practice Metadata and Search Filters",
			Summary:         "How deity and practice labels should behave in whole-entity search.",
			URL:             "/blog/practice-metadata-search-filters",
			Topic:           "tara",
			TopicPath:       []string{"Teaching Topics", "Tara"},
			CategoryPath:    []string{"Teaching Categories", "Practice", "Commentary"},
			People:          []string{"Tenzin Rocha", "Maya Lin"},
			Subjects:        []string{"Practice Metadata"},
			Texts:           []string{"Practice Notes"},
			Deities:         []string{"Tara"},
			Location:        "Santa Fe",
			Sangha:          "Practice Sangha",
			Format:          "Blog",
			Series:          "Bodhisattva Cycle",
			DurationSeconds: 300,
			PublishedAt:     seededPublishedAt(2021, time.December, 6),
			Badge:           "Practice",
			Locale:          locale,
		}, "Practice-oriented article metadata should facet cleanly without being mistaken for anchored transcript evidence."),
	}
	return videoRecords, documents, blogs
}

func seedMediaFixtures(locale string) []seededMediaFixture {
	alternateLocale := "es"
	if strings.EqualFold(locale, alternateLocale) {
		alternateLocale = "en"
	}
	return []seededMediaFixture{
		{
			Media: media.MediaRecord{
				ID:              "media-1",
				Title:           "Search Blueprint Walkthrough",
				Summary:         "A practical architecture walkthrough for grouped transcript search and archive facets.",
				URL:             "/media/search-blueprint",
				Thumbnail:       "/static/search-blueprint.jpg",
				Topic:           "architecture",
				TopicPath:       []string{"Teaching Topics", "Architecture"},
				CategoryPath:    []string{"Teaching Categories", "Commentary", "Systems Design"},
				People:          []string{"Maya Lin", "Jon Alvarez"},
				Subjects:        []string{"Search Architecture", "Provider Strategy"},
				Texts:           []string{"Search Blueprint", "Typesense Notes"},
				Location:        "Boulder",
				Sangha:          "Archive Engineering",
				Format:          "Teaching",
				Series:          "Search Foundations",
				DurationSeconds: 1680,
				PublishedAt:     seededPublishedAt(2024, time.June, 11),
				Badge:           "Blueprint",
				Locale:          locale,
			},
			Parts: []string{
				"This transcript introduces the search blueprint and explains why grouped transcript search keeps parent media stable while filters remain visible.",
				"We compare memory and Typesense behavior so provider parity and transcript ranking can be checked without changing the archive contract.",
				"The walkthrough closes by mapping topic, people, series, and duration filters onto a realistic archive search screen.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-2",
				Title:           "Locale Planning Deep Dive",
				Summary:         "Locale policy, active catalogs, and fallback chains for multilingual search.",
				URL:             "/media/locale-planning",
				Thumbnail:       "/static/locale-planning.jpg",
				Topic:           "localization",
				TopicPath:       []string{"Teaching Topics", "Localization"},
				CategoryPath:    []string{"Teaching Categories", "Commentary", "Internationalization"},
				People:          []string{"Elena Ruiz", "Martin Cole"},
				Subjects:        []string{"Locale Planning", "Accept Language"},
				Texts:           []string{"Locale Search Matrix", "Culture Catalog"},
				Location:        "Madrid",
				Sangha:          "Translation Sangha",
				Format:          "Teaching",
				Series:          "Locale Planning Lab",
				DurationSeconds: 2520,
				PublishedAt:     seededPublishedAt(2023, time.March, 19),
				Badge:           "Locale",
				Locale:          locale,
			},
			Parts: []string{
				"Locale planning should prefer the exact locale before broader fallback matches so multilingual search behavior stays deterministic and inspectable.",
				"Accept-Language headers, scoped catalogs, and fallback chains need to resolve before query planning so providers only see normalized locale filters.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-3",
				Title:           "Editorial Ranking Rules",
				Summary:         "Operational ranking controls for pinning, boosting, burying, and hiding results.",
				URL:             "/media/editorial-rules",
				Thumbnail:       "/static/editorial-rules.jpg",
				Topic:           "ranking",
				TopicPath:       []string{"Teaching Topics", "Ranking"},
				CategoryPath:    []string{"Teaching Categories", "Operations", "Editorial Strategy"},
				People:          []string{"Priya Nair", "Jonah Lee"},
				Subjects:        []string{"Editorial Ranking", "Search Curation"},
				Texts:           []string{"Ranking Playbook"},
				Location:        "New York",
				Sangha:          "Editorial Sangha",
				Format:          "Workshop",
				Series:          "Search Operations",
				DurationSeconds: 3360,
				PublishedAt:     seededPublishedAt(2022, time.October, 5),
				Badge:           "Editorial",
				Locale:          locale,
			},
			Parts: []string{
				"Editorial rules allow curators to pin or hide results while keeping the canonical ranking model explicit and testable.",
				"Rule scope can combine query text, locale, topic, and time windows so promotions stay targeted instead of leaking across the entire archive.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-4",
				Title:           "Building the Indexing Pipeline",
				Summary:         "Projectors, document lineage, and reliable reindex flows for search ingestion.",
				URL:             "/media/indexing-pipeline",
				Thumbnail:       "/static/indexing-pipeline.jpg",
				Topic:           "indexing",
				TopicPath:       []string{"Teaching Topics", "Indexing"},
				CategoryPath:    []string{"Teaching Categories", "Commentary", "Pipeline Design"},
				People:          []string{"Sarah Chen", "Omar Patel"},
				Subjects:        []string{"Document Projection", "Reindex Strategy"},
				Texts:           []string{"Indexer Registry", "Generation Ledger"},
				Location:        "Portland",
				Sangha:          "Systems Sangha",
				Format:          "Seminar",
				Series:          "Search Foundations",
				DurationSeconds: 2760,
				PublishedAt:     seededPublishedAt(2021, time.April, 22),
				Badge:           "Indexing",
				Locale:          locale,
			},
			Parts: []string{
				"The indexing pipeline transforms transcript source records into searchable documents with stable parent metadata and facet fields.",
				"Generation tracking and replay-safe IDs keep incremental reindex operations predictable even when source records arrive out of order.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-5",
				Title:           "Implementing Autocomplete",
				Summary:         "Typeahead behavior, suggestion ranking, and search-box UX decisions.",
				URL:             "/media/autocomplete",
				Thumbnail:       "/static/autocomplete.jpg",
				Topic:           "ui",
				TopicPath:       []string{"Teaching Topics", "UI"},
				CategoryPath:    []string{"Teaching Categories", "Tools", "Interface Patterns"},
				People:          []string{"Lia Park", "Felix Turner"},
				Subjects:        []string{"Autocomplete", "Query Suggestions"},
				Texts:           []string{"Suggest API Guide"},
				Location:        "Online",
				Sangha:          "Product Sangha",
				Format:          "Demo",
				Series:          "Search UX",
				DurationSeconds: 780,
				PublishedAt:     seededPublishedAt(2019, time.August, 14),
				Badge:           "UX",
				Locale:          locale,
			},
			Parts: []string{
				"Autocomplete suggestions help users discover search content as they type while keeping locale-aware suggestion scoring fast.",
				"Debouncing, keyboard navigation, and suggestion selection all shape whether the search box feels trustworthy under real traffic.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-6",
				Title:           "Introduction to Semantic Search",
				Summary:         "Vector retrieval, embeddings, and hybrid ranking for broader recall.",
				URL:             "/media/semantic-search",
				Thumbnail:       "/static/semantic-search.jpg",
				Topic:           "semantic",
				TopicPath:       []string{"Teaching Topics", "Semantic"},
				CategoryPath:    []string{"Teaching Categories", "Commentary", "Retrieval Design"},
				People:          []string{"Nia Brooks", "Dev Shah"},
				Subjects:        []string{"Semantic Search", "Hybrid Retrieval"},
				Texts:           []string{"Embedding Primer", "Search Foundations"},
				Location:        "San Francisco",
				Sangha:          "ML Sangha",
				Format:          "Seminar",
				Series:          "Search Foundations",
				DurationSeconds: 4140,
				PublishedAt:     seededPublishedAt(2024, time.January, 9),
				Badge:           "Semantic",
				Locale:          locale,
			},
			Parts: []string{
				"Semantic search uses vector embeddings to find conceptually related material even when the literal search terms do not overlap.",
				"Hybrid retrieval mixes lexical evidence with semantic similarity so archive search can trade precision and recall in a controlled way.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-7",
				Title:           "Architecture Case Studies",
				Summary:         "Applying grouped search architecture to a realistic archive workflow.",
				URL:             "/media/architecture-case-studies",
				Thumbnail:       "/static/architecture-case-studies.jpg",
				Topic:           "architecture",
				TopicPath:       []string{"Teaching Topics", "Architecture"},
				CategoryPath:    []string{"Teaching Categories", "Workshop", "Case Studies"},
				People:          []string{"Maya Lin", "Archive Research Team"},
				Subjects:        []string{"Architecture Review", "Archive Workflow"},
				Texts:           []string{"Field Notes", "Retrieval Worksheet"},
				Location:        "Mexico City",
				Sangha:          "Field Research Sangha",
				Format:          "Workshop",
				Series:          "Search Case Studies",
				DurationSeconds: 2100,
				PublishedAt:     seededPublishedAt(2025, time.February, 14),
				Badge:           "Case Study",
				Locale:          locale,
			},
			Parts: []string{
				"Architecture reviews compare grouped search flows across archive surfaces so teams can see how series, format, and location filters narrow parent results.",
				"This case study keeps the architecture topic but changes the archive metadata, making cross-field filter combinations easy to verify in grouped results.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-9",
				Title:           "Tara Practice for Search Teams",
				Summary:         "A practice-oriented archive item that exercises deity, topic, and retreat-style filters.",
				URL:             "/media/tara-practice-search-teams",
				Thumbnail:       "/static/tara-practice-search-teams.jpg",
				Topic:           "tara",
				TopicPath:       []string{"Teaching Topics", "Tara"},
				CategoryPath:    []string{"Teaching Categories", "Practice", "Guided Session"},
				People:          []string{"Tenzin Rocha", "Maya Lin"},
				Subjects:        []string{"Tara Practice", "Team Grounding"},
				Texts:           []string{"Praise to the 21 Taras", "Practice Notes"},
				Deities:         []string{"Tara"},
				Location:        "Santa Fe",
				Sangha:          "Practice Sangha",
				Format:          "Retreat Talk",
				Series:          "Bodhisattva Cycle",
				DurationSeconds: 5400,
				PublishedAt:     seededPublishedAt(2018, time.November, 2),
				Badge:           "Practice",
				Locale:          locale,
			},
			Parts: []string{
				"The Tara session opens with a short grounding practice so search teams can reset before working through difficult catalog decisions.",
				"Because this item carries Tara deity metadata, practice category labels, and a long retreat duration, it gives the archive filters something realistic to narrow.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-10",
				Title:           "Catalog Cleanup Clinic",
				Summary:         "A short operational item for metadata maintenance and zero-drift indexing hygiene.",
				URL:             "/media/catalog-cleanup-clinic",
				Thumbnail:       "/static/catalog-cleanup-clinic.jpg",
				Topic:           "indexing",
				TopicPath:       []string{"Teaching Topics", "Indexing"},
				CategoryPath:    []string{"Teaching Categories", "Operations", "Maintenance"},
				People:          []string{"Omar Patel"},
				Subjects:        []string{"Metadata Maintenance"},
				Texts:           []string{"Catalog Checklist"},
				Location:        "Chicago",
				Sangha:          "Archive Engineering",
				Format:          "Office Hour",
				Series:          "Search Operations",
				DurationSeconds: 240,
				PublishedAt:     seededPublishedAt(2016, time.February, 1),
				Badge:           "Clinic",
				Locale:          locale,
			},
			Parts: []string{
				"This clinic is intentionally short so duration bucket filters can separate quick maintenance sessions from deeper archive search trainings.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-11",
				Title:           "Archive Search Office Hours",
				Summary:         "Practical Q&A on triaging search regressions and filter confusion.",
				URL:             "/media/archive-search-office-hours",
				Thumbnail:       "/static/archive-search-office-hours.jpg",
				Topic:           "ui",
				TopicPath:       []string{"Teaching Topics", "UI"},
				CategoryPath:    []string{"Teaching Categories", "Workshop", "Q&A"},
				People:          []string{"Maya Lin", "Felix Turner"},
				Subjects:        []string{"Search Triage"},
				Texts:           []string{"Operator Handbook"},
				Location:        "Boulder",
				Sangha:          "Product Sangha",
				Format:          "Workshop",
				Series:          "Search Operations",
				DurationSeconds: 1320,
				PublishedAt:     seededPublishedAt(2017, time.July, 7),
				Badge:           "Q&A",
				Locale:          locale,
			},
			Parts: []string{
				"Office hours collect search questions from operators who need to debug ranking, pagination, and archive filter combinations without reading code first.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-8",
				Title:           "Arquitectura de Busqueda",
				Summary:         "Version localizada del recorrido de arquitectura de busqueda para el catalogo.",
				URL:             "/media/arquitectura-busqueda",
				Thumbnail:       "/static/arquitectura-busqueda.jpg",
				Topic:           "architecture",
				TopicPath:       []string{"Teaching Topics", "Architecture"},
				CategoryPath:    []string{"Teaching Categories", "Commentary", "Systems Design"},
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
			TrackLocale: alternateLocale,
			Parts: []string{
				"La arquitectura de busqueda agrupa segmentos por recurso padre para que los filtros de locale, formato y serie sigan siendo faciles de inspeccionar.",
			},
		},
		{
			Media: media.MediaRecord{
				ID:              "media-12",
				Title:           "Planificacion de Locale",
				Summary:         "Resumen localizado de catalogos activos y cadenas de fallback para busqueda.",
				URL:             "/media/planificacion-locale",
				Thumbnail:       "/static/planificacion-locale.jpg",
				Topic:           "localization",
				TopicPath:       []string{"Teaching Topics", "Localization"},
				CategoryPath:    []string{"Teaching Categories", "Commentary", "Internationalization"},
				People:          []string{"Elena Ruiz"},
				Subjects:        []string{"Politica de Locale"},
				Texts:           []string{"Matriz de Idiomas"},
				Location:        "Mexico City",
				Sangha:          "Sangha de Traduccion",
				Format:          "Teaching",
				Series:          "Busqueda Global",
				DurationSeconds: 2280,
				PublishedAt:     seededPublishedAt(2022, time.May, 16),
				Badge:           "Espanol",
				Locale:          alternateLocale,
			},
			TrackLocale: alternateLocale,
			Parts: []string{
				"Los catalogos activos, los encabezados Accept-Language y las cadenas de fallback deben resolverse antes del planificador para mantener filtros de locale consistentes.",
			},
		},
	}
}

func buildSeedTranscriptRecords(fixtures []seededMediaFixture) []media.TranscriptRecord {
	total := 0
	for _, fixture := range fixtures {
		total += len(fixture.Parts)
	}
	out := make([]media.TranscriptRecord, 0, total)
	for _, fixture := range fixtures {
		trackLocale := firstNonEmpty(strings.TrimSpace(fixture.TrackLocale), strings.TrimSpace(fixture.Media.Locale))
		for idx, body := range fixture.Parts {
			out = append(out, media.TranscriptRecord{
				ID:    seedTranscriptRecordID(fixture.Media.ID, trackLocale, idx+1),
				Media: fixture.Media,
				Track: types.TranscriptTrack{
					MediaID:      fixture.Media.ID,
					Locale:       trackLocale,
					SourceFormat: "vtt",
					TrackKind:    "captions",
					SourceLocale: trackLocale,
				},
				Format:  "vtt",
				Content: seedTranscriptContent(idx*28, idx*28+24, body),
			})
		}
	}
	return out
}

func buildWholeEntityRecord(docType, sourceType string, item media.MediaRecord, body string) content.Record {
	localeValue := strings.TrimSpace(item.Locale)
	projection := media.BuildArchiveProjection(item, localeValue)
	fields := map[string]any{
		"topic":              projection.TopicLeaf,
		"topic_hierarchy":    append([]string(nil), projection.TopicHierarchy...),
		"category":           projection.CategoryLeaf,
		"category_hierarchy": append([]string(nil), projection.CategoryHierarchy...),
		"people":             append([]string(nil), projection.People...),
		"subject":            append([]string(nil), projection.Subjects...),
		"text":               append([]string(nil), projection.Texts...),
		"deity":              append([]string(nil), projection.Deities...),
		"location":           projection.Location,
		"sangha":             projection.Sangha,
		"format":             projection.Format,
		"series":             projection.Series,
		"decade":             projection.Decade,
		"duration_bucket":    projection.DurationBucket,
		"published_year":     projection.PublishedYear,
		"result_badge":       projection.Badge,
	}
	facets := map[string][]string{
		"topic":              compactStrings(projection.TopicLeaf),
		"topic_hierarchy":    append([]string(nil), projection.TopicHierarchy...),
		"category":           compactStrings(projection.CategoryLeaf),
		"category_hierarchy": append([]string(nil), projection.CategoryHierarchy...),
		"people":             append([]string(nil), projection.People...),
		"subject":            append([]string(nil), projection.Subjects...),
		"text":               append([]string(nil), projection.Texts...),
		"deity":              append([]string(nil), projection.Deities...),
		"locale":             compactStrings(localeValue),
		"decade":             compactStrings(projection.Decade),
		"duration_bucket":    compactStrings(projection.DurationBucket),
		"location":           compactStrings(projection.Location),
		"sangha":             compactStrings(projection.Sangha),
		"format":             compactStrings(projection.Format),
		"series":             compactStrings(projection.Series),
	}
	numeric := map[string]float64{
		"published_year":   float64(projection.PublishedYear),
		"duration_seconds": float64(projection.DurationSeconds),
	}
	metadata := map[string]any{
		"published_at":   item.PublishedAt,
		"published_year": projection.PublishedYear,
	}
	return content.Record{
		ID:         item.ID,
		Type:       docType,
		SourceType: sourceType,
		SourceID:   item.ID,
		Title:      item.Title,
		Summary:    item.Summary,
		Body:       strings.TrimSpace(body),
		URL:        item.URL,
		Locale:     localeValue,
		Fields:     fields,
		Facets:     facets,
		Numeric:    numeric,
		Metadata:   metadata,
	}
}

func seedTranscriptRecordID(mediaID, trackLocale string, part int) string {
	trackLocale = strings.NewReplacer("-", "_", " ", "_").Replace(strings.ToLower(strings.TrimSpace(trackLocale)))
	return fmt.Sprintf("track-%s-%s-part-%d", mediaID, trackLocale, part)
}

func seedTranscriptContent(startSeconds, endSeconds int, body string) string {
	return strings.TrimSpace(fmt.Sprintf(`
WEBVTT

%s --> %s
%s
`, seedTimestamp(startSeconds), seedTimestamp(endSeconds), strings.TrimSpace(body)))
}

func seedTimestamp(seconds int) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d.000", hours, minutes, secs)
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
