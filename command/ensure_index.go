package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type EnsureIndexConfig struct {
	Provider   providers.Provider
	Registry   *indexing.Registry
	Activities []types.ActivityHook
	Metrics    []types.MetricsHook
	Logger     types.Logger
	Clock      types.Clock
}

type EnsureIndex struct {
	provider   providers.Provider
	registry   *indexing.Registry
	activities []types.ActivityHook
	metrics    []types.MetricsHook
	logger     types.Logger
	clock      types.Clock
}

var _ gcommand.Commander[types.EnsureIndexInput] = (*EnsureIndex)(nil)

func NewEnsureIndex(cfg EnsureIndexConfig) (*EnsureIndex, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("registry is required", nil)
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &EnsureIndex{
		provider:   cfg.Provider,
		registry:   cfg.Registry,
		activities: cfg.Activities,
		metrics:    cfg.Metrics,
		logger:     cfg.Logger,
		clock:      cfg.Clock,
	}, nil
}

func (c *EnsureIndex) Execute(ctx context.Context, msg types.EnsureIndexInput) error {
	startedAt := c.clock.Now()
	if err := c.provider.EnsureIndex(ctx, msg.Definition); err != nil {
		observe.Count(ctx, c.metrics, c.logger, "search.ensure_index.error.count", 1, map[string]string{"index": msg.Definition.Name})
		return err
	}
	if err := c.registry.Register(msg.Definition, nil); err != nil {
		observe.Count(ctx, c.metrics, c.logger, "search.ensure_index.error.count", 1, map[string]string{"index": msg.Definition.Name})
		return err
	}
	observe.Count(ctx, c.metrics, c.logger, "search.ensure_index.count", 1, map[string]string{"index": msg.Definition.Name})
	observe.ObserveDuration(ctx, c.metrics, c.logger, "search.ensure_index.duration_ms", startedAt, map[string]string{"index": msg.Definition.Name})
	notifyActivities(ctx, c.clock, c.activities, c.logger, "ensured", "index", msg.Definition.Name, map[string]any{"index": msg.Definition.Name})
	return nil
}
