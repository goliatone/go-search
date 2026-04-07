package jobs

import (
	"context"
	"sync"

	"github.com/goliatone/go-job/queue/cancellation"
)

type MemoryCancellationStore struct {
	mu    sync.RWMutex
	items map[string]cancellation.Request
	subs  []chan cancellation.Request
}

func NewMemoryCancellationStore() *MemoryCancellationStore {
	return &MemoryCancellationStore{items: map[string]cancellation.Request{}}
}

func (s *MemoryCancellationStore) Request(_ context.Context, req cancellation.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[req.Key] = req
	for _, ch := range s.subs {
		select {
		case ch <- req:
		default:
		}
	}
	return nil
}

func (s *MemoryCancellationStore) Get(_ context.Context, key string) (cancellation.Request, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.items[key]
	return req, ok, nil
}

func (s *MemoryCancellationStore) Subscribe(ctx context.Context) (<-chan cancellation.Request, error) {
	ch := make(chan cancellation.Request, 1)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		defer s.mu.Unlock()
		for i := range s.subs {
			if s.subs[i] == ch {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				break
			}
		}
		close(ch)
	}()
	return ch, nil
}
