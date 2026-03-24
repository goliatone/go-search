package cache

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type CachedProviderMetadataConfig struct {
	Provider        providers.Provider
	CapabilityCache Store[types.CapabilitySet]
	HealthCache     Store[types.HealthStatus]
	CapabilityTTL   time.Duration
	HealthTTL       time.Duration
	Strict          bool
	Logger          types.Logger
	Metrics         []types.MetricsHook
}

type CachedProviderMetadata struct {
	provider        providers.Provider
	capabilityCache Store[types.CapabilitySet]
	healthCache     Store[types.HealthStatus]
	capabilityTTL   time.Duration
	healthTTL       time.Duration
	strict          bool
	observer        cacheObserver

	mu         sync.Mutex
	healthKeys map[string]struct{}
}

func NewCachedProviderMetadata(cfg CachedProviderMetadataConfig) (*CachedProviderMetadata, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("cached metadata provider is required", nil)
	}
	if cfg.CapabilityCache == nil && cfg.HealthCache == nil {
		return nil, errs.ConfigurationError("cached metadata requires at least one cache", nil)
	}
	if cfg.CapabilityTTL <= 0 {
		cfg.CapabilityTTL = 5 * time.Minute
	}
	if cfg.HealthTTL <= 0 {
		cfg.HealthTTL = 15 * time.Second
	}
	return &CachedProviderMetadata{
		provider:        cfg.Provider,
		capabilityCache: cfg.CapabilityCache,
		healthCache:     cfg.HealthCache,
		capabilityTTL:   cfg.CapabilityTTL,
		healthTTL:       cfg.HealthTTL,
		strict:          cfg.Strict,
		observer:        newCacheObserver(cfg.Logger, cfg.Metrics),
		healthKeys:      map[string]struct{}{},
	}, nil
}

func (p *CachedProviderMetadata) Name() string { return p.provider.Name() }

func (p *CachedProviderMetadata) Capabilities(ctx context.Context) (types.CapabilitySet, error) {
	if p.capabilityCache == nil {
		return p.provider.Capabilities(ctx)
	}
	key, err := metadataCacheKey(p.provider.Name(), "capabilities", nil)
	if err != nil {
		p.observer.bypass(ctx, "provider_capabilities", p.provider.Name(), "key_build_failed", nil)
		if p.strict {
			return types.CapabilitySet{}, err
		}
		return p.provider.Capabilities(ctx)
	}
	if cached, ok, err := p.capabilityCache.Get(ctx, key); err == nil && ok {
		p.observer.hit(ctx, "provider_capabilities", p.provider.Name(), nil)
		return cached, nil
	} else if err != nil && p.strict {
		p.observer.lookupFailure(ctx, "provider_capabilities", p.provider.Name(), err, nil)
		return types.CapabilitySet{}, err
	} else if err != nil {
		p.observer.lookupFailure(ctx, "provider_capabilities", p.provider.Name(), err, nil)
	} else {
		p.observer.miss(ctx, "provider_capabilities", p.provider.Name(), nil)
	}
	result, err := p.provider.Capabilities(ctx)
	if err != nil {
		return types.CapabilitySet{}, err
	}
	if err := p.capabilityCache.Set(ctx, key, result, p.capabilityTTL); err != nil {
		p.observer.writeFailure(ctx, "provider_capabilities", p.provider.Name(), err, nil)
		if p.strict {
			return types.CapabilitySet{}, err
		}
	}
	return result, nil
}

func (p *CachedProviderMetadata) EnsureIndex(ctx context.Context, def types.IndexDefinition) error {
	err := p.provider.EnsureIndex(ctx, def)
	if err == nil {
		p.invalidateHealth(ctx)
	}
	return err
}

func (p *CachedProviderMetadata) Search(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	return p.provider.Search(ctx, req)
}

func (p *CachedProviderMetadata) Suggest(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	return p.provider.Suggest(ctx, req)
}

func (p *CachedProviderMetadata) UpsertDocuments(ctx context.Context, index string, docs []types.Document) error {
	err := p.provider.UpsertDocuments(ctx, index, docs)
	if err == nil {
		p.invalidateHealth(ctx)
	}
	return err
}

func (p *CachedProviderMetadata) ReplaceDocuments(ctx context.Context, index, registrationKey string, sourceIDs []string, docs []types.Document) error {
	err := p.provider.ReplaceDocuments(ctx, index, registrationKey, sourceIDs, docs)
	if err == nil {
		p.invalidateHealth(ctx)
	}
	return err
}

func (p *CachedProviderMetadata) DeleteDocuments(ctx context.Context, index string, ids []string) error {
	err := p.provider.DeleteDocuments(ctx, index, ids)
	if err == nil {
		p.invalidateHealth(ctx)
	}
	return err
}

