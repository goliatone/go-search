package query

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers/memory"
)

type queryGenerationStore struct {
	values map[string]int64
}

func (s queryGenerationStore) Get(_ context.Context, index string) (int64, error) {
	return s.values[index], nil
}

func (s queryGenerationStore) Bump(context.Context, string) (int64, error) {
	return 0, nil
}

func TestHealthAndStatsRespectRequestedIndexes(t *testing.T) {
	registry := indexing.NewRegistry()
	for _, def := range []types.IndexDefinition{{Name: "media"}, {Name: "articles"}} {
		if err := registry.Register(def, nil); err != nil {
			t.Fatalf("register index: %v", err)
		}
	}
	provider := memory.New(memory.Config{})
	for _, def := range []types.IndexDefinition{{Name: "media"}, {Name: "articles"}, {Name: "external"}} {
		if err := provider.EnsureIndex(context.Background(), def); err != nil {
			t.Fatalf("ensure index: %v", err)
		}
	}
	healthQuery, err := NewHealth(HealthConfig{Provider: provider})
	if err != nil {
		t.Fatalf("new health query: %v", err)
	}
	health, err := healthQuery.Query(context.Background(), types.HealthRequest{Indexes: []string{"media"}})
	if err != nil {
		t.Fatalf("health query: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Name != "media" {
		t.Fatalf("expected only requested health index, got %+v", health.Indexes)
	}
	statsQuery, err := NewStats(StatsConfig{
		Provider:        provider,
		Registry:        registry,
		GenerationStore: queryGenerationStore{values: map[string]int64{"media": 3}},
	})
	if err != nil {
		t.Fatalf("new stats query: %v", err)
	}
	stats, err := statsQuery.Query(context.Background(), types.StatsRequest{Indexes: []string{"media", "external"}})
	if err != nil {
		t.Fatalf("stats query: %v", err)
	}
	if len(stats.Indexes) != 2 {
		t.Fatalf("expected two requested indexes in stats, got %+v", stats.Indexes)
	}
	if stats.Indexes[0].Name != "media" || !stats.Indexes[0].Registered || stats.Indexes[0].Generation != 3 {
		t.Fatalf("expected registered media stats with generation, got %+v", stats.Indexes[0])
	}
	if stats.Indexes[1].Name != "external" || stats.Indexes[1].Registered || stats.Indexes[1].ProviderStatus != "ready" {
		t.Fatalf("expected provider-only external stats, got %+v", stats.Indexes[1])
	}
}
