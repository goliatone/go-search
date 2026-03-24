package searchdemo

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	runtimex "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	cms "github.com/goliatone/go-cms"
	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	cmslifecycle "github.com/goliatone/go-cms/pkg/lifecycle"
	"github.com/goliatone/go-search/adapters/content"
	cmsgosearch "github.com/goliatone/go-search/adapters/gocms"
	usersgosearch "github.com/goliatone/go-search/adapters/gousers"
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/cache"
	"github.com/goliatone/go-search/command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers"
	"github.com/goliatone/go-search/providers/memory"
	providerpostgres "github.com/goliatone/go-search/providers/postgres"
	providertypesense "github.com/goliatone/go-search/providers/typesense"
	"github.com/goliatone/go-search/query"
	generationbunstore "github.com/goliatone/go-search/stores/generation/bun"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

type Config struct {
	Provider                  string
	CacheEnabled              bool
	SeedOnStart               bool
	IndexName                 string
	DefaultLocale             string
	CultureDataPath           string
	PostgresDSN               string
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
	provider          providers.Provider
	registry          *indexing.Registry
	planner           *planner.Planner
	localeRuntime     *locale.I18nRuntime
	generationStore   types.GenerationStore
	generationBackend string
	editorialStore    *memoryEditorialStore
	metrics           *runtimeMetricsHook
	activities        *memoryActivityHook
	cacheEnabled      bool
	cacheWrappers     cacheWrapperStatus
	cacheStores       *runtimeCacheStores
	logger            types.Logger
	ensureIndex       *command.EnsureIndex
	indexer           *indexing.Indexer
	reindex           *command.ReindexIndex
	search            SearchQuerier
	suggest           SuggestQuerier
	health            *query.Health
	stats             *query.Stats
	editorialRules    *query.EditorialRules
	upsertRule        *command.UpsertEditorialRule
	deleteRule        *command.DeleteEditorialRule
	setRuleEnabled    *command.SetEditorialRuleEnabled
	mediaIndex        types.IndexDefinition
	contentShared     types.IndexDefinition
	contentPage       types.IndexDefinition
	contentVideo      types.IndexDefinition
	contentDocument   types.IndexDefinition
	contentBlog       types.IndexDefinition
	usersIndex        types.IndexDefinition
	cmsModule         *cms.Module
	cmsFixture        *cmsFixture
	userStore         *demoUserStore
	cultureDataPath   string
	defaultLocale     string
	reindexBatchSize  int
	seedRecords       []media.TranscriptRecord
	closeProvider     func() error
}

type cmsFixture struct {
	actorID                uuid.UUID
	templateID             uuid.UUID
	pageContentID          uuid.UUID
	pageID                 uuid.UUID
	documentID             uuid.UUID
	blogID                 uuid.UUID
	defaultLocale          string
	secondaryLocale        string
	pageQuery              string
	documentQuery          string
	blogQuery              string
	blogUpdatedESQuery     string
	documentDeletedESQuery string
}

type Status struct {
	Provider          string                 `json:"provider"`
	IndexName         string                 `json:"index_name"`
	DefaultLocale     string                 `json:"default_locale"`
	CultureDataPath   string                 `json:"culture_data_path,omitempty"`
	GenerationBackend string                 `json:"generation_backend"`
	CacheEnabled      bool                   `json:"cache_enabled"`
	CacheWrappers     cacheWrapperStatus     `json:"cache_wrappers"`
	Cache             runtimeCacheSnapshot   `json:"cache"`
	SmokeFlows        []SmokeFlow            `json:"smoke_flows,omitempty"`
	Documents         int                    `json:"documents"`
	Generation        int64                  `json:"generation"`
	EditorialRules    int                    `json:"editorial_rules"`
	Capabilities      types.CapabilitySet    `json:"capabilities"`
	Health            types.HealthStatus     `json:"health"`
	Stats             types.StatsResult      `json:"stats"`
	Metrics           runtimeMetricsSnapshot `json:"metrics"`
	RecentActivities  []types.ActivityEvent  `json:"recent_activities,omitempty"`
}

type SearchRequest struct {
	Query              string              `json:"query"`
	Surface            string              `json:"surface,omitempty"`
	Locale             string              `json:"locale"`
	CacheDisabled      bool                `json:"cache_disabled,omitempty"`
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
	TenantID           string              `json:"tenant_id,omitempty"`
	OrgID              string              `json:"org_id,omitempty"`
	ActorUserID        string              `json:"actor_user_id,omitempty"`
	ActorRole          string              `json:"actor_role,omitempty"`
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
	CacheDisabled   bool   `json:"cache_disabled,omitempty"`
	AcceptLanguage  string `json:"accept_language,omitempty"`
	LocaleSource    string `json:"locale_source,omitempty"`
	LocaleSupported bool   `json:"locale_supported,omitempty"`
	TenantID        string `json:"tenant_id,omitempty"`
	OrgID           string `json:"org_id,omitempty"`
	ActorUserID     string `json:"actor_user_id,omitempty"`
	ActorRole       string `json:"actor_role,omitempty"`
	Limit           int    `json:"limit"`
}

const (
	SurfaceMediaGrouped  = "media_grouped"
	SurfaceContentShared = "content_shared"
	SurfaceContentSplit  = "content_split"
	SurfaceUsers         = "users"

	generationBackendMemory      = "memory"
	generationBackendBunPostgres = "bun_postgres"
)

type cacheWrapperStatus struct {
	Search           bool `json:"search"`
	Suggest          bool `json:"suggest"`
	ProviderMetadata bool `json:"provider_metadata"`
}

type SmokeFlow struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type providerBootstrap struct {
	provider          providers.Provider
	generationStore   types.GenerationStore
	generationBackend string
	closeProvider     func() error
}

