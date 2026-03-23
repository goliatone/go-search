package gousers

import (
	"context"
	"strings"

	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

type RecordMutator interface {
	IndexRecord(ctx context.Context, index, registrationKey, recordID string) ([]types.Document, error)
	DeleteRecord(ctx context.Context, index, registrationKey, recordID string) error
}

type LifecycleHooksConfig struct {
	Indexer         RecordMutator
	Index           string
	RegistrationKey string
	Logger          types.Logger
	Metrics         []types.MetricsHook
}

type LifecycleHooks struct {
	indexer         RecordMutator
	index           string
	registrationKey string
	logger          types.Logger
	metrics         []types.MetricsHook
}

func NewLifecycleHooks(cfg LifecycleHooksConfig) *LifecycleHooks {
	return &LifecycleHooks{
		indexer:         cfg.Indexer,
		index:           cfg.Index,
		registrationKey: strings.TrimSpace(cfg.RegistrationKey),
		logger:          cfg.Logger,
		metrics:         cfg.Metrics,
	}
}

func (h *LifecycleHooks) Hooks() userstypes.Hooks {
	return userstypes.Hooks{
		AfterLifecycle:     h.afterLifecycle,
		AfterProfileChange: h.afterProfileChange,
		AfterRoleChange:    h.afterRoleChange,
		AfterActivity:      h.afterActivity,
	}
}

func (h *LifecycleHooks) afterLifecycle(ctx context.Context, event userstypes.LifecycleEvent) {
	if event.UserID == uuid.Nil {
		return
	}
	if event.ToState == userstypes.LifecycleStateArchived {
		h.delete(ctx, event.UserID)
		return
	}
	h.reindex(ctx, event.UserID)
}

func (h *LifecycleHooks) afterProfileChange(ctx context.Context, event userstypes.ProfileEvent) {
	h.reindex(ctx, event.UserID)
}

func (h *LifecycleHooks) afterRoleChange(ctx context.Context, event userstypes.RoleEvent) {
	if event.UserID == uuid.Nil {
		return
	}
	h.reindex(ctx, event.UserID)
}

func (h *LifecycleHooks) afterActivity(ctx context.Context, event userstypes.ActivityRecord) {
	switch strings.TrimSpace(event.Verb) {
	case "user.created", "user.updated":
	default:
		return
	}
	userID := event.UserID
	if userID == uuid.Nil && strings.EqualFold(strings.TrimSpace(event.ObjectType), "user") {
		parsed, err := uuid.Parse(strings.TrimSpace(event.ObjectID))
		if err == nil {
			userID = parsed
		}
	}
	if userID == uuid.Nil {
		return
	}
	h.reindex(ctx, userID)
}

func (h *LifecycleHooks) reindex(ctx context.Context, userID uuid.UUID) {
	if h == nil || h.indexer == nil || userID == uuid.Nil || h.index == "" {
		return
	}
	observe.Count(ctx, h.metrics, h.logger, "search.adapter.gousers.index.count", 1, map[string]string{"index": h.index})
	if _, err := h.indexer.IndexRecord(ctx, h.index, h.registrationKey, userID.String()); err != nil {
		observe.Error(h.logger, "search.adapter.gousers.index_failed", map[string]any{
			"index":            h.index,
			"registration_key": h.registrationKey,
			"record":           userID.String(),
			"message":          err.Error(),
		})
	}
}

func (h *LifecycleHooks) delete(ctx context.Context, userID uuid.UUID) {
	if h == nil || h.indexer == nil || userID == uuid.Nil || h.index == "" {
		return
	}
	observe.Count(ctx, h.metrics, h.logger, "search.adapter.gousers.delete.count", 1, map[string]string{"index": h.index})
	if err := h.indexer.DeleteRecord(ctx, h.index, h.registrationKey, userID.String()); err != nil {
		observe.Error(h.logger, "search.adapter.gousers.delete_failed", map[string]any{
			"index":            h.index,
			"registration_key": h.registrationKey,
			"record":           userID.String(),
			"message":          err.Error(),
		})
	}
}
