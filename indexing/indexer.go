package indexing

import (
	"context"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type Indexer struct {
	registry         *Registry
	provider         providers.Provider
	generationStore  types.GenerationStore
	progress         types.ProgressReporter
	activities       []types.ActivityHook
	metrics          []types.MetricsHook
	logger           types.Logger
	defaultBatchSize int
	clock            types.Clock
}

type IndexerConfig struct {
	Registry         *Registry
	Provider         providers.Provider
	GenerationStore  types.GenerationStore
	Progress         types.ProgressReporter
	Activities       []types.ActivityHook
	Metrics          []types.MetricsHook
	Logger           types.Logger
	DefaultBatchSize int
	Clock            types.Clock
}

func NewIndexer(cfg IndexerConfig) (*Indexer, error) {
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("index registry is required", nil)
	}
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.DefaultBatchSize <= 0 {
		cfg.DefaultBatchSize = 50
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &Indexer{
		registry:         cfg.Registry,
		provider:         cfg.Provider,
		generationStore:  cfg.GenerationStore,
		progress:         cfg.Progress,
		activities:       cfg.Activities,
		metrics:          cfg.Metrics,
		logger:           cfg.Logger,
		defaultBatchSize: cfg.DefaultBatchSize,
		clock:            cfg.Clock,
	}, nil
}

func (i *Indexer) IndexRecord(ctx context.Context, index, registrationKey, recordID string) ([]types.Document, error) {
	return i.indexRecord(ctx, index, registrationKey, recordID, true, true)
}

func (i *Indexer) DeleteRecord(ctx context.Context, index, registrationKey, recordID string) error {
	return i.deleteRecord(ctx, index, registrationKey, recordID, true, true)
}

func (i *Indexer) ReindexIndex(ctx context.Context, index, registrationKey string, batchSize int) error {
	startedAt := i.clock.Now()
	if batchSize <= 0 {
		batchSize = i.defaultBatchSize
	}
	registrations := i.registry.ListRegistrations(index)
	if len(registrations) == 0 {
		return errs.IndexingSourceMissing(index, nil)
	}
	if registrationKey != "" {
		registration, err := i.registry.ResolveRegistration(index, registrationKey)
		if err != nil {
			return err
		}
		registrations = []RegisteredSource{registration}
	}
	total := 0
	for _, registration := range registrations {
		cursor := ""
		for {
			ids, next, err := registration.Indexer.ListRecordIDs(ctx, batchSize, cursor)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if _, err := i.indexRecord(ctx, index, registration.RegistrationKey, id, false, false); err != nil {
					return err
				}
				total++
				if i.progress != nil {
					i.progress.Report(ctx, types.ProgressUpdate{Index: index, Completed: total, Message: "reindexed"})
				}
			}
			if next == "" || len(ids) == 0 {
				break
			}
			cursor = next
		}
	}
	if err := i.bumpGeneration(ctx, index); err != nil {
		observe.Count(ctx, i.metrics, i.logger, "search.reindex.error.count", 1, map[string]string{"index": index})
		return err
	}
	objectType := "index"
	if len(registrations) == 1 {
		objectType = registrations[0].SourceType
	}
	metadata := map[string]any{"index": index, "documents": total}
	if registrationKey != "" {
		metadata["registration_key"] = registrationKey
	}
	i.emitActivity(ctx, "reindexed", objectType, index, metadata)
	observe.Count(ctx, i.metrics, i.logger, "search.reindex.count", int64(total), map[string]string{"index": index})
	observe.ObserveDuration(ctx, i.metrics, i.logger, "search.reindex.duration_ms", startedAt, map[string]string{"index": index})
	return nil
}

func (i *Indexer) indexRecord(ctx context.Context, index, registrationKey, recordID string, emitActivity bool, bumpGeneration bool) ([]types.Document, error) {
	registration, err := i.registry.ResolveRegistration(index, registrationKey)
	if err != nil {
		return nil, err
	}
	docs, err := registration.Indexer.IndexRecord(ctx, recordID)
	if err != nil {
		return nil, errs.ProjectorFailure(err, map[string]any{"index": index, "record_id": recordID})
	}
	sourceIDs, err := registration.Indexer.DeleteSourceIDs(ctx, recordID)
	if err != nil {
		return nil, err
	}
	if err := i.provider.ReplaceDocuments(ctx, index, sourceIDs, docs); err != nil {
		observe.Count(ctx, i.metrics, i.logger, "search.index_record.error.count", 1, map[string]string{"index": index})
		return nil, err
	}
	if bumpGeneration {
		if err := i.bumpGeneration(ctx, index); err != nil {
			return nil, err
		}
	}
	if emitActivity {
		metadata := map[string]any{"documents": len(docs), "index": index}
		if registration.RegistrationKey != "" {
			metadata["registration_key"] = registration.RegistrationKey
		}
		i.emitActivity(ctx, "indexed", registration.Indexer.SourceType(), recordID, metadata)
	}
	observe.Count(ctx, i.metrics, i.logger, "search.index_record.count", int64(len(docs)), map[string]string{"index": index})
	return docs, nil
}

func (i *Indexer) deleteRecord(ctx context.Context, index, registrationKey, recordID string, emitActivity bool, bumpGeneration bool) error {
	registration, err := i.registry.ResolveRegistration(index, registrationKey)
	if err != nil {
		return err
	}
	sourceIDs, err := registration.Indexer.DeleteSourceIDs(ctx, recordID)
	if err != nil {
		return err
	}
	if err := i.provider.DeleteBySource(ctx, index, sourceIDs); err != nil {
		observe.Count(ctx, i.metrics, i.logger, "search.delete_record.error.count", 1, map[string]string{"index": index})
		return err
	}
	if bumpGeneration {
		if err := i.bumpGeneration(ctx, index); err != nil {
			return err
		}
	}
	if emitActivity {
		metadata := map[string]any{"index": index}
		if registration.RegistrationKey != "" {
			metadata["registration_key"] = registration.RegistrationKey
		}
		i.emitActivity(ctx, "deleted", registration.Indexer.SourceType(), recordID, metadata)
	}
	observe.Count(ctx, i.metrics, i.logger, "search.delete_record.count", 1, map[string]string{"index": index})
	return nil
}

func (i *Indexer) bumpGeneration(ctx context.Context, index string) error {
	if i.generationStore == nil || index == "" {
		return nil
	}
	_, err := i.generationStore.Bump(ctx, index)
	return err
}

func (i *Indexer) emitActivity(ctx context.Context, verb, objectType, objectID string, metadata map[string]any) {
	if len(i.activities) == 0 {
		return
	}
	event := types.ActivityEvent{
		Channel:    "search",
		Verb:       verb,
		ObjectType: objectType,
		ObjectID:   objectID,
		OccurredAt: i.clock.Now().UnixMilli(),
		Metadata:   metadata,
	}
	observe.NotifyActivities(ctx, i.activities, i.logger, event)
}
