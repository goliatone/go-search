package cache

import (
	"context"
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

type suggestDelegateStub struct {
	calls  int
	result types.SuggestResult
}

func (s *suggestDelegateStub) Query(context.Context, types.SuggestRequest) (types.SuggestResult, error) {
	s.calls++
	return s.result, nil
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
