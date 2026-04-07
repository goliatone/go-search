package jobs

import (
	"context"

	"github.com/goliatone/go-search/pkg/types"
)

type trackingGenerationStore struct {
	base    types.GenerationStore
	tracker *Tracker
}

func (s trackingGenerationStore) Get(ctx context.Context, index string) (int64, error) {
	if s.base == nil {
		return 0, nil
	}
	return s.base.Get(ctx, index)
}

func (s trackingGenerationStore) Bump(ctx context.Context, index string) (int64, error) {
	if s.base == nil {
		return 0, nil
	}
	value, err := s.base.Bump(ctx, index)
	if err == nil && s.tracker != nil {
		s.tracker.RecordGeneration(ctx, operationKeyFromContext(ctx), index, value)
	}
	return value, err
}

type trackingProgressReporter struct {
	base    types.ProgressReporter
	tracker *Tracker
}

func (r trackingProgressReporter) Report(ctx context.Context, update types.ProgressUpdate) {
	if r.base != nil {
		r.base.Report(ctx, update)
	}
	if r.tracker != nil {
		r.tracker.RecordProgress(ctx, operationKeyFromContext(ctx), update)
	}
}

type trackingActivityHook struct {
	tracker *Tracker
}

func (h trackingActivityHook) Notify(ctx context.Context, event types.ActivityEvent) {
	if h.tracker == nil {
		return
	}
	h.tracker.RecordActivity(ctx, operationKeyFromContext(ctx), event)
}
