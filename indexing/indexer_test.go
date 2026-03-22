package indexing

import (
	"context"
	"errors"
	"testing"

	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/internal/testkit"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers/memory"
)

type mutableTranscriptSource struct {
	record media.TranscriptRecord
}

func (s *mutableTranscriptSource) Get(context.Context, string) (media.TranscriptRecord, error) {
	return s.record, nil
}

func (s *mutableTranscriptSource) List(context.Context, int, string) ([]media.TranscriptRecord, string, error) {
	return []media.TranscriptRecord{s.record}, "", nil
}

type stubGenerationStore struct {
	bumps int
	err   error
}

func (s *stubGenerationStore) Get(context.Context, string) (int64, error) {
	return int64(s.bumps), s.err
}

func (s *stubGenerationStore) Bump(context.Context, string) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	s.bumps++
	return int64(s.bumps), nil
}

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
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	indexer, err := NewIndexer(IndexerConfig{Registry: registry, Provider: provider})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}
	if _, err := indexer.IndexRecord(context.Background(), "media", "", record.ID); err != nil {
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
	if err := indexer.DeleteRecord(context.Background(), "media", "", record.ID); err != nil {
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
	health, err := provider.Health(context.Background(), types.HealthRequest{})
	if err != nil {
		t.Fatalf("provider health: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Documents != 0 {
		t.Fatalf("expected delete-by-source to remove all derived documents, got %+v", health.Indexes)
	}
	if err := indexer.ReindexIndex(context.Background(), "media", "", 10); err != nil {
		t.Fatalf("reindex index: %v", err)
	}
}

func TestIndexerReplacesDerivedDocumentsForRecord(t *testing.T) {
	record := testkit.SampleTranscriptRecord()
	source := &mutableTranscriptSource{record: record}
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:        "media",
		SourceType:   "transcript",
		MergeVersion: "v1",
		MaxChars:     80,
		MaxGapMS:     600,
	})
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	reg := NewRegistration("media", def, "transcript", source, projector, func(r media.TranscriptRecord) string { return r.ID })
	if err := registry.Register(def, reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	indexer, err := NewIndexer(IndexerConfig{Registry: registry, Provider: provider})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}
	if _, err := indexer.IndexRecord(context.Background(), "media", "", record.ID); err != nil {
		t.Fatalf("first index: %v", err)
	}
	source.record.Content = `1
00:00:01,000 --> 00:00:02,500
ocean wind

2
00:00:10,000 --> 00:00:11,500
harbor bells
`
	if _, err := indexer.IndexRecord(context.Background(), "media", "", record.ID); err != nil {
		t.Fatalf("second index: %v", err)
	}
	page, err := provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search stale query: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("expected stale derived docs to be replaced, got %+v", page.Hits)
	}
	health, err := provider.Health(context.Background(), types.HealthRequest{})
	if err != nil {
		t.Fatalf("provider health: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Documents != 2 {
		t.Fatalf("expected only current derived documents after replacement, got %+v", health.Indexes)
	}
}

func TestIndexerReturnsGenerationBumpFailure(t *testing.T) {
	record := testkit.SampleTranscriptRecord()
	source := media.NewTranscriptSource([]media.TranscriptRecord{record})
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:      "media",
		SourceType: "transcript",
	})
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	reg := NewRegistration("media", def, "transcript", source, projector, func(r media.TranscriptRecord) string { return r.ID })
	if err := registry.Register(def, reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	indexer, err := NewIndexer(IndexerConfig{
		Registry:        registry,
		Provider:        provider,
		GenerationStore: &stubGenerationStore{err: errors.New("boom")},
	})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}
	if _, err := indexer.IndexRecord(context.Background(), "media", "", record.ID); err == nil {
		t.Fatalf("expected generation bump failure to surface")
	}
}

func TestIndexerRequiresRegistrationKeyWhenIndexHasMultipleRegistrations(t *testing.T) {
	record := testkit.SampleTranscriptRecord()
	source := media.NewTranscriptSource([]media.TranscriptRecord{record})
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:      "content",
		SourceType: "video",
	})
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "content"}
	videoReg := NewRegistrationWithKey("content", def, "video", "video", source, projector, func(r media.TranscriptRecord) string { return r.ID })
	blogReg := NewRegistrationWithKey("content", def, "blog_article", "blog_article", source, projector, func(r media.TranscriptRecord) string { return r.ID })
	if err := registry.Register(def, videoReg); err != nil {
		t.Fatalf("register video: %v", err)
	}
	if err := registry.Register(def, blogReg); err != nil {
		t.Fatalf("register blog: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	indexer, err := NewIndexer(IndexerConfig{Registry: registry, Provider: provider})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}
	if _, err := indexer.IndexRecord(context.Background(), "content", "", record.ID); err == nil {
		t.Fatalf("expected registration selection error")
	}
	if _, err := indexer.IndexRecord(context.Background(), "content", "video", record.ID); err != nil {
		t.Fatalf("index record with registration key: %v", err)
	}
}