func New(cfg Config) (*Runtime, error) {
	cfg = normalizeConfig(cfg)

	localeRuntime, err := locale.NewI18nRuntimeFromCultureData(cfg.CultureDataPath, cfg.DefaultLocale)
	if err != nil {
		return nil, err
	}

	bootstrap, err := newProviderBootstrap(cfg)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil && bootstrap.closeProvider != nil {
			_ = bootstrap.closeProvider()
		}
	}()

	logger := newSlogSearchLogger(cfg.Logger)
	metrics := newRuntimeMetricsHook()
	activities := newMemoryActivityHook(32)
	generationStore := bootstrap.generationStore
	editorialStore := newMemoryEditorialStore()
	provider := bootstrap.provider
	cacheStores := &runtimeCacheStores{}
	cacheWrappers := cacheWrapperStatus{}
	if cfg.CacheEnabled {
		cacheStores.capabilities = newRuntimeTTLStore[types.CapabilitySet]()
		cacheStores.health = newRuntimeTTLStore[types.HealthStatus]()
		wrappedProvider, wrapErr := cache.NewCachedProviderMetadata(cache.CachedProviderMetadataConfig{
			Provider:        provider,
			CapabilityCache: cacheStores.capabilities,
			HealthCache:     cacheStores.health,
			Logger:          logger,
			Metrics:         []types.MetricsHook{metrics},
		})
		if wrapErr != nil {
			return nil, wrapErr
		}
		provider = wrappedProvider
		cacheWrappers.ProviderMetadata = true
	}
	registry := indexing.NewRegistry()

	mediaIndex := media.DefaultArchiveIndexDefinition(cfg.IndexName)
	sharedContentIndex := content.DefaultIndexDefinition(contentSharedIndexName(cfg.IndexName))
	pageContentIndex := content.DefaultIndexDefinition(contentPageIndexName(cfg.IndexName))
	videoContentIndex := content.DefaultIndexDefinition(contentVideoIndexName(cfg.IndexName))
	documentContentIndex := content.DefaultIndexDefinition(contentDocumentIndexName(cfg.IndexName))
	blogContentIndex := content.DefaultIndexDefinition(contentBlogIndexName(cfg.IndexName))
	usersIndex := userIndexDefinition(cfg.IndexName)
	userStore := newDemoUserStore(cfg.DefaultLocale)

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

	userLoader, err := usersgosearch.NewRepositoryLoader(usersgosearch.RepositoryLoaderConfig{
		Users:     userStore,
		Inventory: userStore,
		Profiles:  userStore,
		Actor: userstypes.ActorRef{
			ID:   demoUserIndexerActorID(),
			Type: "system",
		},
		ResolveScope: demoUserScopeResolver,
		Logger:       logger,
	})
	if err != nil {
		return nil, err
	}
	if err := registry.Register(usersIndex, indexing.NewRegistration(
		usersIndex.Name,
		usersIndex,
		"user",
		usersgosearch.NewSource(userLoader),
		usersgosearch.NewUserProjector(usersgosearch.UserProjectorConfig{
			Index:      usersIndex.Name,
			SourceType: "user",
		}),
		func(record usersgosearch.UserRecord) string {
			return record.User.ID.String()
		},
	)); err != nil {
		return nil, err
	}

	metricHooks := []types.MetricsHook{metrics}
	activityHooks := []types.ActivityHook{activities, usersgosearch.ActivitySinkHook{
		Sink:    userStore,
		Logger:  logger,
		Metrics: []types.MetricsHook{metrics},
	}}

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
	userStore.SetHooks(usersgosearch.NewLifecycleHooks(usersgosearch.LifecycleHooksConfig{
		Indexer:         indexer,
		Index:           usersIndex.Name,
		RegistrationKey: "user",
		Logger:          logger,
		Metrics:         metricHooks,
	}).Hooks())

	cmsModule, cmsFixture, err := newCMSDemo(cfg, indexer, logger, metricHooks, sharedContentIndex, pageContentIndex, documentContentIndex, blogContentIndex)
	if err != nil {
		return nil, err
	}
	if err := registerCMSDemoSearch(registry, cmsModule, sharedContentIndex, pageContentIndex, documentContentIndex, blogContentIndex); err != nil {
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
		ScopeGuard: demoScopeGuard{},
		Defaults: planner.Defaults{
			DisableIndexGroupByDefault: true,
		},
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
		Editorial: editorialStore,
		Planner:   pln,
		Provider:  provider,
	})
	if err != nil {
		return nil, err
	}

	var searchDelegate SearchQuerier = searchQuery
	var suggestDelegate SuggestQuerier = suggestQuery
	if cfg.CacheEnabled {
		cacheStores.search = newRuntimeTTLStore[types.SearchResultPage]()
		cacheStores.suggest = newRuntimeTTLStore[types.SuggestResult]()
		cachedSearch, cacheErr := cache.NewCachedSearch(cache.CachedSearchConfig{
			Delegate:        searchQuery,
			Cache:           cacheStores.search,
			GenerationStore: generationStore,
			ProviderName:    provider.Name(),
			Logger:          logger,
			Metrics:         []types.MetricsHook{metrics},
		})
		if cacheErr != nil {
			return nil, cacheErr
		}
		cachedSuggest, cacheErr := cache.NewCachedSuggest(cache.CachedSuggestConfig{
			Delegate:        suggestQuery,
			Cache:           cacheStores.suggest,
			GenerationStore: generationStore,
			ProviderName:    provider.Name(),
			Logger:          logger,
			Metrics:         []types.MetricsHook{metrics},
		})
		if cacheErr != nil {
			return nil, cacheErr
		}
		searchDelegate = cachedSearch
		suggestDelegate = cachedSuggest
		cacheWrappers.Search = true
		cacheWrappers.Suggest = true
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
		provider:          provider,
		registry:          registry,
		planner:           pln,
		localeRuntime:     localeRuntime,
		generationStore:   generationStore,
		generationBackend: bootstrap.generationBackend,
		editorialStore:    editorialStore,
		metrics:           metrics,
		activities:        activities,
		cacheEnabled:      cfg.CacheEnabled,
		cacheWrappers:     cacheWrappers,
		cacheStores:       cacheStores,
		logger:            logger,
		ensureIndex:       ensureIndex,
		indexer:           indexer,
		reindex:           reindexCmd,
		search:            searchDelegate,
		suggest:           suggestDelegate,
		health:            healthQuery,
		stats:             statsQuery,
		editorialRules:    editorialRulesQuery,
		upsertRule:        upsertRuleCmd,
		deleteRule:        deleteRuleCmd,
		setRuleEnabled:    setRuleEnabledCmd,
		mediaIndex:        mediaIndex,
		contentShared:     sharedContentIndex,
		contentPage:       pageContentIndex,
		contentVideo:      videoContentIndex,
		contentDocument:   documentContentIndex,
		contentBlog:       blogContentIndex,
		usersIndex:        usersIndex,
		cmsModule:         cmsModule,
		cmsFixture:        cmsFixture,
		userStore:         userStore,
		cultureDataPath:   cfg.CultureDataPath,
		defaultLocale:     cfg.DefaultLocale,
		reindexBatchSize:  cfg.ReindexBatchSize,
		seedRecords:       transcriptRecords,
		closeProvider:     bootstrap.closeProvider,
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

func (r *Runtime) Close() error {
	if r == nil || r.closeProvider == nil {
		return nil
	}
	closeFn := r.closeProvider
	r.closeProvider = nil
	return closeFn()
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
		r.contentPage.Name,
		r.contentVideo.Name,
		r.contentDocument.Name,
		r.contentBlog.Name,
		r.usersIndex.Name,
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
		return []string{r.contentPage.Name, r.contentVideo.Name, r.contentDocument.Name, r.contentBlog.Name}
	case SurfaceUsers:
		return []string{r.usersIndex.Name}
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
		Provider:          r.provider.Name(),
		IndexName:         r.contentShared.Name,
		DefaultLocale:     r.defaultLocale,
		CultureDataPath:   r.cultureDataPath,
		GenerationBackend: r.generationBackend,
		CacheEnabled:      r.cacheEnabled,
		CacheWrappers:     r.cacheWrappers,
		Cache:             r.cacheStores.Snapshot(),
		SmokeFlows:        r.smokeFlows(),
		Documents:         documents,
		Generation:        generation,
		EditorialRules:    len(rules),
		Capabilities:      caps,
		Health:            health,
		Stats:             stats,
		Metrics:           r.metrics.Snapshot(),
		RecentActivities:  r.activities.Snapshot(),
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
	for _, def := range []types.IndexDefinition{r.mediaIndex, r.contentShared, r.contentPage, r.contentVideo, r.contentDocument, r.contentBlog, r.usersIndex} {
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

func (r *Runtime) CreateUser(ctx context.Context, user userstypes.AuthUser) (*userstypes.AuthUser, error) {
	if r == nil || r.userStore == nil {
		return nil, fmt.Errorf("user demo store is not initialized")
	}
	return r.userStore.Create(ctx, &user)
}

func (r *Runtime) UpdateUser(ctx context.Context, user userstypes.AuthUser) (*userstypes.AuthUser, error) {
	if r == nil || r.userStore == nil {
		return nil, fmt.Errorf("user demo store is not initialized")
	}
	return r.userStore.Update(ctx, &user)
}

func (r *Runtime) TransitionUser(ctx context.Context, userID string, target userstypes.LifecycleState) error {
	if r == nil || r.userStore == nil {
		return fmt.Errorf("user demo store is not initialized")
	}
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	_, err = r.userStore.UpdateStatus(ctx, userstypes.ActorRef{ID: demoUserIndexerActorID(), Type: "system"}, uid, target)
	return err
}

func (r *Runtime) UpdateUserProfile(ctx context.Context, userID string, patch userstypes.ProfilePatch) error {
	if r == nil || r.userStore == nil {
		return fmt.Errorf("user demo store is not initialized")
	}
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	patch.CanonicalizeLocale()
	return r.userStore.UpdateProfile(ctx, demoUserIndexerActorID(), uid, patch)
}

func (r *Runtime) UpdateUserRole(ctx context.Context, userID string, role string) error {
	if r == nil || r.userStore == nil {
		return fmt.Errorf("user demo store is not initialized")
	}
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	return r.userStore.UpdateRole(ctx, demoUserIndexerActorID(), uid, role)
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

func (r *Runtime) NormalizeSearchRequest(req SearchRequest) SearchRequest {
	req = r.BindSearchRequest(req)

	switch strings.TrimSpace(req.Surface) {
	case "":
		req.Surface = normalizeSurface("", req.Group)
	default:
		req.Surface = normalizeSurface(req.Surface, false)
	}

	switch req.Surface {
	case SurfaceUsers:
		req.Group = false
	case SurfaceContentShared, SurfaceContentSplit:
		if req.Group {
			req.Surface = SurfaceMediaGrouped
		}
	}

	return req
}

func (r *Runtime) Search(ctx context.Context, req SearchRequest) (types.SearchResultPage, error) {
	if r == nil || r.search == nil {
		return types.SearchResultPage{}, fmt.Errorf("search runtime is not initialized")
	}
	req = r.NormalizeSearchRequest(req)
	sortField, sortDir := normalizeSearchSort(req.SortField, req.SortDir)
	request := types.SearchRequest{
		Indexes: r.SurfaceIndexes(req.Surface),
		Query:   strings.TrimSpace(req.Query),
		Locale:  firstNonEmpty(strings.TrimSpace(req.Locale), r.defaultLocale),
		Page:    positiveOr(req.Page, 1),
		PerPage: positiveOr(req.PerPage, 10),
		Facets:  surfaceFacetRequests(req.Surface),
		Scope: types.Scope{
			TenantID: strings.TrimSpace(req.TenantID),
			OrgID:    strings.TrimSpace(req.OrgID),
		},
		Actor: types.ActorRef{
			UserID:   strings.TrimSpace(req.ActorUserID),
			TenantID: strings.TrimSpace(req.TenantID),
			OrgID:    strings.TrimSpace(req.OrgID),
			Metadata: actorMetadata(req.ActorRole),
		},
	}
	if req.CacheDisabled {
		request.Metadata = map[string]any{"cache_disabled": true}
	}
	if req.Surface == SurfaceMediaGrouped && req.Group {
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
		req.Surface,
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
	request := types.SuggestRequest{
		Indexes: r.SurfaceIndexes(req.Surface),
		Query:   strings.TrimSpace(req.Query),
		Locale:  firstNonEmpty(strings.TrimSpace(req.Locale), r.defaultLocale),
		Limit:   positiveOr(req.Limit, 5),
		Scope: types.Scope{
			TenantID: strings.TrimSpace(req.TenantID),
			OrgID:    strings.TrimSpace(req.OrgID),
		},
		Actor: types.ActorRef{
			UserID:   strings.TrimSpace(req.ActorUserID),
			TenantID: strings.TrimSpace(req.TenantID),
			OrgID:    strings.TrimSpace(req.OrgID),
			Metadata: actorMetadata(req.ActorRole),
		},
	}
	if req.CacheDisabled {
		request.Metadata = map[string]any{"cache_disabled": true}
	}
	return r.suggest.Query(ctx, request)
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

func (r *Runtime) smokeFlows() []SmokeFlow {
	return []SmokeFlow{
		{
			ID:          "archive_grouped",
			Label:       "Grouped archive search",
			Method:      "GET",
			Path:        "/api/demo/search?surface=media_grouped&group=true&locale=en&landing_slug=architecture&q=search",
			Description: "Validates grouped transcript results with hierarchical and disjunctive archive facets.",
		},
		{
			ID:          "heterogeneous_flat",
			Label:       "Flat heterogeneous content search",
			Method:      "GET",
			Path:        "/api/demo/search?surface=content_shared&locale=en&q=search",
			Description: "Checks unified search across video, document, and blog article content.",
		},
		{
			ID:          "shared_reindex",
			Label:       "Shared-index multi-registration rebuild",
			Method:      "POST",
			Path:        "/api/demo/reindex",
			Description: "Rebuilds all demo indexes so shared-index registration coverage can be revalidated with the search smoke flows.",
		},
		{
			ID:          "cache_invalidation",
			Label:       "Cache invalidation after rebuild",
			Method:      "POST+GET",
			Path:        "/api/demo/reindex then repeat /api/demo/search?surface=content_shared&locale=en&q=search",
			Description: "Confirms generation-based cache invalidation after indexed writes or rebuilds.",
		},
	}
}

func normalizeSurface(surface string, grouped bool) string {
	switch strings.TrimSpace(surface) {
	case SurfaceMediaGrouped, SurfaceContentShared, SurfaceContentSplit, SurfaceUsers:
		return strings.TrimSpace(surface)
	}
	if grouped {
		return SurfaceMediaGrouped
	}
	return SurfaceContentShared
}

func surfaceFacetRequests(surface string) []types.FacetRequest {
	if normalizeSurface(surface, false) == SurfaceUsers {
		return []types.FacetRequest{
			{Field: "status", Limit: 10, Disjunctive: true},
			{Field: "role", Limit: 10, Disjunctive: true},
		}
	}
	requests := media.DefaultArchiveFacetRequests()
	if normalizeSurface(surface, false) == SurfaceMediaGrouped {
		return requests
	}
	return append([]types.FacetRequest{{Field: "entity_type", Limit: 10, Disjunctive: true}}, requests...)
}

func contentSharedIndexName(mediaIndex string) string {
	return strings.TrimSpace(mediaIndex) + "_content_shared"
}
func contentPageIndexName(mediaIndex string) string {
	return strings.TrimSpace(mediaIndex) + "_pages"
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

func usersIndexName(mediaIndex string) string {
	return strings.TrimSpace(mediaIndex) + "_users"
}

func newProviderBootstrap(cfg Config) (providerBootstrap, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "memory":
		return providerBootstrap{
			provider:          memory.New(memory.Config{}),
			generationStore:   newMemoryGenerationStore(),
			generationBackend: generationBackendMemory,
		}, nil
	case "postgres":
		if strings.TrimSpace(cfg.PostgresDSN) == "" {
			return providerBootstrap{}, fmt.Errorf("postgres provider requires PostgresDSN")
		}
		sqlDB, err := sql.Open("postgres", strings.TrimSpace(cfg.PostgresDSN))
		if err != nil {
			return providerBootstrap{}, err
		}
		db := bun.NewDB(sqlDB, pgdialect.New())
		closeFn := db.Close
		if err := generationbunstore.Migrations().Migrate(context.Background(), db); err != nil {
			_ = closeFn()
			return providerBootstrap{}, err
		}
		provider, err := providerpostgres.New(providerpostgres.Config{DB: db})
		if err != nil {
			_ = closeFn()
			return providerBootstrap{}, err
		}
		return providerBootstrap{
			provider:          provider,
			generationStore:   generationbunstore.New(generationbunstore.Config{DB: db}),
			generationBackend: generationBackendBunPostgres,
			closeProvider:     closeFn,
		}, nil
	case "typesense":
		provider, err := providertypesense.New(providertypesense.Config{
			ServerURL:        strings.TrimSpace(cfg.TypesenseServerURL),
			APIKey:           strings.TrimSpace(cfg.TypesenseAPIKey),
			CollectionPrefix: strings.TrimSpace(cfg.TypesenseCollectionPrefix),
		})
		if err != nil {
			return providerBootstrap{}, err
		}
		return providerBootstrap{
			provider:          provider,
			generationStore:   newMemoryGenerationStore(),
			generationBackend: generationBackendMemory,
		}, nil
	default:
		return providerBootstrap{}, fmt.Errorf("unsupported search demo provider %q", cfg.Provider)
	}
}

func newCMSDemo(cfg Config, indexer *indexing.Indexer, logger types.Logger, metrics []types.MetricsHook, sharedIndex, pageIndex, documentIndex, blogIndex types.IndexDefinition) (*cms.Module, *cmsFixture, error) {
	cmsCfg := cms.DefaultConfig()
	cmsCfg.DefaultLocale = cfg.DefaultLocale
	cmsCfg.I18N.Locales = demoCMSLocales(cfg.DefaultLocale)
	cmsCfg.I18N.RequireTranslations = true
	cmsCfg.I18N.DefaultLocaleRequired = true
	cmsCfg.Activity.Enabled = false

	lifecycleHook := cmsgosearch.NewLifecycleHook(cmsgosearch.LifecycleHookConfig{
		Indexer: indexer,
		Routes: []cmsgosearch.Route{
			{ResourceType: "page", ContentTypeSlug: "landing-page", Index: sharedIndex.Name, RegistrationKey: "cms_page"},
			{ResourceType: "page", ContentTypeSlug: "landing-page", Index: pageIndex.Name, RegistrationKey: "cms_page"},
			{ResourceType: "content", ContentTypeSlug: "document", Index: sharedIndex.Name, RegistrationKey: "cms_document"},
			{ResourceType: "content", ContentTypeSlug: "document", Index: documentIndex.Name, RegistrationKey: "cms_document"},
			{ResourceType: "content", ContentTypeSlug: "blog-article", Index: sharedIndex.Name, RegistrationKey: "cms_blog_article"},
			{ResourceType: "content", ContentTypeSlug: "blog-article", Index: blogIndex.Name, RegistrationKey: "cms_blog_article"},
		},
		Logger:  logger,
		Metrics: metrics,
	})

	module, err := cms.New(cmsCfg, cms.WithLifecycleHooks(cmslifecycle.Hooks{lifecycleHook}))
	if err != nil {
		return nil, nil, err
	}

	fixture, err := seedCMSDemo(context.Background(), module, cfg.DefaultLocale, sharedIndex.Name)
	if err != nil {
		return nil, nil, err
	}

	return module, fixture, nil
}

func registerCMSDemoSearch(registry *indexing.Registry, module *cms.Module, sharedIndex, pageIndex, documentIndex, blogIndex types.IndexDefinition) error {
	if registry == nil || module == nil {
		return fmt.Errorf("cms demo registry requires runtime and module")
	}

	pageSource := cmsgosearch.NewPageSource(cmsgosearch.PageSourceConfig{
		Service: module.Pages(),
	})
	documentSource := cmsgosearch.NewContentSource(cmsgosearch.ContentSourceConfig{
		Service:          module.Content(),
		ContentTypeSlugs: []string{"document"},
	})
	blogSource := cmsgosearch.NewContentSource(cmsgosearch.ContentSourceConfig{
		Service:          module.Content(),
		ContentTypeSlugs: []string{"blog-article"},
	})

	pageID := func(record *cmspages.Page) string {
		if record == nil {
			return ""
		}
		return record.ID.String()
	}
	contentID := func(record *cmscontent.Content) string {
		if record == nil {
			return ""
		}
		return record.ID.String()
	}

	if err := registry.Register(sharedIndex, indexing.NewRegistrationWithKey(
		sharedIndex.Name,
		sharedIndex,
		"cms_page",
		"page",
		pageSource,
		cmsgosearch.NewPageProjector(cmsgosearch.ProjectorConfig{
			Index:           sharedIndex.Name,
			RegistrationKey: "cms_page",
			SourceType:      "page",
		}),
		pageID,
	)); err != nil {
		return err
	}

	if err := registry.Register(pageIndex, indexing.NewRegistrationWithKey(
		pageIndex.Name,
		pageIndex,
		"cms_page",
		"page",
		pageSource,
		cmsgosearch.NewPageProjector(cmsgosearch.ProjectorConfig{
			Index:           pageIndex.Name,
			RegistrationKey: "cms_page",
			SourceType:      "page",
		}),
		pageID,
	)); err != nil {
		return err
	}

	if err := registry.Register(sharedIndex, indexing.NewRegistrationWithKey(
		sharedIndex.Name,
		sharedIndex,
		"cms_document",
		"document",
		documentSource,
		cmsgosearch.NewDocumentProjector(cmsgosearch.ProjectorConfig{
			Index:           sharedIndex.Name,
			RegistrationKey: "cms_document",
			SourceType:      "document",
		}),
		contentID,
	)); err != nil {
		return err
	}

	if err := registry.Register(documentIndex, indexing.NewRegistrationWithKey(
		documentIndex.Name,
		documentIndex,
		"cms_document",
		"document",
		documentSource,
		cmsgosearch.NewDocumentProjector(cmsgosearch.ProjectorConfig{
			Index:           documentIndex.Name,
			RegistrationKey: "cms_document",
			SourceType:      "document",
		}),
		contentID,
	)); err != nil {
		return err
	}

	if err := registry.Register(sharedIndex, indexing.NewRegistrationWithKey(
		sharedIndex.Name,
		sharedIndex,
		"cms_blog_article",
		"blog_article",
		blogSource,
		cmsgosearch.NewBlogArticleProjector(cmsgosearch.ProjectorConfig{
			Index:           sharedIndex.Name,
			RegistrationKey: "cms_blog_article",
			SourceType:      "blog_article",
		}),
		contentID,
	)); err != nil {
		return err
	}

	return registry.Register(blogIndex, indexing.NewRegistrationWithKey(
		blogIndex.Name,
		blogIndex,
		"cms_blog_article",
		"blog_article",
		blogSource,
		cmsgosearch.NewBlogArticleProjector(cmsgosearch.ProjectorConfig{
			Index:           blogIndex.Name,
			RegistrationKey: "cms_blog_article",
			SourceType:      "blog_article",
		}),
		contentID,
	))
}

func seedCMSDemo(ctx context.Context, module *cms.Module, defaultLocale, sharedIndex string) (*cmsFixture, error) {
	if module == nil {
		return nil, fmt.Errorf("cms demo module is required")
	}

	actorID := uuid.New()
	templateID := uuid.New()
	secondaryLocale := demoSecondaryLocale(defaultLocale)

	pageType, err := module.ContentTypes().Create(ctx, cmscontent.CreateContentTypeRequest{
		Name:         "Landing Page",
		Slug:         "landing-page",
		Schema:       demoCMSContentTypeSchema(),
		Status:       "active",
		CreatedBy:    actorID,
		UpdatedBy:    actorID,
		Capabilities: demoSearchCapabilities(sharedIndex),
	})
	if err != nil {
		return nil, err
	}
	documentType, err := module.ContentTypes().Create(ctx, cmscontent.CreateContentTypeRequest{
		Name:         "Document",
		Slug:         "document",
		Schema:       demoCMSContentTypeSchema(),
		Status:       "active",
		CreatedBy:    actorID,
		UpdatedBy:    actorID,
		Capabilities: demoSearchCapabilities(sharedIndex),
	})
	if err != nil {
		return nil, err
	}
	blogType, err := module.ContentTypes().Create(ctx, cmscontent.CreateContentTypeRequest{
		Name:         "Blog Article",
		Slug:         "blog-article",
		Schema:       demoCMSContentTypeSchema(),
		Status:       "active",
		CreatedBy:    actorID,
		UpdatedBy:    actorID,
		Capabilities: demoSearchCapabilities(sharedIndex),
	})
	if err != nil {
		return nil, err
	}

	pageContent, err := module.Content().Create(ctx, cmscontent.CreateContentRequest{
		ContentTypeID: pageType.ID,
		Slug:          "phase-seven-home-content",
		Status:        "published",
		CreatedBy:     actorID,
		UpdatedBy:     actorID,
		Translations: []cmscontent.ContentTranslationInput{
			{Locale: defaultLocale, Title: "Phase Seven Home Content", Content: map[string]any{"headline": "phasesevenpage", "body": "Phase seven page content for lifecycle search checks."}},
			{Locale: secondaryLocale, Title: "Contenido Inicio Fase Siete", Content: map[string]any{"headline": "phasesevenpage", "body": "Contenido de pagina para comprobar ciclo de vida en busqueda."}},
		},
	})
	if err != nil {
		return nil, err
	}

	pageRecord, err := module.Pages().Create(ctx, cmspages.CreatePageRequest{
		ContentID:  pageContent.ID,
		TemplateID: templateID,
		Slug:       "phase-seven-home",
		Status:     "published",
		CreatedBy:  actorID,
		UpdatedBy:  actorID,
		Translations: []cmspages.PageTranslationInput{
			{Locale: defaultLocale, Title: "PhaseSevenHome Search", Path: "/phase-seven-home"},
			{Locale: secondaryLocale, Title: "PhaseSevenHome Inicio", Path: "/es/phase-seven-home"},
		},
	})
	if err != nil {
		return nil, err
	}

	documentRecord, err := module.Content().Create(ctx, cmscontent.CreateContentRequest{
		ContentTypeID: documentType.ID,
		Slug:          "phase-seven-handbook",
		Status:        "published",
		CreatedBy:     actorID,
		UpdatedBy:     actorID,
		Translations: []cmscontent.ContentTranslationInput{
			{Locale: defaultLocale, Title: "Phase Seven Search Handbook", Content: map[string]any{"headline": "phasesevendocument", "body": "Search handbook for lifecycle indexing checks."}},
			{Locale: secondaryLocale, Title: "Manual de Busqueda Fase Siete", Content: map[string]any{"headline": "phasesevendocumentes", "body": "Manual para verificar eliminacion de traducciones."}},
		},
	})
	if err != nil {
		return nil, err
	}

	blogRecord, err := module.Content().Create(ctx, cmscontent.CreateContentRequest{
		ContentTypeID: blogType.ID,
		Slug:          "phase-seven-notes",
		Status:        "published",
		CreatedBy:     actorID,
		UpdatedBy:     actorID,
		Translations: []cmscontent.ContentTranslationInput{
			{Locale: defaultLocale, Title: "Phase Seven Search Notes", Content: map[string]any{"headline": "phasesevenblog", "body": "Blog article used for lifecycle search smoke coverage."}},
			{Locale: secondaryLocale, Title: "Notas de Busqueda Fase Siete", Content: map[string]any{"headline": "phasesevenblog", "body": "Articulo para comprobar actualizacion de traducciones."}},
		},
	})
	if err != nil {
		return nil, err
	}

	return &cmsFixture{
		actorID:                actorID,
		templateID:             templateID,
		pageContentID:          pageContent.ID,
		pageID:                 pageRecord.ID,
		documentID:             documentRecord.ID,
		blogID:                 blogRecord.ID,
		defaultLocale:          defaultLocale,
		secondaryLocale:        secondaryLocale,
		pageQuery:              "PhaseSevenHome",
		documentQuery:          "phasesevendocument",
		blogQuery:              "phasesevenblog",
		blogUpdatedESQuery:     "phasesevenblogesupdated",
		documentDeletedESQuery: "phasesevendocumentes",
	}, nil
}

func demoCMSLocales(defaultLocale string) []string {
	secondary := demoSecondaryLocale(defaultLocale)
	if secondary == defaultLocale {
		return []string{defaultLocale}
	}
	return []string{defaultLocale, secondary}
}

func demoSecondaryLocale(defaultLocale string) string {
	if strings.EqualFold(strings.TrimSpace(defaultLocale), "es") {
		return "en"
	}
	return "es"
}

func demoCMSContentTypeSchema() map[string]any {
	return map[string]any{
		"fields": []any{"headline", "body"},
	}
}

func demoSearchCapabilities(indexName string) map[string]any {
	return map[string]any{
		"search": map[string]any{
			"enabled": true,
			"index":   indexName,
		},
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

func searchFiltersExpr(surface string, filters map[string][]string, publishedYearGTE, publishedYearLTE, durationSecondsGTE, durationSecondsLTE *int) types.FilterExpr {
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
	if normalizeSurface(surface, false) == SurfaceUsers {
		return collapseFilterTerms(terms)
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

type demoScopeGuard struct{}

func (demoScopeGuard) AllowSearch(context.Context, types.ActorRef, types.SearchRequest) bool {
	return true
}
func (demoScopeGuard) AllowSuggest(context.Context, types.ActorRef, types.SuggestRequest) bool {
	return true
}

func (demoScopeGuard) AllowDocument(_ context.Context, actor types.ActorRef, doc types.Document) bool {
	if !strings.EqualFold(strings.TrimSpace(doc.Type), "user") {
		return true
	}
	role := strings.TrimSpace(fmt.Sprint(actor.Metadata["role"]))
	if strings.EqualFold(role, "support") {
		return strings.TrimSpace(actor.UserID) != "" && strings.EqualFold(strings.TrimSpace(actor.UserID), strings.TrimSpace(doc.SourceID))
	}
	return true
}

func actorMetadata(role string) map[string]any {
	role = strings.TrimSpace(role)
	if role == "" {
		return nil
	}
	return map[string]any{"role": role}
}

func userIndexDefinition(mediaIndex string) types.IndexDefinition {
	return types.IndexDefinition{
		Name:               usersIndexName(mediaIndex),
		Label:              "Users",
		DefaultQueryFields: []string{"title", "summary", "body"},
		SearchableFields:   []string{"title", "summary", "body"},
		FacetFields:        []string{"role", "status"},
		SortableFields:     []string{"title"},
		FilterableFields:   []string{"role", "status"},
		HighlightFields:    []string{"title", "body"},
		DefaultSort:        []types.Sort{{Field: "title", Direction: types.SortAsc}},
	}
}

func demoUserIndexerActorID() uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-000000000900")
}

func demoUserScopeResolver(_ context.Context, user userstypes.AuthUser, profile *userstypes.UserProfile) (userstypes.ScopeFilter, error) {
	if profile != nil {
		scope := profile.Scope.Clone()
		if scope.TenantID != uuid.Nil || scope.OrgID != uuid.Nil || len(scope.Labels) > 0 {
			return scope, nil
		}
	}
	scope := userstypes.ScopeFilter{}
	if raw, ok := user.Metadata["tenant_id"].(string); ok {
		scope.TenantID = parseUUID(raw)
	}
	if raw, ok := user.Metadata["org_id"].(string); ok {
		scope.OrgID = parseUUID(raw)
	}
	return scope, nil
}

type demoUserStore struct {
	mu         sync.RWMutex
	users      map[uuid.UUID]userstypes.AuthUser
	order      []uuid.UUID
	profiles   map[string]userstypes.UserProfile
	activities []userstypes.ActivityRecord
	hooks      userstypes.Hooks
}

func newDemoUserStore(defaultLocale string) *demoUserStore {
	store := &demoUserStore{
		users:    map[uuid.UUID]userstypes.AuthUser{},
		profiles: map[string]userstypes.UserProfile{},
	}
	for _, fixture := range demoUserFixtures(defaultLocale) {
		store.users[fixture.user.ID] = fixture.user
		store.order = append(store.order, fixture.user.ID)
		store.profiles[userProfileKey(fixture.profile.UserID, fixture.profile.Scope)] = fixture.profile
	}
	return store
}

func (s *demoUserStore) SetHooks(hooks userstypes.Hooks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = hooks
}

func (s *demoUserStore) GetByID(_ context.Context, id uuid.UUID) (*userstypes.AuthUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return nil, nil
	}
	copy := user
	return &copy, nil
}

func (s *demoUserStore) GetByIdentifier(_ context.Context, identifier string) (*userstypes.AuthUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	identifier = strings.TrimSpace(identifier)
	for _, user := range s.users {
		if strings.EqualFold(user.Email, identifier) || strings.EqualFold(user.Username, identifier) || strings.EqualFold(user.ID.String(), identifier) {
			copy := user
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *demoUserStore) Create(ctx context.Context, input *userstypes.AuthUser) (*userstypes.AuthUser, error) {
	if input == nil {
		return nil, fmt.Errorf("user is required")
	}
	user := *input
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	s.mu.Lock()
	s.users[user.ID] = user
	if !containsUUID(s.order, user.ID) {
		s.order = append(s.order, user.ID)
	}
	hooks := s.hooks
	s.mu.Unlock()
	s.emitActivity(ctx, hooks, user, "user.created")
	copy := user
	return &copy, nil
}

func (s *demoUserStore) Update(ctx context.Context, input *userstypes.AuthUser) (*userstypes.AuthUser, error) {
	if input == nil || input.ID == uuid.Nil {
		return nil, fmt.Errorf("user id is required")
	}
	user := *input
	s.mu.Lock()
	s.users[user.ID] = user
	hooks := s.hooks
	s.mu.Unlock()
	s.emitActivity(ctx, hooks, user, "user.updated")
	copy := user
	return &copy, nil
}

func (s *demoUserStore) UpdateStatus(ctx context.Context, actor userstypes.ActorRef, id uuid.UUID, next userstypes.LifecycleState, _ ...userstypes.TransitionOption) (*userstypes.AuthUser, error) {
	s.mu.Lock()
	user, ok := s.users[id]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("user %s not found", id)
	}
	prev := user.Status
	user.Status = next
	s.users[id] = user
	hooks := s.hooks
	s.mu.Unlock()
	if hooks.AfterLifecycle != nil {
		scope, _ := demoUserScopeResolver(ctx, user, nil)
		hooks.AfterLifecycle(ctx, userstypes.LifecycleEvent{
			UserID:     id,
			ActorID:    actor.ID,
			FromState:  prev,
			ToState:    next,
			Scope:      scope,
			OccurredAt: time.Now().UTC(),
		})
	}
	copy := user
	return &copy, nil
}

func (s *demoUserStore) AllowedTransitions(context.Context, uuid.UUID) ([]userstypes.LifecycleTransition, error) {
	return nil, nil
}

func (s *demoUserStore) ResetPassword(context.Context, uuid.UUID, string) error { return nil }

func (s *demoUserStore) ListUsers(_ context.Context, filter userstypes.UserInventoryFilter) (userstypes.UserInventoryPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]userstypes.AuthUser, 0, len(s.order))
	for _, id := range s.order {
		user := s.users[id]
		if len(filter.UserIDs) > 0 && !containsUUID(filter.UserIDs, user.ID) {
			continue
		}
		if len(filter.Statuses) > 0 && !containsLifecycle(filter.Statuses, user.Status) {
			continue
		}
		if strings.TrimSpace(filter.Role) != "" && !strings.EqualFold(strings.TrimSpace(filter.Role), strings.TrimSpace(user.Role)) {
			continue
		}
		scope, _ := demoUserScopeResolver(context.Background(), user, nil)
		if filter.Scope.TenantID != uuid.Nil && scope.TenantID != filter.Scope.TenantID {
			continue
		}
		if filter.Scope.OrgID != uuid.Nil && scope.OrgID != filter.Scope.OrgID {
			continue
		}
		items = append(items, user)
	}
	start := filter.Pagination.Offset
	if start < 0 {
		start = 0
	}
	if start > len(items) {
		start = len(items)
	}
	limit := filter.Pagination.Limit
	if limit <= 0 || start+limit > len(items) {
		limit = len(items) - start
	}
	pageItems := append([]userstypes.AuthUser(nil), items[start:start+limit]...)
	next := start + limit
	return userstypes.UserInventoryPage{
		Users:      pageItems,
		Total:      len(items),
		NextOffset: next,
		HasMore:    next < len(items),
	}, nil
}

func (s *demoUserStore) GetProfile(_ context.Context, userID uuid.UUID, scope userstypes.ScopeFilter) (*userstypes.UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profile, ok := s.profiles[userProfileKey(userID, scope)]
	if !ok {
		return nil, nil
	}
	copy := profile
	return &copy, nil
}

func (s *demoUserStore) UpsertProfile(_ context.Context, profile userstypes.UserProfile) (*userstypes.UserProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[userProfileKey(profile.UserID, profile.Scope)] = profile
	copy := profile
	return &copy, nil
}

func (s *demoUserStore) Log(_ context.Context, record userstypes.ActivityRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activities = append(s.activities, record)
	return nil
}

func (s *demoUserStore) UpdateProfile(ctx context.Context, actorID, userID uuid.UUID, patch userstypes.ProfilePatch) error {
	scope, err := s.userScope(ctx, userID)
	if err != nil {
		return err
	}
	current, _ := s.GetProfile(ctx, userID, scope)
	profile := userstypes.UserProfile{
		UserID: userID,
		Scope:  scope,
	}
	if current != nil {
		profile = *current
	}
	if patch.DisplayName != nil {
		profile.DisplayName = *patch.DisplayName
	}
	if patch.AvatarURL != nil {
		profile.AvatarURL = *patch.AvatarURL
	}
	if patch.Locale != nil {
		profile.Locale = *patch.Locale
	}
	if patch.Timezone != nil {
		profile.Timezone = *patch.Timezone
	}
	if patch.Bio != nil {
		profile.Bio = *patch.Bio
	}
	if patch.Contact != nil {
		profile.Contact = cloneAnyMap(patch.Contact)
	}
	if patch.Metadata != nil {
		profile.Metadata = cloneAnyMap(patch.Metadata)
	}
	if _, err := s.UpsertProfile(ctx, profile); err != nil {
		return err
	}
	s.mu.RLock()
	hooks := s.hooks
	s.mu.RUnlock()
	if hooks.AfterProfileChange != nil {
		hooks.AfterProfileChange(ctx, userstypes.ProfileEvent{
			UserID:     userID,
			Scope:      scope,
			ActorID:    actorID,
			OccurredAt: time.Now().UTC(),
			Profile:    profile,
		})
	}
	return nil
}

func (s *demoUserStore) UpdateRole(ctx context.Context, actorID, userID uuid.UUID, role string) error {
	s.mu.Lock()
	user, ok := s.users[userID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("user %s not found", userID)
	}
	user.Role = strings.TrimSpace(role)
	s.users[userID] = user
	hooks := s.hooks
	s.mu.Unlock()
	if hooks.AfterRoleChange != nil {
		scope, _ := s.userScope(ctx, userID)
		hooks.AfterRoleChange(ctx, userstypes.RoleEvent{
			UserID:     userID,
			Action:     "role.assigned",
			ActorID:    actorID,
			Scope:      scope,
			OccurredAt: time.Now().UTC(),
		})
	}
	return nil
}

func (s *demoUserStore) userScope(ctx context.Context, userID uuid.UUID) (userstypes.ScopeFilter, error) {
	user, err := s.GetByID(ctx, userID)
	if err != nil || user == nil {
		return userstypes.ScopeFilter{}, err
	}
	return demoUserScopeResolver(ctx, *user, nil)
}

func (s *demoUserStore) emitActivity(ctx context.Context, hooks userstypes.Hooks, user userstypes.AuthUser, verb string) {
	if hooks.AfterActivity == nil {
		return
	}
	scope, _ := demoUserScopeResolver(ctx, user, nil)
	hooks.AfterActivity(ctx, userstypes.ActivityRecord{
		ID:         uuid.New(),
		UserID:     user.ID,
		Verb:       verb,
		ObjectType: "user",
		ObjectID:   user.ID.String(),
		Channel:    "users",
		TenantID:   scope.TenantID,
		OrgID:      scope.OrgID,
		OccurredAt: time.Now().UTC(),
	})
}

type demoUserFixture struct {
	user    userstypes.AuthUser
	profile userstypes.UserProfile
}

func demoUserFixtures(defaultLocale string) []demoUserFixture {
	tenantA := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	orgA := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	tenantB := uuid.MustParse("00000000-0000-0000-0000-000000000102")
	orgB := uuid.MustParse("00000000-0000-0000-0000-000000000202")
	return []demoUserFixture{
		newDemoUserFixture("00000000-0000-0000-0000-000000001001", "tenant-admin@example.com", "tenant-admin", "Tenant", "Admin", "admin", userstypes.LifecycleStateActive, tenantA, orgA, defaultLocale, "Tenant Admin", "Handles tenant scoped search rollouts and operational checks."),
		newDemoUserFixture("00000000-0000-0000-0000-000000001002", "support@example.com", "support-agent", "Support", "Agent", "support", userstypes.LifecycleStateActive, tenantA, orgA, defaultLocale, "Support Agent", "Support-only user for self-visibility guard checks."),
		newDemoUserFixture("00000000-0000-0000-0000-000000001003", "ops@example.com", "ops-manager", "Ops", "Manager", "manager", userstypes.LifecycleStateSuspended, tenantB, orgB, defaultLocale, "Ops Manager", "Suspended user fixture for lifecycle filtering and tenant isolation."),
		newDemoUserFixture("00000000-0000-0000-0000-000000001004", "editor@example.com", "content-editor", "Content", "Editor", "editor", userstypes.LifecycleStateDisabled, tenantA, orgA, defaultLocale, "Content Editor", "Disabled user retained in search for admin inventory checks."),
	}
}

func newDemoUserFixture(id, email, username, firstName, lastName, role string, status userstypes.LifecycleState, tenantID, orgID uuid.UUID, locale, displayName, bio string) demoUserFixture {
	userID := uuid.MustParse(id)
	metadata := map[string]any{
		"tenant_id": tenantID.String(),
		"org_id":    orgID.String(),
	}
	scope := userstypes.ScopeFilter{TenantID: tenantID, OrgID: orgID}
	return demoUserFixture{
		user: userstypes.AuthUser{
			ID:        userID,
			Role:      role,
			Status:    status,
			Email:     email,
			Username:  username,
			FirstName: firstName,
			LastName:  lastName,
			Metadata:  metadata,
		},
		profile: userstypes.UserProfile{
			UserID:      userID,
			DisplayName: displayName,
			Locale:      locale,
			Bio:         bio,
			Scope:       scope,
		},
	}
}

func userProfileKey(userID uuid.UUID, scope userstypes.ScopeFilter) string {
	return userID.String() + ":" + scope.TenantID.String() + ":" + scope.OrgID.String()
}

func containsUUID(values []uuid.UUID, needle uuid.UUID) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsLifecycle(values []userstypes.LifecycleState, needle userstypes.LifecycleState) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func parseUUID(raw string) uuid.UUID {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return parsed
}
