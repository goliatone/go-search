package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	gcommand "github.com/goliatone/go-command"
	job "github.com/goliatone/go-job"
	queuecmd "github.com/goliatone/go-job/queue/command"
	"github.com/goliatone/go-search/command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type CommandSet struct {
	Indexer      *indexing.Indexer
	IndexRecord  *command.IndexRecord
	DeleteRecord *command.DeleteRecord
	ReindexIndex *command.ReindexIndex
	Registry     *queuecmd.Registry
	Tracker      *Tracker
	JobConfigs   JobConfigs
}

type CommandSetConfig struct {
	Registry         *indexing.Registry
	Provider         providers.Provider
	GenerationStore  types.GenerationStore
	Progress         types.ProgressReporter
	Activities       []types.ActivityHook
	Metrics          []types.MetricsHook
	Logger           types.Logger
	DefaultBatchSize int
	Clock            types.Clock
	Tracker          *Tracker
	JobConfigs       JobConfigs
}

func NewCommandSet(cfg CommandSetConfig) (*CommandSet, error) {
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("index registry is required", nil)
	}
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	tracker := cfg.Tracker
	if tracker == nil {
		tracker = NewTracker()
	}
	jobConfigs := cfg.JobConfigs.normalized()

	activities := make([]types.ActivityHook, 0, len(cfg.Activities)+1)
	activities = append(activities, trackingActivityHook{tracker: tracker})
	activities = append(activities, cfg.Activities...)

	progress := cfg.Progress
	if tracker != nil {
		progress = trackingProgressReporter{base: cfg.Progress, tracker: tracker}
	}
	generationStore := cfg.GenerationStore
	if tracker != nil && cfg.GenerationStore != nil {
		generationStore = trackingGenerationStore{base: cfg.GenerationStore, tracker: tracker}
	}

	indexer, err := indexing.NewIndexer(indexing.IndexerConfig{
		Registry:         cfg.Registry,
		Provider:         cfg.Provider,
		GenerationStore:  generationStore,
		Progress:         progress,
		Activities:       activities,
		Metrics:          cfg.Metrics,
		Logger:           cfg.Logger,
		DefaultBatchSize: cfg.DefaultBatchSize,
		Clock:            cfg.Clock,
	})
	if err != nil {
		return nil, err
	}

	indexCmd, err := command.NewIndexRecord(command.IndexRecordConfig{Indexer: indexer})
	if err != nil {
		return nil, err
	}
	deleteCmd, err := command.NewDeleteRecord(command.DeleteRecordConfig{Indexer: indexer})
	if err != nil {
		return nil, err
	}
	reindexCmd, err := command.NewReindexIndex(command.ReindexIndexConfig{Indexer: indexer})
	if err != nil {
		return nil, err
	}

	registry := queuecmd.NewRegistry()
	if err := registerCommand(registry, types.IndexRecordInput{}, indexCmd.Execute, jobConfigs.IndexRecord); err != nil {
		return nil, err
	}
	if err := registerCommand(registry, types.DeleteRecordInput{}, deleteCmd.Execute, jobConfigs.DeleteRecord); err != nil {
		return nil, err
	}
	if err := registerCommand(registry, types.ReindexIndexInput{}, reindexCmd.Execute, jobConfigs.ReindexIndex); err != nil {
		return nil, err
	}

	return &CommandSet{
		Indexer:      indexer,
		IndexRecord:  indexCmd,
		DeleteRecord: deleteCmd,
		ReindexIndex: reindexCmd,
		Registry:     registry,
		Tracker:      tracker,
		JobConfigs:   jobConfigs,
	}, nil
}

func registerCommand[T any](registry *queuecmd.Registry, template T, handler func(context.Context, T) error, cfg job.Config) error {
	if registry == nil {
		return fmt.Errorf("command registry not configured")
	}
	commandID := gcommand.GetMessageType(template)
	return registry.Register(queuecmd.Entry{
		ID:          commandID,
		MessageType: commandID,
		Config:      cfg,
		Handler: func(ctx context.Context, params map[string]any) error {
			metadata := dispatchMetadataFromParams(params)
			msg, err := decodePayload[T](clonePayload(params))
			if err != nil {
				return err
			}
			ctx = withOperationKey(ctx, metadata.OperationKey)
			return handler(ctx, msg)
		},
	})
}

func decodePayload[T any](params map[string]any) (T, error) {
	var out T
	if len(params) == 0 {
		return out, nil
	}
	body, err := json.Marshal(params)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	return out, nil
}
