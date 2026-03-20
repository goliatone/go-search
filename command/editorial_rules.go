package command

import (
	"context"
	"strings"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
)

type UpsertEditorialRuleConfig struct {
	Store      types.EditorialRuleAdminStore
	Activities []types.ActivityHook
	Metrics    []types.MetricsHook
	Logger     types.Logger
	Clock      types.Clock
}

type UpsertEditorialRule struct {
	store      types.EditorialRuleAdminStore
	activities []types.ActivityHook
	metrics    []types.MetricsHook
	logger     types.Logger
	clock      types.Clock
}

var _ gcommand.Commander[types.UpsertEditorialRuleInput] = (*UpsertEditorialRule)(nil)

func NewUpsertEditorialRule(cfg UpsertEditorialRuleConfig) (*UpsertEditorialRule, error) {
	if cfg.Store == nil {
		return nil, errs.ConfigurationError("editorial rule store is required", nil)
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &UpsertEditorialRule{
		store:      cfg.Store,
		activities: cfg.Activities,
		metrics:    cfg.Metrics,
		logger:     cfg.Logger,
		clock:      cfg.Clock,
	}, nil
}

func (c *UpsertEditorialRule) Execute(ctx context.Context, msg types.UpsertEditorialRuleInput) error {
	if err := validateEditorialRule(msg.Rule); err != nil {
		return err
	}
	if err := c.store.Upsert(ctx, msg.Rule); err != nil {
		return err
	}
	observe.Count(ctx, c.metrics, c.logger, "search.editorial_rule.upsert.count", 1, map[string]string{
		"action": msg.Rule.Action,
	})
	observe.NotifyActivities(ctx, c.activities, c.logger, types.ActivityEvent{
		Channel:    "search",
		Verb:       "upserted",
		ObjectType: "editorial_rule",
		ObjectID:   msg.Rule.ID,
		OccurredAt: c.clock.Now().UnixMilli(),
		Metadata: map[string]any{
			"action":           msg.Rule.Action,
			"parent_target_id": strings.TrimSpace(msg.Rule.ParentTargetID),
			"enabled":          msg.Rule.Enabled,
			"indexes":          append([]string(nil), msg.Rule.Scope.Indexes...),
		},
	})
	return nil
}

type DeleteEditorialRuleConfig struct {
	Store      types.EditorialRuleAdminStore
	Activities []types.ActivityHook
	Metrics    []types.MetricsHook
	Logger     types.Logger
	Clock      types.Clock
}

type DeleteEditorialRule struct {
	store      types.EditorialRuleAdminStore
	activities []types.ActivityHook
	metrics    []types.MetricsHook
	logger     types.Logger
	clock      types.Clock
}

var _ gcommand.Commander[types.DeleteEditorialRuleInput] = (*DeleteEditorialRule)(nil)

func NewDeleteEditorialRule(cfg DeleteEditorialRuleConfig) (*DeleteEditorialRule, error) {
	if cfg.Store == nil {
		return nil, errs.ConfigurationError("editorial rule store is required", nil)
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &DeleteEditorialRule{
		store:      cfg.Store,
		activities: cfg.Activities,
		metrics:    cfg.Metrics,
		logger:     cfg.Logger,
		clock:      cfg.Clock,
	}, nil
}

func (c *DeleteEditorialRule) Execute(ctx context.Context, msg types.DeleteEditorialRuleInput) error {
	if strings.TrimSpace(msg.ID) == "" {
		return errs.InvalidEditorialRule("editorial rule id is required", nil)
	}
	if err := c.store.Delete(ctx, msg.ID); err != nil {
		return err
	}
	observe.Count(ctx, c.metrics, c.logger, "search.editorial_rule.delete.count", 1, nil)
	observe.NotifyActivities(ctx, c.activities, c.logger, types.ActivityEvent{
		Channel:    "search",
		Verb:       "deleted",
		ObjectType: "editorial_rule",
		ObjectID:   msg.ID,
		OccurredAt: c.clock.Now().UnixMilli(),
	})
	return nil
}

type SetEditorialRuleEnabledConfig struct {
	Store      types.EditorialRuleAdminStore
	Activities []types.ActivityHook
	Metrics    []types.MetricsHook
	Logger     types.Logger
	Clock      types.Clock
}

type SetEditorialRuleEnabled struct {
	store      types.EditorialRuleAdminStore
	activities []types.ActivityHook
	metrics    []types.MetricsHook
	logger     types.Logger
	clock      types.Clock
}

var _ gcommand.Commander[types.SetEditorialRuleEnabledInput] = (*SetEditorialRuleEnabled)(nil)

func NewSetEditorialRuleEnabled(cfg SetEditorialRuleEnabledConfig) (*SetEditorialRuleEnabled, error) {
	if cfg.Store == nil {
		return nil, errs.ConfigurationError("editorial rule store is required", nil)
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &SetEditorialRuleEnabled{
		store:      cfg.Store,
		activities: cfg.Activities,
		metrics:    cfg.Metrics,
		logger:     cfg.Logger,
		clock:      cfg.Clock,
	}, nil
}

func (c *SetEditorialRuleEnabled) Execute(ctx context.Context, msg types.SetEditorialRuleEnabledInput) error {
	if strings.TrimSpace(msg.ID) == "" {
		return errs.InvalidEditorialRule("editorial rule id is required", nil)
	}
	if err := c.store.SetEnabled(ctx, msg.ID, msg.Enabled); err != nil {
		return err
	}
	verb := "disabled"
	if msg.Enabled {
		verb = "enabled"
	}
	observe.Count(ctx, c.metrics, c.logger, "search.editorial_rule.toggle.count", 1, map[string]string{
		"enabled": boolLabel(msg.Enabled),
	})
	observe.NotifyActivities(ctx, c.activities, c.logger, types.ActivityEvent{
		Channel:    "search",
		Verb:       verb,
		ObjectType: "editorial_rule",
		ObjectID:   msg.ID,
		OccurredAt: c.clock.Now().UnixMilli(),
		Metadata: map[string]any{
			"enabled": msg.Enabled,
		},
	})
	return nil
}

func validateEditorialRule(rule types.EditorialRankRule) error {
	if strings.TrimSpace(rule.ID) == "" {
		return errs.InvalidEditorialRule("editorial rule id is required", nil)
	}
	if strings.TrimSpace(rule.Action) == "" {
		return errs.InvalidEditorialRule("editorial rule action is required", map[string]any{"id": rule.ID})
	}
	switch rule.Action {
	case types.EditorialActionBoost, types.EditorialActionBury, types.EditorialActionPin, types.EditorialActionHide:
	default:
		return errs.InvalidEditorialRule("unsupported editorial rule action", map[string]any{
			"id":     rule.ID,
			"action": rule.Action,
		})
	}
	if rule.RequiresParentTarget() {
		return errs.InvalidEditorialRule("parent_target_id is required for media/transcript editorial rules", map[string]any{
			"id":        rule.ID,
			"target_id": rule.TargetID,
		})
	}
	return nil
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
