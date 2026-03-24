package cache

import (
	"context"
	"time"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type SuggestQuerier interface {
	Query(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error)
}

type CachedSuggestConfig struct {
	Delegate            SuggestQuerier
	Cache               Store[types.SuggestResult]
	GenerationStore     generationLookup
	ProviderName        string
	TTL                 time.Duration
	Strict              bool
	Logger              types.Logger
	Metrics             []types.MetricsHook
	CacheActorSensitive bool
}

type CachedSuggest struct {
	delegate            SuggestQuerier
	cache               Store[types.SuggestResult]
	generationStore     generationLookup
	providerName        string
	ttl                 time.Duration
	strict              bool
	cacheActorSensitive bool
	observer            cacheObserver
	generations         generationTracker
}

func NewCachedSuggest(cfg CachedSuggestConfig) (*CachedSuggest, error) {
	if cfg.Delegate == nil {
		return nil, errs.ConfigurationError("cached suggest delegate is required", nil)
	}
	if cfg.Cache == nil {
		return nil, errs.ConfigurationError("cached suggest cache is required", nil)
	}
	if cfg.GenerationStore == nil {
		return nil, errs.ConfigurationError("cached suggest generation store is required", nil)
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 15 * time.Second
	}
	return &CachedSuggest{
		delegate:            cfg.Delegate,
		cache:               cfg.Cache,
		generationStore:     cfg.GenerationStore,
		providerName:        cfg.ProviderName,
		ttl:                 cfg.TTL,
		strict:              cfg.Strict,
		cacheActorSensitive: cfg.CacheActorSensitive,
		observer:            newCacheObserver(cfg.Logger, cfg.Metrics),
	}, nil
}

func (q *CachedSuggest) Query(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	if reason, disabled := cacheBypassReason(req.Metadata); disabled {
		q.observer.bypass(ctx, "suggest", q.providerName, reason, map[string]any{
			"indexes": normalizeIndexes(req.Indexes),
		})
		return q.delegate.Query(ctx, req)
	}
	if !q.cacheActorSensitive && actorSensitive(req.Actor) {
		q.observer.bypass(ctx, "suggest", q.providerName, "actor_sensitive", map[string]any{
			"indexes": normalizeIndexes(req.Indexes),
		})
		return q.delegate.Query(ctx, req)
	}
	details, err := suggestCacheDetails(ctx, q.providerName, req, q.generationStore)
	if err != nil {
		q.observer.bypass(ctx, "suggest", q.providerName, "key_build_failed", map[string]any{
			"indexes": normalizeIndexes(req.Indexes),
		})
		if q.strict {
			return types.SuggestResult{}, err
		}
		return q.delegate.Query(ctx, req)
	}
	staleGeneration := q.generations.changed(details.BaseKey, details.GenerationFingerprint)
	if cached, ok, err := q.cache.Get(ctx, details.Key); err == nil && ok {
		q.observer.hit(ctx, "suggest", q.providerName, map[string]any{"indexes": details.Indexes})
		return cached, nil
	} else if err != nil && q.strict {
		q.observer.lookupFailure(ctx, "suggest", q.providerName, err, map[string]any{"indexes": details.Indexes})
		return types.SuggestResult{}, err
	} else if err != nil {
		q.observer.lookupFailure(ctx, "suggest", q.providerName, err, map[string]any{"indexes": details.Indexes})
	} else {
		if staleGeneration {
			q.observer.staleGenerationFallback(ctx, "suggest", q.providerName, map[string]any{"indexes": details.Indexes})
		}
		q.observer.miss(ctx, "suggest", q.providerName, map[string]any{"indexes": details.Indexes})
	}
	result, err := q.delegate.Query(ctx, req)
	if err != nil {
		return types.SuggestResult{}, err
	}
	if err := q.cache.Set(ctx, details.Key, result, q.ttl); err != nil {
		q.observer.writeFailure(ctx, "suggest", q.providerName, err, map[string]any{"indexes": details.Indexes})
		if q.strict {
			return types.SuggestResult{}, err
		}
	}
	return result, nil
}