func (p *CachedProviderMetadata) DeleteBySource(ctx context.Context, index, registrationKey string, sourceIDs []string) error {
	err := p.provider.DeleteBySource(ctx, index, registrationKey, sourceIDs)
	if err == nil {
		p.invalidateHealth(ctx)
	}
	return err
}

func (p *CachedProviderMetadata) ResetRegistration(ctx context.Context, index, registrationKey string) error {
	resetter, ok := p.provider.(providers.RegistrationResetter)
	if !ok {
		return errs.ConfigurationError("cached metadata provider does not support registration reset", map[string]any{
			"index":            index,
			"registration_key": registrationKey,
		})
	}
	err := resetter.ResetRegistration(ctx, index, registrationKey)
	if err == nil {
		p.invalidateHealth(ctx)
	}
	return err
}

func (p *CachedProviderMetadata) Health(ctx context.Context, req types.HealthRequest) (types.HealthStatus, error) {
	if p.healthCache == nil {
		return p.provider.Health(ctx, req)
	}
	key, err := metadataCacheKey(p.provider.Name(), "health", req.Indexes)
	if err != nil {
		p.observer.bypass(ctx, "provider_health", p.provider.Name(), "key_build_failed", map[string]any{"indexes": normalizeIndexes(req.Indexes)})
		if p.strict {
			return types.HealthStatus{}, err
		}
		return p.provider.Health(ctx, req)
	}
	p.recordHealthKey(key)
	if cached, ok, err := p.healthCache.Get(ctx, key); err == nil && ok {
		p.observer.hit(ctx, "provider_health", p.provider.Name(), map[string]any{"indexes": normalizeIndexes(req.Indexes)})
		return cached, nil
	} else if err != nil && p.strict {
		p.observer.lookupFailure(ctx, "provider_health", p.provider.Name(), err, map[string]any{"indexes": normalizeIndexes(req.Indexes)})
		return types.HealthStatus{}, err
	} else if err != nil {
		p.observer.lookupFailure(ctx, "provider_health", p.provider.Name(), err, map[string]any{"indexes": normalizeIndexes(req.Indexes)})
	} else {
		p.observer.miss(ctx, "provider_health", p.provider.Name(), map[string]any{"indexes": normalizeIndexes(req.Indexes)})
	}
	result, err := p.provider.Health(ctx, req)
	if err != nil {
		return types.HealthStatus{}, err
	}
	if err := p.healthCache.Set(ctx, key, result, p.healthTTL); err != nil {
		p.observer.writeFailure(ctx, "provider_health", p.provider.Name(), err, map[string]any{"indexes": normalizeIndexes(req.Indexes)})
		if p.strict {
			return types.HealthStatus{}, err
		}
	}
	return result, nil
}

func (p *CachedProviderMetadata) SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
	if batcher, ok := p.provider.(providers.SearchBatcher); ok {
		return batcher.SearchBatch(ctx, requests)
	}
	out := make([]types.SearchResultPage, 0, len(requests))
	for _, req := range requests {
		page, err := p.provider.Search(ctx, req)
		if err != nil {
			return nil, err
		}
		out = append(out, page)
	}
	return out, nil
}

func (p *CachedProviderMetadata) recordHealthKey(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthKeys[key] = struct{}{}
}

func (p *CachedProviderMetadata) invalidateHealth(ctx context.Context) {
	if p.healthCache == nil {
		return
	}
	p.mu.Lock()
	keys := make([]string, 0, len(p.healthKeys))
	for key := range p.healthKeys {
		keys = append(keys, key)
	}
	p.healthKeys = map[string]struct{}{}
	p.mu.Unlock()
	for _, key := range keys {
		if err := p.healthCache.Delete(ctx, key); err != nil {
			p.observer.invalidateFailure(ctx, "provider_health", p.provider.Name(), err, nil)
		}
	}
}

func cacheBypassReason(metadata map[string]any) (string, bool) {
	if len(metadata) == 0 {
		return "", false
	}
	if reason, ok := metadata["cache_disabled_reason"].(string); ok && strings.TrimSpace(reason) != "" {
		return strings.TrimSpace(reason), true
	}
	disabled, _ := metadata["cache_disabled"].(bool)
	if disabled {
		return "request_disabled", true
	}
	return "", false
}

func actorSensitive(actor types.ActorRef) bool {
	if strings.TrimSpace(actor.UserID) != "" {
		return true
	}
	if strings.TrimSpace(actor.TenantID) != "" {
		return true
	}
	if strings.TrimSpace(actor.OrgID) != "" {
		return true
	}
	return len(actor.Metadata) > 0
}
