package gousers

import (
	"context"

	"github.com/google/uuid"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
)

type RecordMutator interface {
	IndexRecord(ctx context.Context, index string, recordID string) ([]types.Document, error)
	DeleteRecord(ctx context.Context, index string, recordID string) error
}

type LifecycleHooksConfig struct {
	Indexer  RecordMutator
	Index    string
	Logger   types.Logger
	Metrics  []types.MetricsHook
}

type LifecycleHooks struct {
	indexer RecordMutator
	index   string
	logger  types.Logger
	metrics []types.MetricsHook
}

func NewLifecycleHooks(cfg LifecycleHooksConfig) *LifecycleHooks {
	return &LifecycleHooks{
		indexer: cfg.Indexer,
		index:   cfg.Index,
		logger:  cfg.Logger,
		metrics: cfg.Metrics,
	}
}

func (h *LifecycleHooks) Hooks() userstypes.Hooks {
	return userstypes.Hooks{
		AfterLifecycle: h.afterLifecycle,
		AfterProfileChange: h.afterProfileChange,
		AfterRoleChange: h.afterRoleChange,
	}
}

func (h *LifecycleHooks) afterLifecycle(ctx context.Context, event userstypes.LifecycleEvent) {
	h.reindex(ctx, event.UserID)
}

func (h *LifecycleHooks) afterProfileChange(ctx context.Context, event userstypes.ProfileEvent) {
	h.reindex(ctx, event.UserID)
}

func (h *LifecycleHooks) afterRoleChange(ctx context.Context, event userstypes.RoleEvent) {
	h.reindex(ctx, event.UserID)
}

func (h *LifecycleHooks) reindex(ctx context.Context, userID uuid.UUID) {
	if h == nil || h.indexer == nil || userID == uuid.Nil || h.index == "" {
		return
	}
	observe.Count(ctx, h.metrics, h.logger, "search.adapter.gousers.index.count", 1, map[string]string{"index": h.index})
	if _, err := h.indexer.IndexRecord(ctx, h.index, userID.String()); err != nil {
		observe.Error(h.logger, "search.adapter.gousers.index_failed", map[string]any{
			"index":   h.index,
			"record":  userID.String(),
			"message": err.Error(),
		})
	}
}
