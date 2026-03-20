package query

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type EditorialRulesConfig struct {
	Store types.EditorialRuleAdminStore
}

type EditorialRules struct {
	store types.EditorialRuleAdminStore
}

var _ gcommand.Querier[types.EditorialRuleListRequest, []types.EditorialRankRule] = (*EditorialRules)(nil)

func NewEditorialRules(cfg EditorialRulesConfig) (*EditorialRules, error) {
	if cfg.Store == nil {
		return nil, errs.ConfigurationError("editorial rule store is required", nil)
	}
	return &EditorialRules{store: cfg.Store}, nil
}

func (q *EditorialRules) Query(ctx context.Context, req types.EditorialRuleListRequest) ([]types.EditorialRankRule, error) {
	return q.store.List(ctx, req)
}
