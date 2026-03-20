package indexing

import (
	"context"
	"time"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type Indexer struct {
	registry   *Registry
	provider   providers.Provider
	progress   types.ProgressReporter
	activities []types.ActivityHook
	metrics    []types.MetricsHook
}

type IndexerConfig struct {
	Registry   *Registry
	Provider   providers.Provider
	Progress   types.ProgressReporter
	Activities []types.ActivityHook
	Metrics    []types.MetricsHook
}

func NewIndexer(cfg IndexerConfig) (*Indexer, error) {
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("index registry is required", nil)
	}
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	return &Indexer{
		registry:   cfg.Registry,
		provider:   cfg.Provider,
		progress:   cfg.Progress,
		activities: cfg.Activities,
		metrics:    cfg.Metrics,
	}, nil
}

func (i *Indexer) IndexRecord(ctx context.Context, index, recordID string) ([]types.Document, error) {
	indexer, err := i.registry.MustIndexer(index)
	if err != nil {
		return nil, err
	}
	docs, err := indexer.IndexRecord(ctx, recordID)
	if err != nil {
		return nil, errs.ProjectorFailure(err, map[string]any{"index": index, "record_id": recordID})
	}
	if err := i.provider.UpsertDocuments(ctx, index, docs); err != nil {
		return nil, err
	}
	i.emitActivity(ctx, "indexed", indexer.SourceType(), recordID, map[string]any{"documents": len(docs), "index": index})
	return docs, nil
}

func (i *Indexer) DeleteRecord(ctx context.Context, index, recordID string) error {
	indexer, err := i.registry.MustIndexer(index)
	if err != nil {
		return err
	}
	sourceIDs, err := indexer.DeleteSourceIDs(ctx, recordID)
	if err != nil {
		return err
	}
	if err := i.provider.DeleteBySource(ctx, index, sourceIDs); err != nil {
		return err
	}
	i.emitActivity(ctx, "deleted", indexer.SourceType(), recordID, map[string]any{"index": index})
	return nil
}

func (i *Indexer) ReindexIndex(ctx context.Context, index string, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 50
	}
	indexer, err := i.registry.MustIndexer(index)
	if err != nil {
		return err
	}
	cursor := ""
	total := 0
	for {
		ids, next, err := indexer.ListRecordIDs(ctx, batchSize, cursor)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := i.IndexRecord(ctx, index, id); err != nil {
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
	i.emitActivity(ctx, "reindexed", indexer.SourceType(), index, map[string]any{"index": index, "documents": total})
	return nil
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
		OccurredAt: time.Now().UnixMilli(),
		Metadata:   metadata,
	}
	for _, hook := range i.activities {
		hook.Notify(ctx, event)
	}
}
