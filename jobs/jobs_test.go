package jobs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	job "github.com/goliatone/go-job"
	jobqueue "github.com/goliatone/go-job/queue"
	queuecmd "github.com/goliatone/go-job/queue/command"
	"github.com/goliatone/go-job/queue/worker"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers/memory"
)

type memoryGenerationStore struct {
	mu    sync.Mutex
	items map[string]int64
}

func newMemoryGenerationStore() *memoryGenerationStore {
	return &memoryGenerationStore{items: map[string]int64{}}
}

func (s *memoryGenerationStore) Get(_ context.Context, index string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[index], nil
}

func (s *memoryGenerationStore) Bump(_ context.Context, index string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[index]++
	return s.items[index], nil
}

type simpleRecord struct {
	ID    string
	Title string
	Body  string
}

type simpleSource struct {
	records map[string]simpleRecord
}

func (s simpleSource) Get(_ context.Context, id string) (simpleRecord, error) {
	record, ok := s.records[id]
	if !ok {
		return simpleRecord{}, errors.New("record not found")
	}
	return record, nil
}

func (s simpleSource) List(_ context.Context, _ int, _ string) ([]simpleRecord, string, error) {
	ids := make([]string, 0, len(s.records))
	for id := range s.records {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := make([]simpleRecord, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.records[id])
	}
	return out, "", nil
}

type controllableProjector struct {
	mu            sync.Mutex
	failRemaining map[string]int
	allow         <-chan struct{}
}

