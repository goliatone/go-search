package cache

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

type memoryStore[V any] struct {
	items map[string]V
}

func newMemoryStore[V any]() *memoryStore[V] {
	return &memoryStore[V]{items: map[string]V{}}
}

func (s *memoryStore[V]) Get(_ context.Context, key string) (V, bool, error) {
	value, ok := s.items[key]
	return value, ok, nil
}

func (s *memoryStore[V]) Set(_ context.Context, key string, value V, _ time.Duration) error {
	s.items[key] = value
	return nil
}

func (s *memoryStore[V]) Delete(_ context.Context, key string) error {
	delete(s.items, key)
	return nil
}

type failingStore[V any] struct {
	getErr error
	setErr error
}

func (s failingStore[V]) Get(context.Context, string) (V, bool, error) {
	var zero V
	return zero, false, s.getErr
}

func (s failingStore[V]) Set(context.Context, string, V, time.Duration) error {
	return s.setErr
}

func (s failingStore[V]) Delete(context.Context, string) error {
	return nil
}

type generationStoreStub struct {
	items map[string]int64
}

func (s generationStoreStub) Get(_ context.Context, index string) (int64, error) {
	return s.items[index], nil
}

type searchDelegateStub struct {
	calls int
	page  types.SearchResultPage
}

func (s *searchDelegateStub) Query(context.Context, types.SearchRequest) (types.SearchResultPage, error) {
	s.calls++
	return s.page, nil
}

type searchDelegateFunc func(context.Context, types.SearchRequest) (types.SearchResultPage, error)

func (fn searchDelegateFunc) Query(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	return fn(ctx, req)
}

type suggestDelegateStub struct {
	calls  int
	result types.SuggestResult
}

func (s *suggestDelegateStub) Query(context.Context, types.SuggestRequest) (types.SuggestResult, error) {
	s.calls++
	return s.result, nil
}

type metricsHookStub struct {
	counts map[string]int64
}

func newMetricsHookStub() *metricsHookStub {
	return &metricsHookStub{counts: map[string]int64{}}
}

func (h *metricsHookStub) Observe(context.Context, string, float64, map[string]string) {}

func (h *metricsHookStub) Count(_ context.Context, metric string, delta int64, _ map[string]string) {
	h.counts[metric] += delta
}

type loggerStub struct {
	messages []string
}

func (l *loggerStub) Debug(msg string, _ map[string]any) { l.messages = append(l.messages, msg) }
func (l *loggerStub) Info(msg string, _ map[string]any)  { l.messages = append(l.messages, msg) }
func (l *loggerStub) Warn(msg string, _ map[string]any)  { l.messages = append(l.messages, msg) }
func (l *loggerStub) Error(msg string, _ map[string]any) { l.messages = append(l.messages, msg) }

func (l *loggerStub) contains(msg string) bool {
	return slices.Contains(l.messages, msg)
}

type providerStub struct {
	healthCalls int
	capCalls    int
	healthy     bool
}

func (p *providerStub) Name() string { return "stub" }
func (p *providerStub) Capabilities(context.Context) (types.CapabilitySet, error) {
	p.capCalls++
	return types.CapabilitySet{Facets: true}, nil
}
func (p *providerStub) EnsureIndex(context.Context, types.IndexDefinition) error { return nil }
func (p *providerStub) Search(context.Context, types.SearchRequest) (types.SearchResultPage, error) {
	return types.SearchResultPage{}, nil
}
func (p *providerStub) Suggest(context.Context, types.SuggestRequest) (types.SuggestResult, error) {
	return types.SuggestResult{}, nil
}
func (p *providerStub) UpsertDocuments(context.Context, string, []types.Document) error { return nil }
func (p *providerStub) ReplaceDocuments(context.Context, string, string, []string, []types.Document) error {
	return nil
}
func (p *providerStub) DeleteDocuments(context.Context, string, []string) error { return nil }
func (p *providerStub) DeleteBySource(context.Context, string, string, []string) error {
	return nil
}
func (p *providerStub) Health(context.Context, types.HealthRequest) (types.HealthStatus, error) {
	p.healthCalls++
	return types.HealthStatus{Healthy: p.healthy}, nil
}

