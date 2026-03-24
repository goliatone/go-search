package cache

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
)

type cacheObserver struct {
	logger  types.Logger
	metrics []types.MetricsHook
}

func newCacheObserver(logger types.Logger, metrics []types.MetricsHook) cacheObserver {
	return cacheObserver{
		logger:  logger,
		metrics: slices.Clone(metrics),
	}
}

func (o cacheObserver) hit(ctx context.Context, component, provider string, metadata map[string]any) {
	o.count(ctx, "search.cache.hit.count", component, provider, "")
	observe.Debug(o.logger, "search.cache.hit", cacheMetadata(component, provider, "", metadata))
}

func (o cacheObserver) miss(ctx context.Context, component, provider string, metadata map[string]any) {
	o.count(ctx, "search.cache.miss.count", component, provider, "")
	observe.Debug(o.logger, "search.cache.miss", cacheMetadata(component, provider, "", metadata))
}

func (o cacheObserver) staleGenerationFallback(ctx context.Context, component, provider string, metadata map[string]any) {
	o.count(ctx, "search.cache.stale_generation_fallback.count", component, provider, "")
	observe.Info(o.logger, "search.cache.stale_generation_fallback", cacheMetadata(component, provider, "", metadata))
}

func (o cacheObserver) bypass(ctx context.Context, component, provider, reason string, metadata map[string]any) {
	o.count(ctx, "search.cache.bypass.count", component, provider, reason)
	observe.Info(o.logger, "search.cache.bypass", cacheMetadata(component, provider, reason, metadata))
}

func (o cacheObserver) lookupFailure(ctx context.Context, component, provider string, err error, metadata map[string]any) {
	o.count(ctx, "search.cache.lookup_failure.count", component, provider, "")
	enriched := cacheMetadata(component, provider, "", metadata)
	enriched["error"] = fmt.Sprint(err)
	observe.Warn(o.logger, "search.cache.lookup_failure", enriched)
}

func (o cacheObserver) writeFailure(ctx context.Context, component, provider string, err error, metadata map[string]any) {
	o.count(ctx, "search.cache.write_failure.count", component, provider, "")
	enriched := cacheMetadata(component, provider, "", metadata)
	enriched["error"] = fmt.Sprint(err)
	observe.Warn(o.logger, "search.cache.write_failure", enriched)
}

func (o cacheObserver) invalidateFailure(ctx context.Context, component, provider string, err error, metadata map[string]any) {
	o.count(ctx, "search.cache.invalidate_failure.count", component, provider, "")
	enriched := cacheMetadata(component, provider, "", metadata)
	enriched["error"] = fmt.Sprint(err)
	observe.Warn(o.logger, "search.cache.invalidate_failure", enriched)
}

func (o cacheObserver) count(ctx context.Context, metric, component, provider, reason string) {
	labels := map[string]string{
		"component": component,
		"provider":  strings.TrimSpace(provider),
	}
	if reason != "" {
		labels["reason"] = reason
	}
	observe.Count(ctx, o.metrics, o.logger, metric, 1, labels)
}

func cacheMetadata(component, provider, reason string, metadata map[string]any) map[string]any {
	out := map[string]any{
		"component": component,
		"provider":  strings.TrimSpace(provider),
	}
	if reason != "" {
		out["reason"] = reason
	}
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

type generationTracker struct {
	mu     sync.Mutex
	latest map[string]string
}

func (t *generationTracker) changed(baseKey, fingerprint string) bool {
	if t == nil || baseKey == "" || fingerprint == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.latest == nil {
		t.latest = map[string]string{}
	}
	previous, ok := t.latest[baseKey]
	t.latest[baseKey] = fingerprint
	return ok && previous != fingerprint
}
