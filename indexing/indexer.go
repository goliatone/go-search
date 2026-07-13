package indexing

import (
	"context"
	"maps"

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
		ids, err := listRegistrationRecordIDs(ctx, registration.Indexer, batchSize)
		if err != nil {
			return err
		}
		if err := i.resetRegistration(ctx, index, registration.RegistrationKey); err != nil {
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
	i.emitActivity(ctx, types.ActivityEvent{
		Channel:    "search",
		Verb:       "reindexed",
		ObjectType: objectType,
		ObjectID:   index,
		Metadata:   metadata,
	})
	observe.Count(ctx, i.metrics, i.logger, "search.reindex.count", int64(total), map[string]string{"index": index})
	observe.ObserveDuration(ctx, i.metrics, i.logger, "search.reindex.duration_ms", startedAt, map[string]string{"index": index})
	return nil
}

func listRegistrationRecordIDs(ctx context.Context, indexer RecordIndexer, batchSize int) ([]string, error) {
	if indexer == nil {
		return nil, nil
	}
	cursor := ""
	out := []string{}
	for {
		ids, next, err := indexer.ListRecordIDs(ctx, batchSize, cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, ids...)
		if next == "" || len(ids) == 0 {
			return out, nil
		}
		cursor = next
	}
}

func (i *Indexer) resetRegistration(ctx context.Context, index, registrationKey string) error {
	resetter, ok := i.provider.(providers.RegistrationResetter)
	if !ok {
		return errs.ConfigurationError("provider does not support convergent registration reset required by reindex", map[string]any{
			"index":            index,
			"provider":         i.provider.Name(),
			"registration_key": registrationKey,
		})
	}
	return resetter.ResetRegistration(ctx, index, registrationKey)
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
	for i := range docs {
		docs[i].RegistrationKey = registration.RegistrationKey
	}
	sourceIDs, err := registration.Indexer.DeleteSourceIDs(ctx, recordID)
	if err != nil {
		return nil, err
	}
	var replaceErr error
	if recordAware, ok := i.provider.(providers.RecordReplacementProvider); ok {
		replaceErr = recordAware.ReplaceRecordDocuments(ctx, index, registration.RegistrationKey, recordID, sourceIDs, docs)
	} else {
		replaceErr = i.provider.ReplaceDocuments(ctx, index, registration.RegistrationKey, sourceIDs, docs)
	}
	if replaceErr != nil {
		observe.Count(ctx, i.metrics, i.logger, "search.index_record.error.count", 1, map[string]string{"index": index})
		return nil, replaceErr
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
		i.emitActivity(ctx, i.recordActivityEvent(ctx, registration, "indexed", recordID, docs, metadata))
	}
	observe.Count(ctx, i.metrics, i.logger, "search.index_record.count", int64(len(docs)), map[string]string{"index": index})
	return docs, nil
}

func (i *Indexer) deleteRecord(ctx context.Context, index, registrationKey, recordID string, emitActivity bool, bumpGeneration bool) error {
	registration, err := i.registry.ResolveRegistration(index, registrationKey)
	if err != nil {
		return err
	}
	var activity types.ActivityEvent
	if emitActivity {
		metadata := map[string]any{"index": index}
		if registration.RegistrationKey != "" {
			metadata["registration_key"] = registration.RegistrationKey
		}
		activity = i.recordActivityEvent(ctx, registration, "deleted", recordID, nil, metadata)
	}
	sourceIDs, err := registration.Indexer.DeleteSourceIDs(ctx, recordID)
	if err != nil {
		return err
	}
	if err := i.provider.DeleteBySource(ctx, index, registration.RegistrationKey, sourceIDs); err != nil {
		observe.Count(ctx, i.metrics, i.logger, "search.delete_record.error.count", 1, map[string]string{"index": index})
		return err
	}
	if bumpGeneration {
		if err := i.bumpGeneration(ctx, index); err != nil {
			return err
		}
	}
	if emitActivity {
		i.emitActivity(ctx, activity)
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

func (i *Indexer) emitActivity(ctx context.Context, event types.ActivityEvent) {
	if len(i.activities) == 0 {
		return
	}
	if event.Channel == "" {
		event.Channel = "search"
	}
	if event.OccurredAt <= 0 {
		event.OccurredAt = i.clock.Now().UnixMilli()
	}
	observe.NotifyActivities(ctx, i.activities, i.logger, event)
}

func (i *Indexer) recordActivityEvent(ctx context.Context, registration RegisteredSource, verb, recordID string, docs []types.Document, metadata map[string]any) types.ActivityEvent {
	event := types.ActivityEvent{
		Channel:    "search",
		Verb:       verb,
		ObjectType: registration.Indexer.SourceType(),
		ObjectID:   recordID,
		RecordID:   recordID,
		Metadata:   cloneActivityMetadata(metadata),
	}
	if len(event.Metadata) == 0 {
		event.Metadata = map[string]any{}
	}
	event.Metadata["record_id"] = recordID
	if resolver, ok := registration.Indexer.(ActivityEventResolver); ok {
		resolved, err := resolver.ResolveActivityEvent(ctx, verb, recordID, docs, event.Metadata)
		if err != nil {
			observe.Warn(i.logger, "search.activity_context_resolver_failed", map[string]any{
				"index":            registration.Index,
				"registration_key": registration.RegistrationKey,
				"record_id":        recordID,
				"message":          err.Error(),
			})
		} else {
			event = mergeActivityEvent(event, resolved)
		}
	}
	return event
}

func mergeActivityEvent(base, resolved types.ActivityEvent) types.ActivityEvent {
	if resolved.Channel != "" {
		base.Channel = resolved.Channel
	}
	if resolved.Verb != "" {
		base.Verb = resolved.Verb
	}
	if resolved.ObjectType != "" {
		base.ObjectType = resolved.ObjectType
	}
	if resolved.ObjectID != "" {
		base.ObjectID = resolved.ObjectID
	}
	if resolved.RecordID != "" {
		base.RecordID = resolved.RecordID
	}
	if resolved.ActorID != "" {
		base.ActorID = resolved.ActorID
	}
	if resolved.TenantID != "" {
		base.TenantID = resolved.TenantID
	}
	if resolved.OrgID != "" {
		base.OrgID = resolved.OrgID
	}
	if resolved.OccurredAt > 0 {
		base.OccurredAt = resolved.OccurredAt
	}
	if len(resolved.Metadata) > 0 {
		if base.Metadata == nil {
			base.Metadata = map[string]any{}
		}
		maps.Copy(base.Metadata, resolved.Metadata)
	}
	return base
}

func cloneActivityMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	maps.Copy(out, metadata)
	return out
}
