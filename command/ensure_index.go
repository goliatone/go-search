package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type EnsureIndexConfig struct {
	Provider   providers.Provider
	Registry   *indexing.Registry
	Activities []types.ActivityHook
}

type EnsureIndex struct {
	provider   providers.Provider
	registry   *indexing.Registry
	activities []types.ActivityHook
}

var _ gcommand.Commander[types.EnsureIndexInput] = (*EnsureIndex)(nil)

func NewEnsureIndex(cfg EnsureIndexConfig) (*EnsureIndex, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("registry is required", nil)
	}
	return &EnsureIndex{provider: cfg.Provider, registry: cfg.Registry, activities: cfg.Activities}, nil
}

func (c *EnsureIndex) Execute(ctx context.Context, msg types.EnsureIndexInput) error {
	if err := c.provider.EnsureIndex(ctx, msg.Definition); err != nil {
		return err
	}
	if err := c.registry.Register(msg.Definition, nil); err != nil {
		return err
	}
	notifyActivities(ctx, c.activities, "ensured", "index", msg.Definition.Name, map[string]any{"index": msg.Definition.Name})
	return nil
}
