package jobs

import (
	"context"
	"errors"
	"time"

	job "github.com/goliatone/go-job"
	jobqueue "github.com/goliatone/go-job/queue"
	"github.com/goliatone/go-job/queue/cancellation"
	queuecmd "github.com/goliatone/go-job/queue/command"
	"github.com/goliatone/go-job/queue/worker"
	"github.com/goliatone/go-search/internal/errs"
)

type WorkerRuntime struct {
	worker       *worker.Worker
	statusReader jobqueue.DispatchStatusReader
	tracker      *Tracker
}

type WorkerConfig struct {
	Dequeuer     jobqueue.Dequeuer
	StatusReader jobqueue.DispatchStatusReader
	CancelStore  cancellation.Store
	Registry     *queuecmd.Registry
	Tracker      *Tracker
	RetryPolicy  worker.RetryPolicy
	Options      []worker.Option
}

func NewWorker(cfg WorkerConfig) (*WorkerRuntime, error) {
	if cfg.Dequeuer == nil {
		return nil, errs.ConfigurationError("jobs dequeuer is required", nil)
	}
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("jobs registry is required", nil)
	}
	if cfg.Tracker == nil {
		cfg.Tracker = NewTracker()
	}
	hooks := trackingHooks{tracker: cfg.Tracker}
	opts := make([]worker.Option, 0, len(cfg.Options)+4)
	opts = append(opts, cfg.Options...)
	opts = append(opts, worker.WithHooks(hooks))
	if cfg.CancelStore != nil {
		opts = append(opts, worker.WithCancellationStore(cfg.CancelStore))
		opts = append(opts, worker.WithCancelKeyExtractor(func(msg *job.ExecutionMessage) string {
			return operationKeyFromMessage(msg)
		}))
	}
	if cfg.RetryPolicy != nil {
		opts = append(opts, worker.WithRetryPolicy(cfg.RetryPolicy))
	} else {
		opts = append(opts, worker.WithRetryPolicy(worker.DefaultRetryPolicy{
			MaxAttempts: 3,
			Backoff: worker.BackoffConfig{
				Strategy:    worker.BackoffExponential,
				Interval:    100 * time.Millisecond,
				MaxInterval: 2 * time.Second,
				Jitter:      true,
			},
		}))
	}
	w, err := queuecmd.NewLocalWorker(cfg.Dequeuer, cfg.Registry, queuecmd.LocalWorkerConfig{WorkerOptions: opts})
	if err != nil {
		return nil, err
	}
	return &WorkerRuntime{worker: w, statusReader: cfg.StatusReader, tracker: cfg.Tracker}, nil
}

func (r *WorkerRuntime) Start(ctx context.Context) error {
	if r == nil || r.worker == nil {
		return nil
	}
	return r.worker.Start(ctx)
}

func (r *WorkerRuntime) Stop(ctx context.Context) error {
	if r == nil || r.worker == nil {
		return nil
	}
	return r.worker.Stop(ctx)
}

func (r *WorkerRuntime) Pause() error {
	if r == nil || r.worker == nil {
		return nil
	}
	return r.worker.Pause()
}

func (r *WorkerRuntime) Resume() error {
	if r == nil || r.worker == nil {
		return nil
	}
	return r.worker.Resume()
}

func (r *WorkerRuntime) Status() worker.Status {
	if r == nil || r.worker == nil {
		return worker.Status{}
	}
	return r.worker.Status()
}

type trackingHooks struct {
	tracker *Tracker
}

func (h trackingHooks) OnStart(ctx context.Context, event worker.Event) {
	if h.tracker == nil {
		return
	}
	h.tracker.MarkStarted(ctx, operationKeyFromMessage(event.Message), event.Attempt)
}

func (h trackingHooks) OnSuccess(ctx context.Context, event worker.Event) {
	if h.tracker == nil {
		return
	}
	h.tracker.MarkSucceeded(ctx, operationKeyFromMessage(event.Message), event.Attempt)
}

func (h trackingHooks) OnFailure(ctx context.Context, event worker.Event) {
	if h.tracker == nil {
		return
	}
	state := jobqueue.DispatchStateFailed
	if errors.Is(event.Err, context.Canceled) {
		state = jobqueue.DispatchStateCanceled
	}
	h.tracker.MarkFailed(ctx, operationKeyFromMessage(event.Message), event.Attempt, event.Err, state)
}

func (h trackingHooks) OnRetry(ctx context.Context, event worker.Event) {
	if h.tracker == nil {
		return
	}
	next := time.Now().Add(event.Delay)
	h.tracker.MarkRetry(ctx, operationKeyFromMessage(event.Message), event.Attempt, event.Err, &next)
}

func operationKeyFromMessage(msg *job.ExecutionMessage) string {
	if msg == nil {
		return ""
	}
	return dispatchMetadataFromParams(msg.Parameters).OperationKey
}
