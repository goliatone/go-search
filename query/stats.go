package query

import (
	"context"
	"time"

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
}

type Stats struct {
	provider        providers.Provider
	registry        *indexing.Registry
	generationStore types.GenerationStore
}

var _ gcommand.Querier[types.StatsRequest, types.StatsResult] = (*Stats)(nil)

func NewStats(cfg StatsConfig) (*Stats, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("registry is required", nil)
	}
	return &Stats{provider: cfg.Provider, registry: cfg.Registry, generationStore: cfg.GenerationStore}, nil
}

func (q *Stats) Query(ctx context.Context, _ types.StatsRequest) (types.StatsResult, error) {
	caps, err := q.provider.Capabilities(ctx)
	if err != nil {
		return types.StatsResult{}, err
	}
	health, err := q.provider.Health(ctx)
	if err != nil {
		return types.StatsResult{}, err
	}
	out := types.StatsResult{
		Provider:     q.provider.Name(),
		Capabilities: caps,
		Indexes:      make([]types.IndexStats, 0, len(q.registry.ListIndexes())),
		Metadata:     map[string]any{"generated_at": time.Now().UnixMilli()},
	}
	healthDocs := map[string]int{}
	for _, idx := range health.Indexes {
		healthDocs[idx.Name] = idx.Documents
	}
	for _, def := range q.registry.ListIndexes() {
		generation := int64(0)
		if q.generationStore != nil {
			generation, _ = q.generationStore.Get(ctx, def.Name)
		}
		out.Indexes = append(out.Indexes, types.IndexStats{
			Name:           def.Name,
			Documents:      healthDocs[def.Name],
			Generation:     generation,
			Registered:     true,
			ProviderStatus: "healthy",
		})
	}
	return out, nil
}
