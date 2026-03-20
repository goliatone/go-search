package command

import (
	"context"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

func notifyActivities(ctx context.Context, clock types.Clock, hooks []types.ActivityHook, verb, objectType, objectID string, metadata map[string]any) {
	if clock == nil {
		clock = types.SystemClock()
	}
	event := types.ActivityEvent{
		Channel:    "search",
		Verb:       verb,
		ObjectType: objectType,
		ObjectID:   objectID,
		OccurredAt: clock.Now().UnixMilli(),
		Metadata:   metadata,
	}
	for _, hook := range hooks {
		hook.Notify(ctx, event)
	}
}

func bumpGeneration(ctx context.Context, store types.GenerationStore, index string) error {
	if store == nil || index == "" {
		return nil
	}
	if _, err := store.Bump(ctx, index); err != nil {
		return errs.Wrap(err, map[string]any{"index": index, "operation": "generation_bump"})
	}
	return nil
}
