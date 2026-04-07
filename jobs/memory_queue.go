package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	job "github.com/goliatone/go-job"
	"github.com/goliatone/go-job/queue"
)

type MemoryQueue struct {
	storage *MemoryStorage
}

type MemoryStorage struct {
	mu     sync.Mutex
	now    func() time.Time
	nextID uint64
	items  map[string]*memoryDispatch
}

type memoryDispatch struct {
	id          string
	message     *job.ExecutionMessage
	attempts    int
	createdAt   time.Time
	updatedAt   time.Time
	availableAt time.Time
	leasedAt    time.Time
	leaseUntil  time.Time
	leaseToken  string
	lastError   string
	state       queue.DispatchState
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{
		storage: &MemoryStorage{
			now:   time.Now,
			items: map[string]*memoryDispatch{},
		},
	}
}

func (q *MemoryQueue) Storage() *MemoryStorage {
	if q == nil {
		return nil
	}
	return q.storage
}

func (q *MemoryQueue) Enqueue(ctx context.Context, msg *job.ExecutionMessage) (queue.EnqueueReceipt, error) {
	return q.storage.Enqueue(ctx, msg)
}

func (q *MemoryQueue) EnqueueAt(ctx context.Context, msg *job.ExecutionMessage, at time.Time) (queue.EnqueueReceipt, error) {
	return q.storage.EnqueueAt(ctx, msg, at)
}

func (q *MemoryQueue) EnqueueAfter(ctx context.Context, msg *job.ExecutionMessage, delay time.Duration) (queue.EnqueueReceipt, error) {
	return q.storage.EnqueueAfter(ctx, msg, delay)
}

func (q *MemoryQueue) Dequeue(ctx context.Context) (queue.Delivery, error) {
	msg, receipt, err := q.storage.Dequeue(ctx)
	if err != nil || msg == nil {
		return nil, err
	}
	return &memoryDelivery{storage: q.storage, message: msg, receipt: receipt}, nil
}

func (q *MemoryQueue) GetDispatchStatus(ctx context.Context, dispatchID string) (queue.DispatchStatus, error) {
	return q.storage.GetDispatchStatus(ctx, dispatchID)
}

func (s *MemoryStorage) Enqueue(_ context.Context, msg *job.ExecutionMessage) (queue.EnqueueReceipt, error) {
	return s.enqueueAt(msg, s.now().UTC())
}

func (s *MemoryStorage) EnqueueAt(_ context.Context, msg *job.ExecutionMessage, at time.Time) (queue.EnqueueReceipt, error) {
	return s.enqueueAt(msg, at.UTC())
}

func (s *MemoryStorage) EnqueueAfter(_ context.Context, msg *job.ExecutionMessage, delay time.Duration) (queue.EnqueueReceipt, error) {
	if delay < 0 {
		return queue.EnqueueReceipt{}, fmt.Errorf("delay must be >= 0")
	}
	return s.enqueueAt(msg, s.now().UTC().Add(delay))
}

func (s *MemoryStorage) Dequeue(_ context.Context) (*job.ExecutionMessage, queue.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	var candidate *memoryDispatch
	for _, item := range s.items {
		if item.leaseToken != "" {
			continue
		}
		if item.state != queue.DispatchStateAccepted && item.state != queue.DispatchStateRetrying {
			continue
		}
		if item.availableAt.After(now) {
			continue
		}
		if candidate == nil || item.availableAt.Before(candidate.availableAt) || (item.availableAt.Equal(candidate.availableAt) && item.id < candidate.id) {
			candidate = item
		}
	}
	if candidate == nil {
		return nil, queue.Receipt{}, nil
	}
	candidate.attempts++
	candidate.leasedAt = now
	candidate.updatedAt = now
	candidate.leaseUntil = now
	candidate.leaseToken = fmt.Sprintf("%s-%d", candidate.id, candidate.attempts)
	candidate.state = queue.DispatchStateRunning
	return cloneExecutionMessage(candidate.message), queue.Receipt{
		ID:          candidate.id,
		Token:       candidate.leaseToken,
		Attempts:    candidate.attempts,
		LeasedAt:    candidate.leasedAt,
		AvailableAt: candidate.availableAt,
		CreatedAt:   candidate.createdAt,
		LastError:   candidate.lastError,
	}, nil
}