func (p *controllableProjector) Project(ctx context.Context, record simpleRecord) ([]types.Document, error) {
	if p.allow != nil {
		select {
		case <-p.allow:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p.mu.Lock()
	if remaining := p.failRemaining[record.ID]; remaining > 0 {
		p.failRemaining[record.ID] = remaining - 1
		p.mu.Unlock()
		return nil, fmt.Errorf("planned projector failure for %s", record.ID)
	}
	p.mu.Unlock()
	return []types.Document{{
		ID:         "doc-" + record.ID,
		Type:       types.DocumentTypeDocument,
		SourceType: "record",
		SourceID:   record.ID,
		Title:      record.Title,
		Body:       record.Body,
		Locale:     "en",
	}}, nil
}

type harness struct {
	commandSet *CommandSet
	client     *Client
	worker     *WorkerRuntime
	provider   *memory.Provider
	queue      *MemoryQueue
	cancel     *MemoryCancellationStore
	store      DispatchStore
}

func newHarness(t *testing.T, records map[string]simpleRecord, projector *controllableProjector, retryPolicy worker.RetryPolicy) *harness {
	return newHarnessWithStore(t, records, projector, retryPolicy, NewMemoryDispatchStore(), nil, nil)
}

type enqueueDelayQueue struct {
	*MemoryQueue
	delay time.Duration
}

func (q enqueueDelayQueue) Enqueue(ctx context.Context, msg *job.ExecutionMessage) (jobqueue.EnqueueReceipt, error) {
	receipt, err := q.MemoryQueue.Enqueue(ctx, msg)
	if err != nil {
		return receipt, err
	}
	time.Sleep(q.delay)
	return receipt, nil
}

func (q enqueueDelayQueue) EnqueueAt(ctx context.Context, msg *job.ExecutionMessage, at time.Time) (jobqueue.EnqueueReceipt, error) {
	receipt, err := q.MemoryQueue.EnqueueAt(ctx, msg, at)
	if err != nil {
		return receipt, err
	}
	time.Sleep(q.delay)
	return receipt, nil
}

func (q enqueueDelayQueue) EnqueueAfter(ctx context.Context, msg *job.ExecutionMessage, delay time.Duration) (jobqueue.EnqueueReceipt, error) {
	receipt, err := q.MemoryQueue.EnqueueAfter(ctx, msg, delay)
	if err != nil {
		return receipt, err
	}
	time.Sleep(q.delay)
	return receipt, nil
}

func newHarnessWithStore(t *testing.T, records map[string]simpleRecord, projector *controllableProjector, retryPolicy worker.RetryPolicy, store DispatchStore, queue *MemoryQueue, enqueuer jobqueue.ScheduledEnqueuer) *harness {
	t.Helper()

	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "documents"}
	registration := indexing.NewRegistration(def.Name, def, "record", simpleSource{records: records}, projector, func(record simpleRecord) string {
		return record.ID
	})
	if err := registry.Register(def, registration); err != nil {
		t.Fatalf("register source: %v", err)
	}

	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	commandSet, err := NewCommandSet(CommandSetConfig{
		Registry:        registry,
		Provider:        provider,
		GenerationStore: newMemoryGenerationStore(),
		Tracker:         NewTracker(WithDispatchStore(store)),
	})
	if err != nil {
		t.Fatalf("new command set: %v", err)
	}

	if queue == nil {
		queue = NewMemoryQueue()
	}
	cancelStore := NewMemoryCancellationStore()
	if enqueuer == nil {
		enqueuer = queue
	}
	client, err := NewClient(ClientConfig{
		Enqueuer:     enqueuer,
		StatusReader: queue,
		CancelStore:  cancelStore,
		Store:        store,
		Registry:     commandSet.Registry,
		Tracker:      commandSet.Tracker,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	runtime, err := NewWorker(WorkerConfig{
		Dequeuer:     queue,
		StatusReader: queue,
		CancelStore:  cancelStore,
		Registry:     commandSet.Registry,
		Tracker:      commandSet.Tracker,
		RetryPolicy:  retryPolicy,
		Options: []worker.Option{
			worker.WithIdleDelay(5 * time.Millisecond),
			worker.WithCancelPollInterval(10 * time.Millisecond),
		},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	return &harness{
		commandSet: commandSet,
		client:     client,
		worker:     runtime,
		provider:   provider,
		queue:      queue,
		cancel:     cancelStore,
		store:      store,
	}
}

func waitForState(t *testing.T, client *Client, dispatchID string, states ...string) DispatchSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, ok, err := client.Get(context.Background(), dispatchID)
		if err != nil {
			t.Fatalf("get snapshot: %v", err)
		}
		if ok {
			if slices.Contains(states, snapshot.State) {
				return snapshot
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %v", states)
	return DispatchSnapshot{}
}

func TestAsyncIndexRecordDuplicateDeliverySafetyAndSummary(t *testing.T) {
	projector := &controllableProjector{}
	h := newHarness(t, map[string]simpleRecord{
		"alpha": {ID: "alpha", Title: "Alpha", Body: "archive alpha"},
	}, projector, worker.DefaultRetryPolicy{MaxAttempts: 2})

	ctx := t.Context()
	if err := h.worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer h.worker.Stop(context.Background())

	first, err := h.client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{})
	if err != nil {
		t.Fatalf("enqueue index record: %v", err)
	}
	firstSnapshot := waitForState(t, h.client, first.DispatchID, string(jobqueue.DispatchStateSucceeded))
	health, err := h.provider.Health(context.Background(), types.HealthRequest{})
	if err != nil {
		t.Fatalf("provider health: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Documents != 1 {
		t.Fatalf("expected single indexed document, got %+v", health.Indexes)
	}
	if firstSnapshot.Summary.Generation == nil || *firstSnapshot.Summary.Generation != 1 {
		t.Fatalf("expected generation summary, got %+v", firstSnapshot.Summary)
	}
	if len(firstSnapshot.Summary.Activities) == 0 {
		t.Fatalf("expected activity summary, got %+v", firstSnapshot.Summary)
	}

	second, err := h.client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{})
	if err != nil {
		t.Fatalf("enqueue duplicate index record: %v", err)
	}
	waitForState(t, h.client, second.DispatchID, string(jobqueue.DispatchStateSucceeded))
	health, err = h.provider.Health(context.Background(), types.HealthRequest{})
	if err != nil {
		t.Fatalf("provider health after duplicate: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Documents != 1 {
		t.Fatalf("expected duplicate delivery to stay convergent, got %+v", health.Indexes)
	}
}

func TestAsyncTrackingSurvivesWorkerCompletionBeforeReceiptBinding(t *testing.T) {
	projector := &controllableProjector{}
	slowQueue := NewMemoryQueue()
	h := newHarnessWithStore(t, map[string]simpleRecord{
		"alpha": {ID: "alpha", Title: "Alpha", Body: "archive alpha"},
	}, projector, worker.DefaultRetryPolicy{MaxAttempts: 1}, NewMemoryDispatchStore(), slowQueue, enqueueDelayQueue{MemoryQueue: slowQueue, delay: 75 * time.Millisecond})

	ctx := t.Context()
	if err := h.worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer h.worker.Stop(context.Background())

	receipt, err := h.client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{})
	if err != nil {
		t.Fatalf("enqueue delayed receipt dispatch: %v", err)
	}
	snapshot := waitForState(t, h.client, receipt.DispatchID, string(jobqueue.DispatchStateSucceeded))
	if snapshot.Summary.Generation == nil || *snapshot.Summary.Generation == 0 {
		t.Fatalf("expected generation summary despite pre-bind completion, got %+v", snapshot.Summary)
	}
	if len(snapshot.Summary.Activities) == 0 {
		t.Fatalf("expected activity summary despite pre-bind completion, got %+v", snapshot.Summary)
	}
}

func TestAsyncReindexPartialBatchRecoveryAndProgress(t *testing.T) {
	projector := &controllableProjector{
		failRemaining: map[string]int{"beta": 1},
	}
	h := newHarness(t, map[string]simpleRecord{
		"alpha": {ID: "alpha", Title: "Alpha", Body: "archive alpha"},
		"beta":  {ID: "beta", Title: "Beta", Body: "archive beta"},
	}, projector, worker.DefaultRetryPolicy{
		MaxAttempts: 2,
		Backoff: worker.BackoffConfig{
			Strategy: worker.BackoffFixed,
			Interval: 10 * time.Millisecond,
		},
	})

	ctx := t.Context()
	if err := h.worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer h.worker.Stop(context.Background())

	receipt, err := h.client.EnqueueReindexIndex(context.Background(), types.ReindexIndexInput{
		Index: "documents",
	}, DispatchOptions{})
	if err != nil {
		t.Fatalf("enqueue reindex: %v", err)
	}
	snapshot := waitForState(t, h.client, receipt.DispatchID, string(jobqueue.DispatchStateSucceeded))

	if snapshot.Attempt < 2 {
		t.Fatalf("expected retry attempt to be recorded, got %+v", snapshot)
	}
	if snapshot.Summary.Completed != 2 {
		t.Fatalf("expected completed reindex summary, got %+v", snapshot.Summary)
	}
	if len(snapshot.Progress.History) < 3 {
		t.Fatalf("expected progress history across retry, got %+v", snapshot.Progress)
	}
	if len(snapshot.Summary.Activities) == 0 || snapshot.Summary.Activities[len(snapshot.Summary.Activities)-1].Verb != "reindexed" {
		t.Fatalf("expected reindexed activity summary, got %+v", snapshot.Summary.Activities)
	}

	health, err := h.provider.Health(context.Background(), types.HealthRequest{})
	if err != nil {
		t.Fatalf("provider health: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Documents != 2 {
		t.Fatalf("expected convergent reindex result, got %+v", health.Indexes)
	}
}

func TestAsyncPauseResumeCancelAndRestart(t *testing.T) {
	block := make(chan struct{})
	projector := &controllableProjector{allow: block}
	h := newHarness(t, map[string]simpleRecord{
		"alpha": {ID: "alpha", Title: "Alpha", Body: "archive alpha"},
	}, projector, worker.DefaultRetryPolicy{MaxAttempts: 1})

	ctx := t.Context()
	if err := h.worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer h.worker.Stop(context.Background())

	if err := h.worker.Pause(); err != nil {
		t.Fatalf("pause worker: %v", err)
	}
	pausedReceipt, err := h.client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{})
	if err != nil {
		t.Fatalf("enqueue while paused: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	pausedSnapshot, ok, err := h.client.Get(context.Background(), pausedReceipt.DispatchID)
	if err != nil {
		t.Fatalf("get paused snapshot: %v", err)
	}
	if !ok || pausedSnapshot.State != string(jobqueue.DispatchStateAccepted) {
		t.Fatalf("expected accepted state while paused, got %+v", pausedSnapshot)
	}
	if err := h.worker.Resume(); err != nil {
		t.Fatalf("resume worker: %v", err)
	}
	if err := h.client.Cancel(context.Background(), CancelRequest{DispatchID: pausedReceipt.DispatchID, Reason: "operator cancel"}); err != nil {
		t.Fatalf("cancel dispatch: %v", err)
	}
	canceled := waitForState(t, h.client, pausedReceipt.DispatchID, string(jobqueue.DispatchStateCanceled))
	if !canceled.CancelRequested {
		t.Fatalf("expected cancel request marker, got %+v", canceled)
	}

	close(block)
	restarted, err := h.client.Restart(context.Background(), pausedReceipt.DispatchID, RestartOptions{})
	if err != nil {
		t.Fatalf("restart dispatch: %v", err)
	}
	waitForState(t, h.client, restarted.DispatchID, string(jobqueue.DispatchStateSucceeded))

	health, err := h.provider.Health(context.Background(), types.HealthRequest{})
	if err != nil {
		t.Fatalf("provider health: %v", err)
	}
	if len(health.Indexes) != 1 || health.Indexes[0].Documents != 1 {
		t.Fatalf("expected restarted job to index one document, got %+v", health.Indexes)
	}
}

func TestAsyncRejectsDuplicateOperationKey(t *testing.T) {
	projector := &controllableProjector{}
	h := newHarness(t, map[string]simpleRecord{
		"alpha": {ID: "alpha", Title: "Alpha", Body: "archive alpha"},
	}, projector, worker.DefaultRetryPolicy{MaxAttempts: 1})

	ctx := t.Context()
	if err := h.worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer h.worker.Stop(context.Background())

	const operationKey = "shared-op"
	first, err := h.client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{OperationKey: operationKey})
	if err != nil {
		t.Fatalf("enqueue first operation key: %v", err)
	}
	if _, err := h.client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{OperationKey: operationKey}); err == nil {
		t.Fatalf("expected duplicate operation key conflict")
	}
	waitForState(t, h.client, first.DispatchID, string(jobqueue.DispatchStateSucceeded))
}

func TestAsyncGetAndRestartSurviveTrackerReplacement(t *testing.T) {
	projector := &controllableProjector{}
	store := NewMemoryDispatchStore()
	h := newHarnessWithStore(t, map[string]simpleRecord{
		"alpha": {ID: "alpha", Title: "Alpha", Body: "archive alpha"},
	}, projector, worker.DefaultRetryPolicy{MaxAttempts: 1}, store, nil, nil)

	ctx := t.Context()
	if err := h.worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	defer h.worker.Stop(context.Background())

	first, err := h.client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{})
	if err != nil {
		t.Fatalf("enqueue first dispatch: %v", err)
	}
	waitForState(t, h.client, first.DispatchID, string(jobqueue.DispatchStateSucceeded))

	replacement, err := NewClient(ClientConfig{
		Enqueuer:     h.queue,
		StatusReader: h.queue,
		CancelStore:  h.cancel,
		Store:        store,
		Registry:     h.commandSet.Registry,
		Tracker:      NewTracker(),
	})
	if err != nil {
		t.Fatalf("new replacement client: %v", err)
	}
	snapshot, ok, err := replacement.Get(context.Background(), first.DispatchID)
	if err != nil {
		t.Fatalf("replacement get: %v", err)
	}
	if !ok || snapshot.DispatchID != first.DispatchID {
		t.Fatalf("expected replacement client to load snapshot from store, got %+v", snapshot)
	}
	restarted, err := replacement.Restart(context.Background(), first.DispatchID, RestartOptions{})
	if err != nil {
		t.Fatalf("replacement restart: %v", err)
	}
	waitForState(t, replacement, restarted.DispatchID, string(jobqueue.DispatchStateSucceeded))
}

func TestOutboxAdapterLifecycle(t *testing.T) {
	queue := NewMemoryQueue()
	adapter := NewOutboxStore(queue.Storage())
	client, err := NewClient(ClientConfig{
		Enqueuer:     queue,
		StatusReader: queue,
		Registry: func() *queuecmd.Registry {
			reg := queuecmd.NewRegistry()
			_ = reg.Register(queuecmd.Entry{
				ID:      "search::index_record",
				Handler: func(context.Context, map[string]any) error { return nil },
			})
			return reg
		}(),
		Tracker: NewTracker(),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	receipt, err := client.EnqueueIndexRecord(context.Background(), types.IndexRecordInput{
		Index:    "documents",
		RecordID: "alpha",
	}, DispatchOptions{})
	if err != nil {
		t.Fatalf("enqueue dispatch: %v", err)
	}

	claimed, err := adapter.ClaimPending(context.Background(), "worker-1", 10, 30*time.Second)
	if err != nil {
		t.Fatalf("claim pending: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != receipt.DispatchID {
		t.Fatalf("expected claimed outbox entry, got %+v", claimed)
	}
	if err := adapter.MarkCompleted(context.Background(), claimed[0].ID, claimed[0].LeaseToken); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	status, err := queue.GetDispatchStatus(context.Background(), receipt.DispatchID)
	if err != nil {
		t.Fatalf("dispatch status: %v", err)
	}
	if status.State != jobqueue.DispatchStateSucceeded {
		t.Fatalf("expected succeeded state after outbox completion, got %+v", status)
	}
}
