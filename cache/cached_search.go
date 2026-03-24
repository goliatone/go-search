package cache

import (
	"context"
	"time"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type SearchQuerier interface {
	Query(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error)
}

type CachedSearchConfig struct {
	Delegate            SearchQuerier
	Cache               Store[types.SearchResultPage]
	GenerationStore     generationLookup
	ProviderName        string
	TTL                 time.Duration
	Strict              bool
	Logger              types.Logger
	Metrics             []types.MetricsHook
	CacheActorSensitive bool
}

type CachedSearch struct {
	delegate            SearchQuerier
	cache               Store[types.SearchResultPage]
	generationStore     generationLookup
	providerName        string
	ttl                 time.Duration
	strict              bool
	cacheActorSensitive bool
	observer            cacheObserver
	generations         generationTracker
}

func NewCachedSearch(cfg CachedSearchConfig) (*CachedSearch, error) {
	if cfg.Delegate == nil {
		return nil, errs.ConfigurationError("cached search delegate is required", nil)
	}
	if cfg.Cache == nil {
		return nil, errs.ConfigurationError("cached search cache is required", nil)
	}
	if cfg.GenerationStore == nil {
		return nil, errs.ConfigurationError("cached search generation store is required", nil)
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 30 * time.Second
	}
	return &CachedSearch{
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

func (q *CachedSearch) Query(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	if reason, disabled := cacheBypassReason(req.Metadata); disabled {
		q.observer.bypass(ctx, "search", q.providerName, reason, map[string]any{
			"indexes": normalizeIndexes(req.Indexes),
		})
		return q.delegate.Query(ctx, req)
	}
	if !q.cacheActorSensitive && actorSensitive(req.Actor) {
		q.observer.bypass(ctx, "search", q.providerName, "actor_sensitive", map[string]any{
			"indexes": normalizeIndexes(req.Indexes),
		})
		return q.delegate.Query(ctx, req)
	}
	details, err := searchCacheDetails(ctx, q.providerName, req, q.generationStore)
	if err != nil {
		q.observer.bypass(ctx, "search", q.providerName, "key_build_failed", map[string]any{
			"indexes": normalizeIndexes(req.Indexes),
		})
		if q.strict {
			return types.SearchResultPage{}, err
		}
		return q.delegate.Query(ctx, req)
	}
	staleGeneration := q.generations.changed(details.BaseKey, details.GenerationFingerprint)
	if cached, ok, err := q.cache.Get(ctx, details.Key); err == nil && ok {
		q.observer.hit(ctx, "search", q.providerName, map[string]any{"indexes": details.Indexes})
		return cached, nil
	} else if err != nil && q.strict {
		q.observer.lookupFailure(ctx, "search", q.providerName, err, map[string]any{"indexes": details.Indexes})
		return types.SearchResultPage{}, err
	} else if err != nil {
		q.observer.lookupFailure(ctx, "search", q.providerName, err, map[string]any{"indexes": details.Indexes})
	} else {
		if staleGeneration {
			q.observer.staleGenerationFallback(ctx, "search", q.providerName, map[string]any{"indexes": details.Indexes})
		}
		q.observer.miss(ctx, "search", q.providerName, map[string]any{"indexes": details.Indexes})
	}
	page, err := q.delegate.Query(ctx, req)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	if err := q.cache.Set(ctx, details.Key, page, q.ttl); err != nil {
		q.observer.writeFailure(ctx, "search", q.providerName, err, map[string]any{"indexes": details.Indexes})
		if q.strict {
			return types.SearchResultPage{}, err
		}
	}
	return page, nil
}
