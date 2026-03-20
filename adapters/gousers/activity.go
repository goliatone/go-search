package gousers

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/goliatone/go-search/pkg/types"
)

type ActivitySinkHook struct {
	Sink userstypes.ActivitySink
}

func (h ActivitySinkHook) Notify(ctx context.Context, event types.ActivityEvent) {
	if h.Sink == nil {
		return
	}
	_ = h.Sink.Log(ctx, userstypes.ActivityRecord{
		ID:         uuid.New(),
		ActorID:    parseUUID(event.ActorID),
		Verb:       strings.TrimSpace(event.Verb),
		ObjectType: strings.TrimSpace(event.ObjectType),
		ObjectID:   strings.TrimSpace(event.ObjectID),
		Channel:    strings.TrimSpace(event.Channel),
		TenantID:   parseUUID(event.TenantID),
		Data:       cloneMetadata(event.Metadata),
		OccurredAt: time.UnixMilli(event.OccurredAt),
	})
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
