package jobs

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-job/queue"
	"github.com/goliatone/go-search/pkg/types"
)

type Tracker struct {
	mu             sync.RWMutex
	now            func() time.Time
	store          DispatchStore
	byDispatchID   map[string]*DispatchSnapshot
	byOperationKey map[string]*DispatchSnapshot
	batches        map[string][]string
}

type TrackerOption func(*Tracker)

func WithDispatchStore(store DispatchStore) TrackerOption {
	return func(t *Tracker) {
		t.store = store
	}
}

func NewTracker(opts ...TrackerOption) *Tracker {
	t := &Tracker{
		now:            time.Now,
		byDispatchID:   map[string]*DispatchSnapshot{},
		byOperationKey: map[string]*DispatchSnapshot{},
		batches:        map[string][]string{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

func (t *Tracker) SetStore(store DispatchStore) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.store = store
	t.mu.Unlock()
}

func (t *Tracker) Prepare(ctx context.Context, draft DispatchSnapshot) error {
	if t == nil {
		return nil
	}
	operationKey := strings.TrimSpace(draft.OperationKey)
	if operationKey == "" {
		return nil
	}
	draft.DispatchID = strings.TrimSpace(draft.DispatchID)
	if draft.State == "" {
		draft.State = string(queue.DispatchStateAccepted)
	}
	now := t.now().UTC()
	if draft.EnqueuedAt == nil {
		draft.EnqueuedAt = &now
	}
	if draft.UpdatedAt == nil {
		draft.UpdatedAt = &now
	}

	t.mu.Lock()
	if _, exists := t.byOperationKey[operationKey]; exists {
		t.mu.Unlock()
		return queueConflictError(operationKey)
	}
	if t.store != nil {
		if snapshot, ok, err := t.store.GetByOperationKey(ctx, operationKey); err != nil {
			t.mu.Unlock()
			return err
		} else if ok && snapshot.OperationKey != "" {
			t.mu.Unlock()
			return queueConflictError(operationKey)
		}
	}
	snapshot := cloneDispatchSnapshot(draft)
	t.indexSnapshotLocked(&snapshot)
	t.mu.Unlock()
	return nil
}

func (t *Tracker) Bind(ctx context.Context, operationKey string, receipt DispatchReceipt) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	snapshot := t.byOperationKey[strings.TrimSpace(operationKey)]
	if snapshot == nil {
		t.mu.Unlock()
		return nil
	}
	snapshot.DispatchID = receipt.DispatchID
	snapshot.CommandID = receipt.CommandID
	snapshot.Operation = receipt.Operation
	snapshot.CorrelationID = receipt.CorrelationID
	snapshot.BatchID = receipt.BatchID
	snapshot.BatchPosition = receipt.BatchPosition
	enqueuedAt := receipt.EnqueuedAt.UTC()
	snapshot.EnqueuedAt = &enqueuedAt
	snapshot.UpdatedAt = &enqueuedAt
	t.indexSnapshotLocked(snapshot)
	cloned := cloneDispatchSnapshot(*snapshot)
	store := t.store
	t.mu.Unlock()
	if store != nil && cloned.DispatchID != "" {
		return store.Upsert(ctx, cloned)
	}
	return nil
}

func (t *Tracker) Abandon(operationKey string) {
	if t == nil || strings.TrimSpace(operationKey) == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := t.byOperationKey[strings.TrimSpace(operationKey)]
	if snapshot == nil || snapshot.DispatchID != "" {
		return
	}
	delete(t.byOperationKey, strings.TrimSpace(operationKey))
}

func (t *Tracker) RecordProgress(ctx context.Context, operationKey string, update types.ProgressUpdate) {
	if t == nil || strings.TrimSpace(operationKey) == "" {
		return
	}
	t.mu.Lock()
	snapshot := t.byOperationKey[strings.TrimSpace(operationKey)]
	if snapshot == nil {
		t.mu.Unlock()
		return
	}
	clonedUpdate := cloneProgressUpdate(update)
	snapshot.Progress.Current = &clonedUpdate
	snapshot.Progress.History = append(snapshot.Progress.History, clonedUpdate)
	snapshot.Summary.Completed = clonedUpdate.Completed
	if clonedUpdate.Total > 0 {
		snapshot.Summary.Total = clonedUpdate.Total
	}
	if snapshot.Summary.Index == "" && clonedUpdate.Index != "" {
		snapshot.Summary.Index = clonedUpdate.Index
	}
	now := t.now().UTC()
	snapshot.UpdatedAt = &now
	cloned := cloneDispatchSnapshot(*snapshot)
	store := t.store
	t.mu.Unlock()
	t.persist(ctx, store, cloned)
}

func (t *Tracker) RecordGeneration(ctx context.Context, operationKey, index string, generation int64) {
	if t == nil || strings.TrimSpace(operationKey) == "" {
		return
	}
	t.mu.Lock()
	snapshot := t.byOperationKey[strings.TrimSpace(operationKey)]
	if snapshot == nil {
		t.mu.Unlock()
		return
	}
	value := generation
	snapshot.Summary.Generation = &value
	if snapshot.Summary.Index == "" {
		snapshot.Summary.Index = index
	}
	now := t.now().UTC()
	snapshot.UpdatedAt = &now
	cloned := cloneDispatchSnapshot(*snapshot)
	store := t.store
	t.mu.Unlock()
	t.persist(ctx, store, cloned)
}

func (t *Tracker) RecordActivity(ctx context.Context, operationKey string, event types.ActivityEvent) {
	if t == nil || strings.TrimSpace(operationKey) == "" {
		return
	}
	t.mu.Lock()
	snapshot := t.byOperationKey[strings.TrimSpace(operationKey)]
	if snapshot == nil {
		t.mu.Unlock()
		return
	}
	clonedEvent := cloneActivityEvent(event)
	snapshot.Summary.Activities = append(snapshot.Summary.Activities, clonedEvent)
	if len(snapshot.Summary.Activities) > 0 {
		last := snapshot.Summary.Activities[len(snapshot.Summary.Activities)-1]
		switch last.Verb {
		case "indexed":
			if count, ok := intFromAny(last.Metadata["documents"]); ok {
				snapshot.Summary.Documents = count
			}
		case "reindexed":
			if count, ok := intFromAny(last.Metadata["documents"]); ok {
				snapshot.Summary.Documents = count
				snapshot.Summary.Completed = count
			}
		}
	}
	now := t.now().UTC()
	snapshot.UpdatedAt = &now
	cloned := cloneDispatchSnapshot(*snapshot)
	store := t.store
	t.mu.Unlock()
	t.persist(ctx, store, cloned)
}

func (t *Tracker) MarkStarted(ctx context.Context, operationKey string, attempt int) {
	t.markAttemptState(ctx, operationKey, queue.DispatchStateRunning, attempt, "", nil)
}

func (t *Tracker) MarkRetry(ctx context.Context, operationKey string, attempt int, err error, nextRun *time.Time) {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	t.markAttemptState(ctx, operationKey, queue.DispatchStateRetrying, attempt, reason, nextRun)
}

func (t *Tracker) MarkSucceeded(ctx context.Context, operationKey string, attempt int) {
	t.markAttemptState(ctx, operationKey, queue.DispatchStateSucceeded, attempt, "", nil)
}

func (t *Tracker) MarkFailed(ctx context.Context, operationKey string, attempt int, err error, state queue.DispatchState) {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	t.markAttemptState(ctx, operationKey, state, attempt, reason, nil)
}

func (t *Tracker) MarkCancelRequested(ctx context.Context, dispatchID string) {
	if t == nil || strings.TrimSpace(dispatchID) == "" {
		return
	}
	t.mu.Lock()
	snapshot := t.byDispatchID[strings.TrimSpace(dispatchID)]
	if snapshot == nil {
		t.mu.Unlock()
		return
	}
	snapshot.CancelRequested = true
	now := t.now().UTC()
	snapshot.UpdatedAt = &now
	cloned := cloneDispatchSnapshot(*snapshot)
	store := t.store
	t.mu.Unlock()
	t.persist(ctx, store, cloned)
}

func (t *Tracker) UpdateStatus(ctx context.Context, dispatchID string, status queue.DispatchStatus) {
	if t == nil || strings.TrimSpace(dispatchID) == "" {
		return
	}
	t.mu.Lock()
	snapshot := t.byDispatchID[strings.TrimSpace(dispatchID)]
	if snapshot == nil {
		t.mu.Unlock()
		return
	}
	snapshot.State = string(status.State)
	snapshot.Attempt = status.Attempt
	snapshot.LastError = status.TerminalReason
	snapshot.TerminalReason = status.TerminalReason
	if status.EnqueuedAt != nil {
		value := status.EnqueuedAt.UTC()
		snapshot.EnqueuedAt = &value
	}
	if status.UpdatedAt != nil {
		value := status.UpdatedAt.UTC()
		snapshot.UpdatedAt = &value
	}
	if status.NextRunAt != nil {
		value := status.NextRunAt.UTC()
		snapshot.NextRunAt = &value
	} else {
		snapshot.NextRunAt = nil
	}
	cloned := cloneDispatchSnapshot(*snapshot)
	store := t.store
	t.mu.Unlock()
	t.persist(ctx, store, cloned)
}

func (t *Tracker) Get(ctx context.Context, dispatchID string, reader queue.DispatchStatusReader) (DispatchSnapshot, bool, error) {
	if t == nil || strings.TrimSpace(dispatchID) == "" {
		return DispatchSnapshot{}, false, nil
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if reader != nil {
		status, err := reader.GetDispatchStatus(ctx, dispatchID)
		if err != nil && err != queue.ErrDispatchNotFound {
			return DispatchSnapshot{}, false, err
		}
		if err == nil {
			t.UpdateStatus(ctx, dispatchID, status)
		}
	}
	if snapshot, ok := t.lookupByDispatchID(dispatchID); ok {
		return snapshot, true, nil
	}
	if t.store != nil {
		snapshot, ok, err := t.store.Get(ctx, dispatchID)
		if err != nil {
			return DispatchSnapshot{}, false, err
		}
		if ok {
			t.upsertSnapshot(snapshot)
			return snapshot, true, nil
		}
	}
	return DispatchSnapshot{}, false, nil
}

func (t *Tracker) ListBatch(ctx context.Context, batchID string, reader queue.DispatchStatusReader) ([]DispatchSnapshot, error) {
	if t == nil || strings.TrimSpace(batchID) == "" {
		return nil, nil
	}
	batchID = strings.TrimSpace(batchID)
	out := t.lookupBatch(batchID)
	if len(out) == 0 && t.store != nil {
		stored, err := t.store.ListBatch(ctx, batchID)
		if err != nil {
			return nil, err
		}
		for _, snapshot := range stored {
			t.upsertSnapshot(snapshot)
		}
		out = stored
	}
	if reader != nil {
		for i := range out {
			status, err := reader.GetDispatchStatus(ctx, out[i].DispatchID)
			if err != nil && err != queue.ErrDispatchNotFound {
				return nil, err
			}
			if err == nil {
				t.UpdateStatus(ctx, out[i].DispatchID, status)
				updated, ok := t.lookupByDispatchID(out[i].DispatchID)
				if ok {
					out[i] = updated
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b DispatchSnapshot) int {
		switch {
		case a.BatchPosition < b.BatchPosition:
			return -1
		case a.BatchPosition > b.BatchPosition:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (t *Tracker) lookupByDispatchID(dispatchID string) (DispatchSnapshot, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	snapshot := t.byDispatchID[strings.TrimSpace(dispatchID)]
	if snapshot == nil {
		return DispatchSnapshot{}, false
	}
	return cloneDispatchSnapshot(*snapshot), true
}

func (t *Tracker) lookupByOperationKey(operationKey string) (DispatchSnapshot, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	snapshot := t.byOperationKey[strings.TrimSpace(operationKey)]
	if snapshot == nil {
		return DispatchSnapshot{}, false
	}
	return cloneDispatchSnapshot(*snapshot), true
}

func (t *Tracker) lookupBatch(batchID string) []DispatchSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := append([]string(nil), t.batches[strings.TrimSpace(batchID)]...)
	out := make([]DispatchSnapshot, 0, len(ids))
	for _, id := range ids {
		snapshot := t.byDispatchID[id]
		if snapshot == nil {
			continue
		}
		out = append(out, cloneDispatchSnapshot(*snapshot))
	}
	return out
}

func (t *Tracker) upsertSnapshot(snapshot DispatchSnapshot) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	copy := cloneDispatchSnapshot(snapshot)
	t.indexSnapshotLocked(&copy)
}

func (t *Tracker) indexSnapshotLocked(snapshot *DispatchSnapshot) {
	if snapshot == nil {
		return
	}
	if snapshot.OperationKey != "" {
		t.byOperationKey[snapshot.OperationKey] = snapshot
	}
	if snapshot.DispatchID != "" {
		t.byDispatchID[snapshot.DispatchID] = snapshot
	}
	if snapshot.BatchID != "" && snapshot.DispatchID != "" {
		if !slices.Contains(t.batches[snapshot.BatchID], snapshot.DispatchID) {
			t.batches[snapshot.BatchID] = append(t.batches[snapshot.BatchID], snapshot.DispatchID)
		}
	}
}

func (t *Tracker) markAttemptState(ctx context.Context, operationKey string, state queue.DispatchState, attempt int, reason string, nextRun *time.Time) {
	if t == nil || strings.TrimSpace(operationKey) == "" {
		return
	}
	t.mu.Lock()
	snapshot := t.byOperationKey[strings.TrimSpace(operationKey)]
	if snapshot == nil {
		t.mu.Unlock()
		return
	}
	snapshot.State = string(state)
	snapshot.Attempt = attempt
	snapshot.LastError = reason
	snapshot.TerminalReason = reason
	if state == queue.DispatchStateRunning || state == queue.DispatchStateSucceeded {
		snapshot.LastError = ""
		snapshot.TerminalReason = ""
	}
	now := t.now().UTC()
	snapshot.UpdatedAt = &now
	if nextRun != nil {
		value := nextRun.UTC()
		snapshot.NextRunAt = &value
	} else {
		snapshot.NextRunAt = nil
	}
	cloned := cloneDispatchSnapshot(*snapshot)
	store := t.store
	t.mu.Unlock()
	t.persist(ctx, store, cloned)
}

func (t *Tracker) persist(ctx context.Context, store DispatchStore, snapshot DispatchSnapshot) {
	if store == nil || snapshot.DispatchID == "" {
		return
	}
	_ = store.Upsert(ctx, snapshot)
}

func queueConflictError(operationKey string) error {
	return queueConflict{operationKey: operationKey}
}

type queueConflict struct {
	operationKey string
}

func (e queueConflict) Error() string {
	return "operation key already exists: " + e.operationKey
}

func cloneDispatchSnapshot(in DispatchSnapshot) DispatchSnapshot {
	out := in
	if in.EnqueuedAt != nil {
		value := *in.EnqueuedAt
		out.EnqueuedAt = &value
	}
	if in.UpdatedAt != nil {
		value := *in.UpdatedAt
		out.UpdatedAt = &value
	}
	if in.NextRunAt != nil {
		value := *in.NextRunAt
		out.NextRunAt = &value
	}
	out.Payload = cloneStringAnyMap(in.Payload)
	out.Metadata = cloneStringAnyMap(in.Metadata)
	out.Progress = DispatchProgress{}
	if in.Progress.Current != nil {
		value := cloneProgressUpdate(*in.Progress.Current)
		out.Progress.Current = &value
	}
	out.Progress.History = make([]types.ProgressUpdate, len(in.Progress.History))
	for i, item := range in.Progress.History {
		out.Progress.History[i] = cloneProgressUpdate(item)
	}
	out.Summary = in.Summary
	if in.Summary.Generation != nil {
		value := *in.Summary.Generation
		out.Summary.Generation = &value
	}
	out.Summary.Activities = make([]types.ActivityEvent, len(in.Summary.Activities))
	for i, item := range in.Summary.Activities {
		out.Summary.Activities[i] = cloneActivityEvent(item)
	}
	return out
}

func seedSummary(payload map[string]any) DispatchSummary {
	return DispatchSummary{
		Index:           stringFromMap(payload, "index"),
		RegistrationKey: stringFromMap(payload, "registration_key"),
		RecordID:        stringFromMap(payload, "record_id"),
		BatchSize:       intFromMap(payload, "batch_size"),
	}
}

func cloneProgressUpdate(in types.ProgressUpdate) types.ProgressUpdate {
	out := in
	out.Metadata = cloneStringAnyMap(in.Metadata)
	return out
}

func cloneActivityEvent(in types.ActivityEvent) types.ActivityEvent {
	out := in
	out.Metadata = cloneStringAnyMap(in.Metadata)
	return out
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func stringFromMap(input map[string]any, key string) string {
	if len(input) == 0 {
		return ""
	}
	value, _ := input[key].(string)
	return value
}

func intFromMap(input map[string]any, key string) int {
	value, ok := intFromAny(input[key])
	if !ok {
		return 0
	}
	return value
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	default:
		return 0, false
	}
}
