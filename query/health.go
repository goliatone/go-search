package query

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers"
)

type HealthConfig struct {
	Provider providers.Provider
}

type Health struct {
	provider providers.Provider
}

var _ gcommand.Querier[types.HealthRequest, types.HealthStatus] = (*Health)(nil)

func NewHealth(cfg HealthConfig) (*Health, error) {
	if cfg.Provider == nil {
		return nil, errs.ConfigurationError("provider is required", nil)
	}
	return &Health{provider: cfg.Provider}, nil
}

func (q *Health) Query(ctx context.Context, req types.HealthRequest) (types.HealthStatus, error) {
	return q.provider.Health(ctx, req)
}