func TestCachedSearchUsesGenerationAwareKey(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore[types.SearchResultPage]()
	generations := generationStoreStub{items: map[string]int64{"media": 1}}
	delegate := &searchDelegateStub{page: types.SearchResultPage{Total: 1}}
	cached, err := NewCachedSearch(CachedSearchConfig{
		Delegate:        delegate,
		Cache:           store,
		GenerationStore: generations,
		ProviderName:    "memory",
	})
	if err != nil {
		t.Fatalf("new cached search: %v", err)
	}
	req := types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "Prayer",
		Locale:  "en",
		Locales: []string{"bo", "en"},
		Page:    1,
		PerPage: 10,
	}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("first query: %v", err)
	}
	req.Locales = []string{"en", "bo"}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("second query: %v", err)
	}
	if delegate.calls != 1 {
		t.Fatalf("expected one delegate call, got %d", delegate.calls)
	}
	generations.items["media"] = 2
	cached.generationStore = generations
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("third query: %v", err)
	}
	if delegate.calls != 2 {
		t.Fatalf("expected generation bump to invalidate cache, got %d calls", delegate.calls)
	}
}

func TestCachedSuggestHonorsCacheDisabledMetadata(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore[types.SuggestResult]()
	delegate := &suggestDelegateStub{result: types.SuggestResult{Items: []types.SuggestHit{{ID: "1"}}}}
	cached, err := NewCachedSuggest(CachedSuggestConfig{
		Delegate:        delegate,
		Cache:           store,
		GenerationStore: generationStoreStub{items: map[string]int64{"media": 1}},
		ProviderName:    "memory",
	})
	if err != nil {
		t.Fatalf("new cached suggest: %v", err)
	}
	req := types.SuggestRequest{
		Indexes:  []string{"media"},
		Query:    "ocean",
		Metadata: map[string]any{"cache_disabled": true},
	}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if delegate.calls != 2 {
		t.Fatalf("expected uncached delegate calls, got %d", delegate.calls)
	}
}

func TestSearchCacheKeyCanonicalizesInFilterValueOrder(t *testing.T) {
	ctx := context.Background()
	generations := generationStoreStub{items: map[string]int64{"media": 3}}
	reqA := types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "archive",
		Filters: types.TermExpr{
			Field: "topic_hierarchy",
			Op:    types.FilterOpIn,
			Value: []string{"Teaching Topics > Tara", "Teaching Topics > Architecture"},
		},
	}
	reqB := types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "archive",
		Filters: types.TermExpr{
			Field: "topic_hierarchy",
			Op:    types.FilterOpIn,
			Value: []string{"Teaching Topics > Architecture", "Teaching Topics > Tara"},
		},
	}
	keyA, err := searchCacheKey(ctx, "postgres", reqA, generations)
	if err != nil {
		t.Fatalf("searchCacheKey A: %v", err)
	}
	keyB, err := searchCacheKey(ctx, "postgres", reqB, generations)
	if err != nil {
		t.Fatalf("searchCacheKey B: %v", err)
	}
	if keyA != keyB {
		t.Fatalf("expected canonical cache keys, got %q != %q", keyA, keyB)
	}
}

