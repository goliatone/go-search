package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type DeleteDocumentsConfig struct {
	Provider        providers.Provider
	GenerationStore types.GenerationStore
	Activities      []types.ActivityHook
	Clock           types.Clock
}

type DeleteDocuments struct {
	provider        providers.Provider
	generationStore types.GenerationStore
	activities      []types.ActivityHook
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
		clock:           cfg.Clock,
	}, nil
}

func (c *DeleteDocuments) Execute(ctx context.Context, msg types.DeleteDocumentsInput) error {
	if err := c.provider.DeleteDocuments(ctx, msg.Index, msg.IDs); err != nil {
		return err
	}
	if err := bumpGeneration(ctx, c.generationStore, msg.Index); err != nil {
		return err
	}
	notifyActivities(ctx, c.clock, c.activities, "deleted", "documents", msg.Index, map[string]any{"index": msg.Index, "count": len(msg.IDs)})
	return nil
}
