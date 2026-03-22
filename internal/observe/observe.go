package observe

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

func Debug(logger types.Logger, msg string, metadata map[string]any) {
	if logger == nil {
		return
	}
	logger.Debug(msg, cloneAnyMap(metadata))
}

func Info(logger types.Logger, msg string, metadata map[string]any) {
	if logger == nil {
		return
	}
	logger.Info(msg, cloneAnyMap(metadata))
}

func Warn(logger types.Logger, msg string, metadata map[string]any) {
	if logger == nil {
		return
	}
	logger.Warn(msg, cloneAnyMap(metadata))
}

func Error(logger types.Logger, msg string, metadata map[string]any) {
	if logger == nil {
		return
	}
	logger.Error(msg, cloneAnyMap(metadata))
}

func Count(ctx context.Context, metrics []types.MetricsHook, logger types.Logger, metric string, delta int64, labels map[string]string) {
	for _, hook := range metrics {
		if hook == nil {
			continue
		}
		callMetric(ctx, logger, metric, labels, func() {
			hook.Count(ctx, metric, delta, cloneStringMap(labels))
		})
	}
}

func ObserveDuration(ctx context.Context, metrics []types.MetricsHook, logger types.Logger, metric string, startedAt time.Time, labels map[string]string) {
	if startedAt.IsZero() {
		return
	}
	ms := float64(time.Since(startedAt).Milliseconds())
	for _, hook := range metrics {
		if hook == nil {
			continue
		}
		callMetric(ctx, logger, metric, labels, func() {
			hook.Observe(ctx, metric, ms, cloneStringMap(labels))
		})
	}
}

func NotifyActivities(ctx context.Context, hooks []types.ActivityHook, logger types.Logger, event types.ActivityEvent) {
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					Error(logger, "search.activity_hook_panic", map[string]any{
						"channel":     event.Channel,
						"verb":        event.Verb,
						"object_type": event.ObjectType,
						"object_id":   event.ObjectID,
						"panic":       fmt.Sprint(recovered),
					})
				}
			}()
			hook.Notify(ctx, cloneActivityEvent(event))
		}()
	}
}

func callMetric(ctx context.Context, logger types.Logger, metric string, labels map[string]string, fn func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			Error(logger, "search.metrics_hook_panic", map[string]any{
				"metric": metric,
				"labels": cloneStringMap(labels),
				"panic":  fmt.Sprint(recovered),
			})
		}
	}()
	fn()
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func cloneActivityEvent(event types.ActivityEvent) types.ActivityEvent {
	event.Metadata = cloneAnyMap(event.Metadata)
	return event
}
