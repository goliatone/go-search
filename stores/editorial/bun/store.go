package bunstore

import (
	"context"
	"time"

	repository "github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/uptrace/bun"
)

type Store struct {
	db *bun.DB
}

var _ repository.DBProvider = (*Store)(nil)

func New(db *bun.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *bun.DB {
	return s.db
}

func (s *Store) Upsert(ctx context.Context, rule types.EditorialRankRule) error {
	model := toModel(rule)
	_, err := s.db.NewInsert().
		Model(&model).
		On("CONFLICT (id) DO UPDATE").
		Set("target_type = EXCLUDED.target_type").
		Set("target_id = EXCLUDED.target_id").
		Set("parent_target_id = EXCLUDED.parent_target_id").
		Set("action = EXCLUDED.action").
		Set("weight = EXCLUDED.weight").
		Set("position = EXCLUDED.position").
		Set("enabled = EXCLUDED.enabled").
		Set("indexes = EXCLUDED.indexes").
		Set("tenant_id = EXCLUDED.tenant_id").
		Set("org_id = EXCLUDED.org_id").
		Set("locale = EXCLUDED.locale").
		Set("topic = EXCLUDED.topic").
		Set("query = EXCLUDED.query").
		Set("ranking_profile = EXCLUDED.ranking_profile").
		Set("filters = EXCLUDED.filters").
		Set("starts_at_unix = EXCLUDED.starts_at_unix").
		Set("ends_at_unix = EXCLUDED.ends_at_unix").
		Set("reason = EXCLUDED.reason").
		Set("metadata = EXCLUDED.metadata").
		Exec(ctx)
	return err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.NewDelete().Model((*RuleModel)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (s *Store) ListApplicable(ctx context.Context, req types.SearchRequest) ([]types.EditorialRankRule, error) {
	models := []RuleModel{}
	q := s.db.NewSelect().Model(&models).Where("enabled = ?", true)
	if len(req.Indexes) == 1 {
		q = q.Where("array_length(indexes, 1) IS NULL OR ? = ANY(indexes)", req.Indexes[0])
	}
	if req.Scope.TenantID != "" {
		q = q.Where("(tenant_id = '' OR tenant_id = ?)", req.Scope.TenantID)
	}
	if req.Scope.OrgID != "" {
		q = q.Where("(org_id = '' OR org_id = ?)", req.Scope.OrgID)
	}
	if req.Locale != "" {
		q = q.Where("(locale = '' OR locale = ?)", req.Locale)
	}
	if req.RankingProfile != "" {
		q = q.Where("(ranking_profile = '' OR ranking_profile = ?)", req.RankingProfile)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]types.EditorialRankRule, 0, len(models))
	for _, model := range models {
		out = append(out, toRule(model))
	}
	return out, nil
}

func toModel(rule types.EditorialRankRule) RuleModel {
	model := RuleModel{
		ID:             rule.ID,
		TargetType:     rule.TargetType,
		TargetID:       rule.TargetID,
		ParentTargetID: rule.ParentTargetID,
		Action:         rule.Action,
		Weight:         rule.Weight,
		Position:       rule.Position,
		Enabled:        rule.Enabled,
		Indexes:        append([]string(nil), rule.Scope.Indexes...),
		TenantID:       rule.Scope.TenantID,
		OrgID:          rule.Scope.OrgID,
		Locale:         rule.Scope.Locale,
		Topic:          rule.Scope.Topic,
		Query:          rule.Scope.Query,
		RankingProfile: rule.Scope.RankingProfile,
		Filters:        rule.Scope.Filters,
		Reason:         rule.Reason,
		Metadata:       rule.Metadata,
	}
	if rule.StartsAt != nil {
		v := rule.StartsAt.Unix()
		model.StartsAtUnix = &v
	}
	if rule.EndsAt != nil {
		v := rule.EndsAt.Unix()
		model.EndsAtUnix = &v
	}
	return model
}

func toRule(model RuleModel) types.EditorialRankRule {
	rule := types.EditorialRankRule{
		ID:             model.ID,
		TargetType:     model.TargetType,
		TargetID:       model.TargetID,
		ParentTargetID: model.ParentTargetID,
		Action:         model.Action,
		Weight:         model.Weight,
		Position:       model.Position,
		Enabled:        model.Enabled,
		Scope: types.EditorialScope{
			Indexes:        append([]string(nil), model.Indexes...),
			TenantID:       model.TenantID,
			OrgID:          model.OrgID,
			Locale:         model.Locale,
			Topic:          model.Topic,
			Query:          model.Query,
			RankingProfile: model.RankingProfile,
			Filters:        model.Filters,
		},
		Reason:   model.Reason,
		Metadata: model.Metadata,
	}
	if model.StartsAtUnix != nil {
		t := time.Unix(*model.StartsAtUnix, 0)
		rule.StartsAt = &t
	}
	if model.EndsAtUnix != nil {
		t := time.Unix(*model.EndsAtUnix, 0)
		rule.EndsAt = &t
	}
	return rule
}
