package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/goliatone/go-job/queue"
)

type failingDispatchStore struct{ err error }

func (s failingDispatchStore) Upsert(context.Context, DispatchSnapshot) error { return s.err }
func (failingDispatchStore) Get(context.Context, string) (DispatchSnapshot, bool, error) {
	return DispatchSnapshot{}, false, nil
}
func (failingDispatchStore) GetByOperationKey(context.Context, string) (DispatchSnapshot, bool, error) {
	return DispatchSnapshot{}, false, nil
}
func (failingDispatchStore) ListBatch(context.Context, string) ([]DispatchSnapshot, error) {
	return nil, nil
}

func TestMemoryDispatchStoreRejectsStaleSnapshots(t *testing.T) {
	store := NewMemoryDispatchStore()
	ctx := context.Background()
	now := time.Now().UTC()
	newest := DispatchSnapshot{DispatchID: "dispatch-1", OperationKey: "operation-1", Revision: 4, State: string(queue.DispatchStateSucceeded), UpdatedAt: &now}
	if err := store.Upsert(ctx, newest); err != nil {
		t.Fatal(err)
	}
	older := newest
	older.Revision = 3
	older.State = string(queue.DispatchStateRunning)
	if err := store.Upsert(ctx, older); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(ctx, newest.DispatchID)
	if err != nil || !ok {
		t.Fatalf("get snapshot: ok=%v err=%v", ok, err)
	}
	if got.Revision != newest.Revision || got.State != newest.State {
		t.Fatalf("stale snapshot replaced terminal state: %+v", got)
	}
}

func TestTrackerSurfacesAsynchronousPersistenceErrors(t *testing.T) {
	want := errors.New("persistence unavailable")
	reported := make(chan error, 1)
	tracker := NewTracker(
		WithDispatchStore(failingDispatchStore{err: want}),
		WithPersistenceErrorHandler(func(_ context.Context, _ DispatchSnapshot, err error) { reported <- err }),
	)
	ctx := context.Background()
	if err := tracker.Prepare(ctx, DispatchSnapshot{OperationKey: "operation-1"}); err != nil {
		t.Fatal(err)
	}
	bindErr := tracker.Bind(ctx, "operation-1", DispatchReceipt{DispatchID: "dispatch-1", OperationKey: "operation-1", EnqueuedAt: time.Now().UTC()})
	if !errors.Is(bindErr, want) {
		t.Fatalf("bind error = %v, want %v", bindErr, want)
	}
	select {
	case got := <-reported:
		if !errors.Is(got, want) {
			t.Fatalf("reported error = %v", got)
		}
	default:
		t.Fatal("persistence error handler was not called")
	}
	if !errors.Is(tracker.PersistenceError(), want) {
		t.Fatalf("last persistence error = %v", tracker.PersistenceError())
	}
}

func TestTrackerIgnoresOlderExternalStatus(t *testing.T) {
	tracker := NewTracker()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := tracker.Prepare(ctx, DispatchSnapshot{OperationKey: "operation-1"}); err != nil {
		t.Fatal(err)
	}
	if err := tracker.Bind(ctx, "operation-1", DispatchReceipt{DispatchID: "dispatch-1", OperationKey: "operation-1", EnqueuedAt: now}); err != nil {
		t.Fatal(err)
	}
	tracker.MarkSucceeded(ctx, "operation-1", 1)
	older := now.Add(-time.Minute)
	tracker.UpdateStatus(ctx, "dispatch-1", queue.DispatchStatus{State: queue.DispatchStateRunning, UpdatedAt: &older})
	got, ok := tracker.lookupByDispatchID("dispatch-1")
	if !ok || got.State != string(queue.DispatchStateSucceeded) {
		t.Fatalf("older status regressed snapshot: %+v", got)
	}
}
