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
}

type DeleteDocuments struct {
	provider        providers.Provider
	generationStore types.GenerationStore
	activities      []types.ActivityHook
}

var _ gcommand.Commander[types.DeleteDocumentsInput] = (*DeleteDocuments)(nil)

func NewDeleteDocuments(cfg DeleteDocumentsConfig) (*DeleteDocuments, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	return &DeleteDocuments{provider: cfg.Provider, generationStore: cfg.GenerationStore, activities: cfg.Activities}, nil
}

func (c *DeleteDocuments) Execute(ctx context.Context, msg types.DeleteDocumentsInput) error {
	if err := c.provider.DeleteDocuments(ctx, msg.Index, msg.IDs); err != nil {
		return err
	}
	bumpGeneration(ctx, c.generationStore, msg.Index)
	notifyActivities(ctx, c.activities, "deleted", "documents", msg.Index, map[string]any{"index": msg.Index, "count": len(msg.IDs)})
	return nil
}
