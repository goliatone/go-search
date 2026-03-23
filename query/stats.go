package query

import (
	"context"
	"maps"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type StatsConfig struct {
	Provider        providers.Provider
	Registry        *indexing.Registry
	GenerationStore types.GenerationStore
	Clock           types.Clock
}

type Stats struct {
	provider        providers.Provider
	registry        *indexing.Registry
	generationStore types.GenerationStore
	clock           types.Clock
}

var _ gcommand.Querier[types.StatsRequest, types.StatsResult] = (*Stats)(nil)

func NewStats(cfg StatsConfig) (*Stats, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("registry is required", nil)
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &Stats{
		provider:        cfg.Provider,
		registry:        cfg.Registry,
		generationStore: cfg.GenerationStore,
		clock:           cfg.Clock,
	}, nil
}

func (q *Stats) Query(ctx context.Context, req types.StatsRequest) (types.StatsResult, error) {
	caps, err := q.provider.Capabilities(ctx)
	if err != nil {
		return types.StatsResult{}, err
	}
	health, err := q.provider.Health(ctx, types.HealthRequest(req))
	if err != nil {
		return types.StatsResult{}, err
	}
	indexNames := resolveStatsIndexes(req.Indexes, q.registry.ListIndexes(), health.Indexes)
	out := types.StatsResult{
		Provider:     q.provider.Name(),
		Capabilities: caps,
		Indexes:      make([]types.IndexStats, 0, len(indexNames)),
		Metadata:     map[string]any{"generated_at": q.clock.Now().UnixMilli()},
	}
	registryIndexes := map[string]types.IndexDefinition{}
	for _, def := range q.registry.ListIndexes() {
		registryIndexes[def.Name] = def
	}
	healthIndexes := map[string]types.IndexHealth{}
	for _, idx := range health.Indexes {
		healthIndexes[idx.Name] = idx
	}
	for _, name := range indexNames {
		generation := int64(0)
		if q.generationStore != nil {
			if _, ok := registryIndexes[name]; ok {
				generation, err = q.generationStore.Get(ctx, name)
				if err != nil {
					return types.StatsResult{}, errs.Wrap(err, map[string]any{
						"index":  name,
						"source": "generation_store",
					})
				}
			}
		}
		indexHealth, hasHealth := healthIndexes[name]
		providerStatus := "unknown"
		metadata := map[string]any{}
		if hasHealth {
			providerStatus = "ready"
			if !indexHealth.Ready {
				providerStatus = "not_ready"
			}
			if indexHealth.Message != "" {
				metadata["provider_message"] = indexHealth.Message
			}
			maps.Copy(metadata, indexHealth.Metadata)
		}
		out.Indexes = append(out.Indexes, types.IndexStats{
			Name:           name,
			Documents:      indexHealth.Documents,
			Generation:     generation,
			Registered:     registryIndexes[name].Name != "",
			ProviderStatus: providerStatus,
			Metadata:       metadataOrNil(metadata),
		})
	}
	return out, nil
}

func resolveStatsIndexes(requested []string, registered []types.IndexDefinition, health []types.IndexHealth) []string {
	if len(requested) > 0 {
		return append([]string(nil), requested...)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(registered)+len(health))
	for _, def := range registered {
		if _, ok := seen[def.Name]; ok {
			continue
		}
		seen[def.Name] = struct{}{}
		out = append(out, def.Name)
	}
	for _, idx := range health {
		if _, ok := seen[idx.Name]; ok {
			continue
		}
		seen[idx.Name] = struct{}{}
		out = append(out, idx.Name)
	}
	return out
}

func metadataOrNil(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
