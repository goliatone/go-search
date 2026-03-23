package gousers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

type ActivitySinkHook struct {
	Sink    userstypes.ActivitySink
	Logger  types.Logger
	Metrics []types.MetricsHook
}

func (h ActivitySinkHook) Notify(ctx context.Context, event types.ActivityEvent) {
	if h.Sink == nil {
		return
	}
	record := userstypes.ActivityRecord{
		ID:         uuid.New(),
		UserID:     parseUUID(resolveUserID(event)),
		ActorID:    parseUUID(firstString(event.ActorID, metadataString(event.Metadata, "actor_id"))),
		Verb:       strings.TrimSpace(event.Verb),
		ObjectType: strings.TrimSpace(event.ObjectType),
		ObjectID:   strings.TrimSpace(event.ObjectID),
		Channel:    strings.TrimSpace(event.Channel),
		TenantID:   parseUUID(firstString(event.TenantID, metadataString(event.Metadata, "tenant_id"))),
		OrgID:      parseUUID(firstString(event.OrgID, metadataString(event.Metadata, "org_id"))),
		Data:       cloneMetadata(event.Metadata),
		OccurredAt: time.UnixMilli(event.OccurredAt),
	}
	if err := h.Sink.Log(ctx, record); err != nil {
		observe.Count(ctx, h.Metrics, h.Logger, "search.adapter.gousers.activity_sink.error.count", 1, nil)
		observe.Error(h.Logger, "search.adapter.gousers.activity_sink_failed", map[string]any{
			"channel":     record.Channel,
			"verb":        record.Verb,
			"object_type": record.ObjectType,
			"object_id":   record.ObjectID,
			"record_id":   strings.TrimSpace(event.RecordID),
			"tenant_id":   firstString(event.TenantID, metadataString(event.Metadata, "tenant_id")),
			"org_id":      firstString(event.OrgID, metadataString(event.Metadata, "org_id")),
			"message":     err.Error(),
		})
	}
}

func parseUUID(raw string) uuid.UUID {
	if strings.TrimSpace(raw) == "" {
		return uuid.Nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func resolveUserID(event types.ActivityEvent) string {
	if userID := metadataString(event.Metadata, "user_id"); userID != "" {
		return userID
	}
	if recordID := strings.TrimSpace(event.RecordID); recordID != "" {
		return recordID
	}
	if strings.EqualFold(strings.TrimSpace(event.ObjectType), "user") {
		return strings.TrimSpace(event.ObjectID)
	}
	return ""
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