func TestCachedProviderMetadataCachesCapabilitiesAndInvalidatesHealth(t *testing.T) {
	ctx := context.Background()
	provider := &providerStub{healthy: true}
	wrapper, err := NewCachedProviderMetadata(CachedProviderMetadataConfig{
		Provider:        provider,
		CapabilityCache: newMemoryStore[types.CapabilitySet](),
		HealthCache:     newMemoryStore[types.HealthStatus](),
	})
	if err != nil {
		t.Fatalf("new cached provider metadata: %v", err)
	}
	if _, err := wrapper.Capabilities(ctx); err != nil {
		t.Fatalf("capabilities 1: %v", err)
	}
	if _, err := wrapper.Capabilities(ctx); err != nil {
		t.Fatalf("capabilities 2: %v", err)
	}
	if provider.capCalls != 1 {
		t.Fatalf("expected one capabilities call, got %d", provider.capCalls)
	}
	if _, err := wrapper.Health(ctx, types.HealthRequest{Indexes: []string{"media"}}); err != nil {
		t.Fatalf("health 1: %v", err)
	}
	if _, err := wrapper.Health(ctx, types.HealthRequest{Indexes: []string{"media"}}); err != nil {
		t.Fatalf("health 2: %v", err)
	}
	if provider.healthCalls != 1 {
		t.Fatalf("expected cached health, got %d calls", provider.healthCalls)
	}
	if err := wrapper.UpsertDocuments(ctx, "media", nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := wrapper.Health(ctx, types.HealthRequest{Indexes: []string{"media"}}); err != nil {
		t.Fatalf("health 3: %v", err)
	}
	if provider.healthCalls != 2 {
		t.Fatalf("expected health invalidation after write, got %d calls", provider.healthCalls)
	}
}

func TestCachedSearchTreatsLookupFailureAsAdvisoryByDefault(t *testing.T) {
	ctx := context.Background()
	delegate := &searchDelegateStub{page: types.SearchResultPage{Total: 2}}
	metrics := newMetricsHookStub()
	logger := &loggerStub{}
	cached, err := NewCachedSearch(CachedSearchConfig{
		Delegate:        delegate,
		Cache:           failingStore[types.SearchResultPage]{getErr: errors.New("cache read failed")},
		GenerationStore: generationStoreStub{items: map[string]int64{"media": 1}},
		ProviderName:    "memory",
		Logger:          logger,
		Metrics:         []types.MetricsHook{metrics},
	})
	if err != nil {
		t.Fatalf("new cached search: %v", err)
	}
	page, err := cached.Query(ctx, types.SearchRequest{Indexes: []string{"media"}, Query: "ocean"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if page.Total != 2 || delegate.calls != 1 {
		t.Fatalf("expected delegate fallback, page=%+v calls=%d", page, delegate.calls)
	}
	if metrics.counts["search.cache.lookup_failure.count"] != 1 {
		t.Fatalf("lookup failure count = %d", metrics.counts["search.cache.lookup_failure.count"])
	}
	if !logger.contains("search.cache.lookup_failure") {
		t.Fatalf("expected lookup failure log, got %v", logger.messages)
	}
}

func TestCachedSearchTreatsWriteFailureAsAdvisoryByDefault(t *testing.T) {
	ctx := context.Background()
	delegate := &searchDelegateStub{page: types.SearchResultPage{Total: 3}}
	metrics := newMetricsHookStub()
	logger := &loggerStub{}
	cached, err := NewCachedSearch(CachedSearchConfig{
		Delegate:        delegate,
		Cache:           failingStore[types.SearchResultPage]{setErr: errors.New("cache write failed")},
		GenerationStore: generationStoreStub{items: map[string]int64{"media": 1}},
		ProviderName:    "memory",
		Logger:          logger,
		Metrics:         []types.MetricsHook{metrics},
	})
	if err != nil {
		t.Fatalf("new cached search: %v", err)
	}
	page, err := cached.Query(ctx, types.SearchRequest{Indexes: []string{"media"}, Query: "ocean"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if page.Total != 3 || delegate.calls != 1 {
		t.Fatalf("expected delegate result after advisory write failure, page=%+v calls=%d", page, delegate.calls)
	}
	if metrics.counts["search.cache.write_failure.count"] != 1 {
		t.Fatalf("write failure count = %d", metrics.counts["search.cache.write_failure.count"])
	}
	if !logger.contains("search.cache.write_failure") {
		t.Fatalf("expected write failure log, got %v", logger.messages)
	}
}

func TestCachedSearchRecordsBypassAndStaleGenerationMetrics(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore[types.SearchResultPage]()
	metrics := newMetricsHookStub()
	logger := &loggerStub{}
	generations := generationStoreStub{items: map[string]int64{"media": 1}}
	delegateCalls := 0
	cached, err := NewCachedSearch(CachedSearchConfig{
		Delegate: searchDelegateFunc(func(context.Context, types.SearchRequest) (types.SearchResultPage, error) {
			delegateCalls++
			return types.SearchResultPage{Total: delegateCalls}, nil
		}),
		Cache:           store,
		GenerationStore: generations,
		ProviderName:    "memory",
		Logger:          logger,
		Metrics:         []types.MetricsHook{metrics},
	})
	if err != nil {
		t.Fatalf("new cached search: %v", err)
	}

	_, err = cached.Query(ctx, types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Metadata: map[string]any{
			"cache_disabled":        true,
			"cache_disabled_reason": "integration_test",
		},
	})
	if err != nil {
		t.Fatalf("bypass query: %v", err)
	}
	if metrics.counts["search.cache.bypass.count"] != 1 {
		t.Fatalf("bypass count = %d", metrics.counts["search.cache.bypass.count"])
	}

	req := types.SearchRequest{Indexes: []string{"media"}, Query: "prayer"}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if metrics.counts["search.cache.hit.count"] != 1 {
		t.Fatalf("hit count = %d", metrics.counts["search.cache.hit.count"])
	}
	generations.items["media"] = 2
	cached.generationStore = generations
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query after generation bump: %v", err)
	}
	if metrics.counts["search.cache.stale_generation_fallback.count"] != 1 {
		t.Fatalf("stale generation fallback count = %d", metrics.counts["search.cache.stale_generation_fallback.count"])
	}
	if !logger.contains("search.cache.stale_generation_fallback") {
		t.Fatalf("expected stale generation log, got %v", logger.messages)
	}
}

func TestCachedSuggestBypassesActorSensitiveRequestsByDefault(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore[types.SuggestResult]()
	delegate := &suggestDelegateStub{result: types.SuggestResult{Items: []types.SuggestHit{{ID: "1"}}}}
	metrics := newMetricsHookStub()
	cached, err := NewCachedSuggest(CachedSuggestConfig{
		Delegate:        delegate,
		Cache:           store,
		GenerationStore: generationStoreStub{items: map[string]int64{"media": 1}},
		ProviderName:    "memory",
		Metrics:         []types.MetricsHook{metrics},
	})
	if err != nil {
		t.Fatalf("new cached suggest: %v", err)
	}
	req := types.SuggestRequest{
		Indexes: []string{"media"},
		Query:   "ocean",
		Actor: types.ActorRef{
			UserID:   "user-1",
			Metadata: map[string]any{"role": "member"},
		},
	}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if delegate.calls != 2 {
		t.Fatalf("expected actor-sensitive requests to bypass cache, got %d delegate calls", delegate.calls)
	}
	if metrics.counts["search.cache.bypass.count"] != 2 {
		t.Fatalf("bypass count = %d", metrics.counts["search.cache.bypass.count"])
	}
}

func TestCachedSearchKeyBuildFailureUsesBypassMetric(t *testing.T) {
	ctx := context.Background()
	delegate := &searchDelegateStub{page: types.SearchResultPage{Total: 1}}
	metrics := newMetricsHookStub()
	cached, err := NewCachedSearch(CachedSearchConfig{
		Delegate: delegate,
		Cache:    newMemoryStore[types.SearchResultPage](),
		GenerationStore: generationStoreStub{items: map[string]int64{
			"media": 1,
		}},
		ProviderName: "memory",
		Metrics:      []types.MetricsHook{metrics},
	})
	if err != nil {
		t.Fatalf("new cached search: %v", err)
	}
	req := types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "ocean",
		Metadata: map[string]any{
			"broken": func() {},
		},
	}
	if _, err := cached.Query(ctx, req); err != nil {
		t.Fatalf("query: %v", err)
	}
	if metrics.counts["search.cache.bypass.count"] != 1 {
		t.Fatalf("bypass count = %d", metrics.counts["search.cache.bypass.count"])
	}
	if delegate.calls != 1 {
		t.Fatalf("delegate calls = %d", delegate.calls)
	}
}
