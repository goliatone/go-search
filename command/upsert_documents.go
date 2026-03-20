package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type UpsertDocumentsConfig struct {
	Provider        providers.Provider
	GenerationStore types.GenerationStore
	Activities      []types.ActivityHook
}

type UpsertDocuments struct {
	provider        providers.Provider
	generationStore types.GenerationStore
	activities      []types.ActivityHook
}

var _ gcommand.Commander[types.UpsertDocumentsInput] = (*UpsertDocuments)(nil)

func NewUpsertDocuments(cfg UpsertDocumentsConfig) (*UpsertDocuments, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	return &UpsertDocuments{provider: cfg.Provider, generationStore: cfg.GenerationStore, activities: cfg.Activities}, nil
}

func (c *UpsertDocuments) Execute(ctx context.Context, msg types.UpsertDocumentsInput) error {
	if err := c.provider.UpsertDocuments(ctx, msg.Index, msg.Documents); err != nil {
		return err
	}
	bumpGeneration(ctx, c.generationStore, msg.Index)
	notifyActivities(ctx, c.activities, "upserted", "documents", msg.Index, map[string]any{"index": msg.Index, "count": len(msg.Documents)})
	return nil
}
