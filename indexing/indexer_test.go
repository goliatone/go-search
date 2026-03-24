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

type recordingActivityHook struct {
	events []types.ActivityEvent
}

func (h *recordingActivityHook) Notify(_ context.Context, event types.ActivityEvent) {
	h.events = append(h.events, event)
}

type activityUserRecord struct {
	ID       string
	TenantID string
	OrgID    string
}

type activityUserSource struct {
	record activityUserRecord
}

func (s activityUserSource) Get(context.Context, string) (activityUserRecord, error) {
	return s.record, nil
}

func (s activityUserSource) List(context.Context, int, string) ([]activityUserRecord, string, error) {
	return []activityUserRecord{s.record}, "", nil
}

type activityUserProjector struct{}

func (activityUserProjector) Project(_ context.Context, record activityUserRecord) ([]types.Document, error) {
	return []types.Document{{
		ID:         "user-doc-" + record.ID,
		Type:       "user",
		SourceType: "user",
		SourceID:   record.ID,
		Title:      "User " + record.ID,
		Scope: types.Scope{
			TenantID: record.TenantID,
			OrgID:    record.OrgID,
		},
		Fields: map[string]any{
			"user_id": record.ID,
		},
	}}, nil
}

type mutableTranscriptSource struct {
	record media.TranscriptRecord
}

func (s *mutableTranscriptSource) Get(context.Context, string) (media.TranscriptRecord, error) {
	return s.record, nil
}

func (s *mutableTranscriptSource) List(context.Context, int, string) ([]media.TranscriptRecord, string, error) {
	return []media.TranscriptRecord{s.record}, "", nil
}

type mutableTranscriptListSource struct {
	records map[string]media.TranscriptRecord
}

func (s *mutableTranscriptListSource) Get(_ context.Context, id string) (media.TranscriptRecord, error) {
	record, ok := s.records[id]
	if !ok {
		return media.TranscriptRecord{}, errors.New("record not found")
	}
	return record, nil
}

