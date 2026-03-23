package searchdemo

import (
	"context"
	"sync"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

type runtimeCacheEntry[V any] struct {
	value     V
	expiresAt time.Time
}

type runtimeCacheStoreStats struct {
	Entries int `json:"entries"`
	Hits    int `json:"hits"`
	Misses  int `json:"misses"`
	Sets    int `json:"sets"`
	Deletes int `json:"deletes"`
}

type runtimeCacheSnapshot struct {
	Search       runtimeCacheStoreStats `json:"search"`
	Suggest      runtimeCacheStoreStats `json:"suggest"`
	Capabilities runtimeCacheStoreStats `json:"capabilities"`
	Health       runtimeCacheStoreStats `json:"health"`
}

type runtimeCacheStores struct {
	search       *runtimeTTLStore[types.SearchResultPage]
	suggest      *runtimeTTLStore[types.SuggestResult]
	capabilities *runtimeTTLStore[types.CapabilitySet]
	health       *runtimeTTLStore[types.HealthStatus]
}

func (s *runtimeCacheStores) Snapshot() runtimeCacheSnapshot {
	if s == nil {
		return runtimeCacheSnapshot{}
	}
	return runtimeCacheSnapshot{
		Search:       snapshotStore(s.search),
		Suggest:      snapshotStore(s.suggest),
		Capabilities: snapshotStore(s.capabilities),
		Health:       snapshotStore(s.health),
	}
}

func snapshotStore[V any](store *runtimeTTLStore[V]) runtimeCacheStoreStats {
	if store == nil {
		return runtimeCacheStoreStats{}
	}
	return store.Snapshot()
}

type runtimeTTLStore[V any] struct {
	mu      sync.RWMutex
	items   map[string]runtimeCacheEntry[V]
	stats   runtimeCacheStoreStats
	zeroVal V
}

func newRuntimeTTLStore[V any]() *runtimeTTLStore[V] {
	return &runtimeTTLStore[V]{items: map[string]runtimeCacheEntry[V]{}}
}

func (s *runtimeTTLStore[V]) Get(_ context.Context, key string) (V, bool, error) {
	now := time.Now().UTC()
	s.mu.RLock()
	entry, ok := s.items[key]
	s.mu.RUnlock()
	if !ok {
		s.recordMiss()
		return s.zeroVal, false, nil
	}
	if !entry.expiresAt.IsZero() && !entry.expiresAt.After(now) {
		if err := s.Delete(context.Background(), key); err != nil {
			return s.zeroVal, false, err
		}
		s.recordMiss()
		return s.zeroVal, false, nil
	}
	s.recordHit()
	return entry.value, true, nil
}

func (s *runtimeTTLStore[V]) Set(_ context.Context, key string, value V, ttl time.Duration) error {
	expiresAt := time.Time{}
	if ttl > 0 {
		expiresAt = time.Now().UTC().Add(ttl)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = runtimeCacheEntry[V]{value: value, expiresAt: expiresAt}
	s.stats.Sets++
	s.stats.Entries = s.liveEntryCountLocked(time.Now().UTC())
	return nil
}

func (s *runtimeTTLStore[V]) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[key]; ok {
		delete(s.items, key)
		s.stats.Deletes++
	}
	s.stats.Entries = s.liveEntryCountLocked(time.Now().UTC())
	return nil
}

func (s *runtimeTTLStore[V]) Snapshot() runtimeCacheStoreStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Entries = s.liveEntryCountLocked(time.Now().UTC())
	return s.stats
}

func (s *runtimeTTLStore[V]) recordHit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Hits++
	s.stats.Entries = s.liveEntryCountLocked(time.Now().UTC())
}

func (s *runtimeTTLStore[V]) recordMiss() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Misses++
	s.stats.Entries = s.liveEntryCountLocked(time.Now().UTC())
}

func (s *runtimeTTLStore[V]) liveEntryCountLocked(now time.Time) int {
	live := 0
	for key, entry := range s.items {
		if !entry.expiresAt.IsZero() && !entry.expiresAt.After(now) {
			delete(s.items, key)
			continue
		}
		live++
	}
	return live
}
