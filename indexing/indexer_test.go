package indexing

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers/memory"
)

func TestIndexerIndexDeleteAndReindex(t *testing.T) {
	record := testkit.SampleTranscriptRecord()
	source := media.NewTranscriptSource([]media.TranscriptRecord{record})
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:        "media",
		SourceType:   "transcript",
		MergeVersion: "v1",
	})
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	reg := NewRegistration("media", def, "transcript", source, projector, func(r media.TranscriptRecord) string { return r.ID })
	if err := registry.Register(def, reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	provider := memory.New()
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	indexer, err := NewIndexer(IndexerConfig{Registry: registry, Provider: provider})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}
	if _, err := indexer.IndexRecord(context.Background(), "media", record.ID); err != nil {
		t.Fatalf("index record: %v", err)
	}
	page, err := provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Page:    1,
		PerPage: 10,
		GroupBy: "parent_id",
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if page.Total == 0 {
		t.Fatalf("expected indexed results")
	}
	if err := indexer.DeleteRecord(context.Background(), "media", record.ID); err != nil {
		t.Fatalf("delete record: %v", err)
	}
	page, err = provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("expected zero results after delete")
	}
	health, err := provider.Health(context.Background())
	if err != nil {
		t.Fatalf("provider health: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Documents != 0 {
		t.Fatalf("expected delete-by-source to remove all derived documents, got %+v", health.Indexes)
	}
	if err := indexer.ReindexIndex(context.Background(), "media", 10); err != nil {
		t.Fatalf("reindex index: %v", err)
	}
}
