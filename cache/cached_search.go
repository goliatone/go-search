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
	Delegate        SearchQuerier
	Cache           Store[types.SearchResultPage]
	GenerationStore generationLookup
	ProviderName    string
	TTL             time.Duration
	Strict          bool
}

type CachedSearch struct {
	delegate        SearchQuerier
	cache           Store[types.SearchResultPage]
	generationStore generationLookup
	providerName    string
	ttl             time.Duration
	strict          bool
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
		delegate:        cfg.Delegate,
		cache:           cfg.Cache,
		generationStore: cfg.GenerationStore,
		providerName:    cfg.ProviderName,
		ttl:             cfg.TTL,
		strict:          cfg.Strict,
	}, nil
}

func (q *CachedSearch) Query(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	if cacheDisabled(req.Metadata) {
		return q.delegate.Query(ctx, req)
	}
	key, err := searchCacheKey(ctx, q.providerName, req, q.generationStore)
	if err != nil {
		if q.strict {
			return types.SearchResultPage{}, err
		}
		return q.delegate.Query(ctx, req)
	}
	if cached, ok, err := q.cache.Get(ctx, key); err == nil && ok {
		return cached, nil
	} else if err != nil && q.strict {
		return types.SearchResultPage{}, err
	}
	page, err := q.delegate.Query(ctx, req)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	if err := q.cache.Set(ctx, key, page, q.ttl); err != nil && q.strict {
		return types.SearchResultPage{}, err
	}
	return page, nil
}