func (s *MemoryStorage) Ack(_ context.Context, receipt queue.Receipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[receipt.ID]
	if !ok {
		return queue.ErrDispatchNotFound
	}
	if item.leaseToken != receipt.Token {
		return fmt.Errorf("invalid lease token")
	}
	item.leaseToken = ""
	item.leaseUntil = time.Time{}
	item.updatedAt = s.now().UTC()
	item.state = queue.DispatchStateSucceeded
	return nil
}

func (s *MemoryStorage) Nack(_ context.Context, receipt queue.Receipt, opts queue.NackOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[receipt.ID]
	if !ok {
		return queue.ErrDispatchNotFound
	}
	if item.leaseToken != receipt.Token {
		return fmt.Errorf("invalid lease token")
	}
	item.leaseToken = ""
	item.leaseUntil = time.Time{}
	item.updatedAt = s.now().UTC()
	item.lastError = opts.Reason
	switch opts.Disposition {
	case queue.NackDispositionRetry:
		item.availableAt = item.updatedAt.Add(opts.Delay)
		item.state = queue.DispatchStateRetrying
	case queue.NackDispositionCanceled:
		item.state = queue.DispatchStateCanceled
	case queue.NackDispositionDeadLetter:
		item.state = queue.DispatchStateDeadLetter
	default:
		item.state = queue.DispatchStateFailed
	}
	return nil
}

func (s *MemoryStorage) ExtendLease(_ context.Context, receipt queue.Receipt, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[receipt.ID]
	if !ok {
		return queue.ErrDispatchNotFound
	}
	if item.leaseToken != receipt.Token {
		return fmt.Errorf("invalid lease token")
	}
	now := s.now().UTC()
	item.updatedAt = now
	item.leaseUntil = now.Add(ttl)
	return nil
}

func (s *MemoryStorage) GetDispatchStatus(_ context.Context, dispatchID string) (queue.DispatchStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[dispatchID]
	if !ok {
		return queue.DispatchStatus{}, queue.ErrDispatchNotFound
	}
	enqueuedAt := item.createdAt
	updatedAt := item.updatedAt
	var nextRunAt *time.Time
	if item.state == queue.DispatchStateRetrying {
		value := item.availableAt
		nextRunAt = &value
	}
	return queue.DispatchStatus{
		DispatchID:     item.id,
		State:          item.state,
		Attempt:        item.attempts,
		EnqueuedAt:     &enqueuedAt,
		UpdatedAt:      &updatedAt,
		NextRunAt:      nextRunAt,
		TerminalReason: item.lastError,
	}, nil
}

func (s *MemoryStorage) enqueueAt(msg *job.ExecutionMessage, at time.Time) (queue.EnqueueReceipt, error) {
	if s == nil {
		return queue.EnqueueReceipt{}, fmt.Errorf("memory storage not configured")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	now := s.now().UTC()
	id := fmt.Sprintf("dispatch-%06d", s.nextID)
	s.items[id] = &memoryDispatch{
		id:          id,
		message:     cloneExecutionMessage(msg),
		createdAt:   now,
		updatedAt:   now,
		availableAt: at,
		state:       queue.DispatchStateAccepted,
	}
	return queue.EnqueueReceipt{DispatchID: id, EnqueuedAt: now}, nil
}

type memoryDelivery struct {
	storage *MemoryStorage
	message *job.ExecutionMessage
	receipt queue.Receipt
}

func (d *memoryDelivery) Message() *job.ExecutionMessage {
	return cloneExecutionMessage(d.message)
}

func (d *memoryDelivery) Ack(ctx context.Context) error {
	return d.storage.Ack(ctx, d.receipt)
}

func (d *memoryDelivery) Nack(ctx context.Context, opts queue.NackOptions) error {
	return d.storage.Nack(ctx, d.receipt, opts)
}

func (d *memoryDelivery) ExtendLease(ctx context.Context, ttl time.Duration) error {
	return d.storage.ExtendLease(ctx, d.receipt, ttl)
}

func (d *memoryDelivery) Attempts() int {
	return d.receipt.Attempts
}

func cloneExecutionMessage(msg *job.ExecutionMessage) *job.ExecutionMessage {
	if msg == nil {
		return nil
	}
	out := *msg
	out.Parameters = cloneStringAnyMap(msg.Parameters)
	if msg.Result != nil {
		value := *msg.Result
		out.Result = &value
	}
	return &out
}
