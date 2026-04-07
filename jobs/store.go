package jobs

import "context"

type DispatchStore interface {
	Upsert(ctx context.Context, snapshot DispatchSnapshot) error
	Get(ctx context.Context, dispatchID string) (DispatchSnapshot, bool, error)
	GetByOperationKey(ctx context.Context, operationKey string) (DispatchSnapshot, bool, error)
	ListBatch(ctx context.Context, batchID string) ([]DispatchSnapshot, error)
}

type MemoryDispatchStore struct {
	tracker *Tracker
}

func NewMemoryDispatchStore() *MemoryDispatchStore {
	return &MemoryDispatchStore{tracker: NewTracker()}
}

func (s *MemoryDispatchStore) Upsert(_ context.Context, snapshot DispatchSnapshot) error {
	if s == nil {
		return nil
	}
	s.tracker.upsertSnapshot(snapshot)
	return nil
}

func (s *MemoryDispatchStore) Get(_ context.Context, dispatchID string) (DispatchSnapshot, bool, error) {
	if s == nil {
		return DispatchSnapshot{}, false, nil
	}
	snapshot, ok := s.tracker.lookupByDispatchID(dispatchID)
	return snapshot, ok, nil
}

func (s *MemoryDispatchStore) GetByOperationKey(_ context.Context, operationKey string) (DispatchSnapshot, bool, error) {
	if s == nil {
		return DispatchSnapshot{}, false, nil
	}
	snapshot, ok := s.tracker.lookupByOperationKey(operationKey)
	return snapshot, ok, nil
}

func (s *MemoryDispatchStore) ListBatch(_ context.Context, batchID string) ([]DispatchSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	return s.tracker.lookupBatch(batchID), nil
}
