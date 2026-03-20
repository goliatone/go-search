package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type DeleteDocumentsConfig struct {
	Provider        providers.Provider
	GenerationStore types.GenerationStore
	Activities      []types.ActivityHook
	Metrics         []types.MetricsHook
	Logger          types.Logger
	Clock           types.Clock
}

type DeleteDocuments struct {
	provider        providers.Provider
	generationStore types.GenerationStore
	activities      []types.ActivityHook
	metrics         []types.MetricsHook
	logger          types.Logger
	clock           types.Clock
}

var _ gcommand.Commander[types.DeleteDocumentsInput] = (*DeleteDocuments)(nil)

func NewDeleteDocuments(cfg DeleteDocumentsConfig) (*DeleteDocuments, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &DeleteDocuments{
		provider:        cfg.Provider,
		generationStore: cfg.GenerationStore,
		activities:      cfg.Activities,
		metrics:         cfg.Metrics,
		logger:          cfg.Logger,
		clock:           cfg.Clock,
	}, nil
}

func (c *DeleteDocuments) Execute(ctx context.Context, msg types.DeleteDocumentsInput) error {
	startedAt := c.clock.Now()
	if err := c.provider.DeleteDocuments(ctx, msg.Index, msg.IDs); err != nil {
		observe.Count(ctx, c.metrics, c.logger, "search.delete_documents.error.count", 1, map[string]string{"index": msg.Index})
		return err
	}
	if err := bumpGeneration(ctx, c.generationStore, msg.Index); err != nil {
		observe.Count(ctx, c.metrics, c.logger, "search.delete_documents.error.count", 1, map[string]string{"index": msg.Index})
		return err
	}
	observe.Count(ctx, c.metrics, c.logger, "search.delete_documents.count", int64(len(msg.IDs)), map[string]string{"index": msg.Index})
	observe.ObserveDuration(ctx, c.metrics, c.logger, "search.delete_documents.duration_ms", startedAt, map[string]string{"index": msg.Index})
	notifyActivities(ctx, c.clock, c.activities, c.logger, "deleted", "documents", msg.Index, map[string]any{"index": msg.Index, "count": len(msg.IDs)})
	return nil
}
