package command

import (
	"context"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

func notifyActivities(ctx context.Context, hooks []types.ActivityHook, verb, objectType, objectID string, metadata map[string]any) {
	event := types.ActivityEvent{
		Channel:    "search",
		Verb:       verb,
		ObjectType: objectType,
		ObjectID:   objectID,
		OccurredAt: time.Now().UnixMilli(),
		Metadata:   metadata,
	}
	for _, hook := range hooks {
		hook.Notify(ctx, event)
	}
}

func bumpGeneration(ctx context.Context, store types.GenerationStore, index string) {
	if store == nil || index == "" {
		return
	}
	_, _ = store.Bump(ctx, index)
}