func (s *mutableTranscriptListSource) List(context.Context, int, string) ([]media.TranscriptRecord, string, error) {
	out := make([]media.TranscriptRecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	return out, "", nil
}

type sharedRegistrationRecord struct {
	ID    string
	Type  string
	Title string
	Body  string
}

type sharedRegistrationSource struct {
	record sharedRegistrationRecord
}

func (s sharedRegistrationSource) Get(context.Context, string) (sharedRegistrationRecord, error) {
	return s.record, nil
}

func (s sharedRegistrationSource) List(context.Context, int, string) ([]sharedRegistrationRecord, string, error) {
	return []sharedRegistrationRecord{s.record}, "", nil
}

type sharedRegistrationProjector struct {
	sourceType string
}

func (p sharedRegistrationProjector) Project(_ context.Context, record sharedRegistrationRecord) ([]types.Document, error) {
	return []types.Document{{
		ID:         record.ID,
		Type:       record.Type,
		SourceType: p.sourceType,
		SourceID:   record.ID,
		Title:      record.Title,
		Body:       record.Body,
		URL:        "/" + p.sourceType + "/" + record.ID,
		Locale:     "en",
	}}, nil
}

type aliasRecord struct {
	ID       string
	AliasID  string
	Title    string
	Body     string
	Locale   string
	SourceID string
}

type aliasSource struct {
	records map[string]aliasRecord
}

func (s aliasSource) Get(_ context.Context, id string) (aliasRecord, error) {
	record, ok := s.records[id]
	if !ok {
		return aliasRecord{}, errors.New("record not found")
	}
	return record, nil
}

func (s aliasSource) List(context.Context, int, string) ([]aliasRecord, string, error) {
	out := make([]aliasRecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	return out, "", nil
}

type aliasProjector struct{}

func (aliasProjector) Project(_ context.Context, record aliasRecord) ([]types.Document, error) {
	return []types.Document{{
		ID:    "doc-" + record.AliasID,
		Type:  types.DocumentTypeDocument,
		Title: record.Title,
		Body:  record.Body,
		URL:   "/docs/" + record.AliasID,
		Locale: func() string {
			if record.Locale != "" {
				return record.Locale
			}
			return "en"
		}(),
	}}, nil
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

func TestIndexerReindexRemovesStaleDerivedDocuments(t *testing.T) {
	recordA := testkit.SampleTranscriptRecord()
	recordB := testkit.SampleTranscriptRecord()
	recordB.ID = "track-second"
	recordB.Media.ID = "video-second"
	recordB.Content = "1\n00:00:01,000 --> 00:00:02,500\nmountain bells\n"
	source := &mutableTranscriptListSource{
		records: map[string]media.TranscriptRecord{
			recordA.ID: recordA,
			recordB.ID: recordB,
		},
	}
	projector := media.NewTranscriptProjector(media.TranscriptProjectorConfig{
		Index:      "media",
		SourceType: "transcript",
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
	if err := indexer.ReindexIndex(context.Background(), "media", "", 10); err != nil {
		t.Fatalf("initial reindex: %v", err)
	}
	source.records = map[string]media.TranscriptRecord{recordB.ID: recordB}
	if err := indexer.ReindexIndex(context.Background(), "media", "", 10); err != nil {
		t.Fatalf("second reindex: %v", err)
	}
	page, err := provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search stale content: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("expected stale removed record to disappear after reindex, got %+v", page.Hits)
	}
	page, err = provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "mountain",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search surviving content: %v", err)
	}
	if page.Total == 0 {
		t.Fatalf("expected surviving record after convergent reindex")
	}
}

func TestIndexerDeleteRecordResolvesCanonicalSourceID(t *testing.T) {
	source := aliasSource{records: map[string]aliasRecord{
		"db-1": {
			ID:      "db-1",
			AliasID: "public-1",
			Title:   "Ocean Workbook",
			Body:    "ocean prayer notes",
			Locale:  "en",
		},
	}}
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "documents"}
	reg := NewRegistration("documents", def, "document", source, aliasProjector{}, func(r aliasRecord) string { return r.AliasID })
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
	if _, err := indexer.IndexRecord(context.Background(), "documents", "", "db-1"); err != nil {
		t.Fatalf("index record: %v", err)
	}
	if err := indexer.DeleteRecord(context.Background(), "documents", "", "db-1"); err != nil {
		t.Fatalf("delete record: %v", err)
	}
	page, err := provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{"documents"},
		Query:   "ocean",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("expected canonical source-id delete to remove document, got %+v", page.Hits)
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

func TestIndexerEmitsScopedRecordActivityContext(t *testing.T) {
	record := activityUserRecord{
		ID:       "00000000-0000-0000-0000-000000000111",
		TenantID: "00000000-0000-0000-0000-000000000211",
		OrgID:    "00000000-0000-0000-0000-000000000311",
	}
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "users"}
	reg := NewRegistration("users", def, "user", activityUserSource{record: record}, activityUserProjector{}, func(r activityUserRecord) string { return r.ID })
	if err := registry.Register(def, reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	hook := &recordingActivityHook{}
	indexer, err := NewIndexer(IndexerConfig{Registry: registry, Provider: provider, Activities: []types.ActivityHook{hook}})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}

	if _, err := indexer.IndexRecord(context.Background(), "users", "", record.ID); err != nil {
		t.Fatalf("index record: %v", err)
	}
	if err := indexer.DeleteRecord(context.Background(), "users", "", record.ID); err != nil {
		t.Fatalf("delete record: %v", err)
	}
	if len(hook.events) != 2 {
		t.Fatalf("events = %#v", hook.events)
	}
	for _, event := range hook.events {
		if event.RecordID != record.ID {
			t.Fatalf("record id = %#v", event)
		}
		if event.TenantID != record.TenantID || event.OrgID != record.OrgID {
			t.Fatalf("scope context = %#v", event)
		}
		if event.ObjectType != "user" || event.ObjectID != record.ID {
			t.Fatalf("object context = %#v", event)
		}
		if event.Metadata["user_id"] != record.ID {
			t.Fatalf("metadata = %#v", event.Metadata)
		}
	}
}

func TestIndexerScopesSharedIndexReplacementAndDeleteByRegistrationKey(t *testing.T) {
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "content_shared"}
	videoRecord := sharedRegistrationRecord{
		ID:    "shared-1",
		Type:  types.DocumentTypeVideo,
		Title: "Shared Video",
		Body:  "architecture video",
	}
	documentRecord := sharedRegistrationRecord{
		ID:    "shared-1",
		Type:  types.DocumentTypeDocument,
		Title: "Shared Document",
		Body:  "architecture workbook",
	}
	videoReg := NewRegistrationWithKey(
		def.Name,
		def,
		"video",
		"video",
		sharedRegistrationSource{record: videoRecord},
		sharedRegistrationProjector{sourceType: "video"},
		func(record sharedRegistrationRecord) string { return record.ID },
	)
	documentReg := NewRegistrationWithKey(
		def.Name,
		def,
		"document",
		"document",
		sharedRegistrationSource{record: documentRecord},
		sharedRegistrationProjector{sourceType: "document"},
		func(record sharedRegistrationRecord) string { return record.ID },
	)
	if err := registry.Register(def, videoReg); err != nil {
		t.Fatalf("register video: %v", err)
	}
	if err := registry.Register(def, documentReg); err != nil {
		t.Fatalf("register document: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	indexer, err := NewIndexer(IndexerConfig{Registry: registry, Provider: provider})
	if err != nil {
		t.Fatalf("new indexer: %v", err)
	}
	if _, err := indexer.IndexRecord(context.Background(), def.Name, "video", videoRecord.ID); err != nil {
		t.Fatalf("index video: %v", err)
	}
	if _, err := indexer.IndexRecord(context.Background(), def.Name, "document", documentRecord.ID); err != nil {
		t.Fatalf("index document: %v", err)
	}
	page, err := provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{def.Name},
		Query:   "architecture",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search shared: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected both registrations to survive shared ID collision, got %+v", page.Hits)
	}
	if err := indexer.DeleteRecord(context.Background(), def.Name, "video", videoRecord.ID); err != nil {
		t.Fatalf("delete video: %v", err)
	}
	page, err = provider.Search(context.Background(), types.SearchRequest{
		Indexes: []string{def.Name},
		Query:   "architecture",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if page.Total != 1 || len(page.Hits) != 1 || page.Hits[0].Type != types.DocumentTypeDocument {
		t.Fatalf("expected document registration to remain after scoped delete, got %+v", page.Hits)
	}
}
