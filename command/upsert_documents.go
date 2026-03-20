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
	Clock           types.Clock
}

type UpsertDocuments struct {
	provider        providers.Provider
	generationStore types.GenerationStore
	activities      []types.ActivityHook
	clock           types.Clock
}

var _ gcommand.Commander[types.UpsertDocumentsInput] = (*UpsertDocuments)(nil)

func NewUpsertDocuments(cfg UpsertDocumentsConfig) (*UpsertDocuments, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &UpsertDocuments{
		provider:        cfg.Provider,
		generationStore: cfg.GenerationStore,
		activities:      cfg.Activities,
		clock:           cfg.Clock,
	}, nil
}

func (c *UpsertDocuments) Execute(ctx context.Context, msg types.UpsertDocumentsInput) error {
	if err := c.provider.UpsertDocuments(ctx, msg.Index, msg.Documents); err != nil {
		return err
	}
	if err := bumpGeneration(ctx, c.generationStore, msg.Index); err != nil {
		return err
	}
	notifyActivities(ctx, c.clock, c.activities, "upserted", "documents", msg.Index, map[string]any{"index": msg.Index, "count": len(msg.Documents)})
	return nil
}
