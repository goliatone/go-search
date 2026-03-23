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
	Delegate        SuggestQuerier
	Cache           Store[types.SuggestResult]
	GenerationStore generationLookup
	ProviderName    string
	TTL             time.Duration
	Strict          bool
}

type CachedSuggest struct {
	delegate        SuggestQuerier
	cache           Store[types.SuggestResult]
	generationStore generationLookup
	providerName    string
	ttl             time.Duration
	strict          bool
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
		delegate:        cfg.Delegate,
		cache:           cfg.Cache,
		generationStore: cfg.GenerationStore,
		providerName:    cfg.ProviderName,
		ttl:             cfg.TTL,
		strict:          cfg.Strict,
	}, nil
}

func (q *CachedSuggest) Query(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	if cacheDisabled(req.Metadata) {
		return q.delegate.Query(ctx, req)
	}
	key, err := suggestCacheKey(ctx, q.providerName, req, q.generationStore)
	if err != nil {
		if q.strict {
			return types.SuggestResult{}, err
		}
		return q.delegate.Query(ctx, req)
	}
	if cached, ok, err := q.cache.Get(ctx, key); err == nil && ok {
		return cached, nil
	} else if err != nil && q.strict {
		return types.SuggestResult{}, err
	}
	result, err := q.delegate.Query(ctx, req)
	if err != nil {
		return types.SuggestResult{}, err
	}
	if err := q.cache.Set(ctx, key, result, q.ttl); err != nil && q.strict {
		return types.SuggestResult{}, err
	}
	return result, nil
}
