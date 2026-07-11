package ranking

import (
	"context"
	"fmt"
	"github.com/goliatone/go-search/pkg/types"
	"sync"
)

type SignalInput struct {
	Request types.SearchRequest
	Hit     types.SearchHit
	Profile RankingProfile
	Now     types.Clock
}
type SignalResult struct {
	Value  float64
	Reason string
}
type Signal interface {
	Evaluate(context.Context, SignalInput) (SignalResult, error)
}
type SignalFunc func(context.Context, SignalInput) (SignalResult, error)

func (f SignalFunc) Evaluate(ctx context.Context, in SignalInput) (SignalResult, error) {
	return f(ctx, in)
}

type SignalRegistry struct {
	mu     sync.RWMutex
	items  map[string]Signal
	sealed bool
}

func NewSignalRegistry() *SignalRegistry { return &SignalRegistry{items: map[string]Signal{}} }
func (r *SignalRegistry) Register(id string, s Signal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("registry is sealed")
	}
	if id == "" || s == nil {
		return fmt.Errorf("signal id and implementation are required")
	}
	if _, ok := r.items[id]; ok {
		return fmt.Errorf("duplicate signal %q", id)
	}
	r.items[id] = s
	return nil
}
func (r *SignalRegistry) Seal() { r.mu.Lock(); r.sealed = true; r.mu.Unlock() }
func (r *SignalRegistry) Resolve(id string) (Signal, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.items[id]
	return s, ok
}

type SignalSpec struct {
	ID                                       string
	Weight, MinContribution, MaxContribution float64
}

func EvaluateSignals(ctx context.Context, registry *SignalRegistry, specs []SignalSpec, in SignalInput) (float64, []types.AppliedSignalContribution, error) {
	total := 0.0
	out := make([]types.AppliedSignalContribution, 0, len(specs))
	for _, spec := range specs {
		s, ok := registry.Resolve(spec.ID)
		if !ok {
			return 0, nil, fmt.Errorf("unknown signal %q", spec.ID)
		}
		result, err := s.Evaluate(ctx, in)
		if err != nil {
			return 0, nil, err
		}
		c := result.Value * spec.Weight
		if c < spec.MinContribution {
			c = spec.MinContribution
		}
		if c > spec.MaxContribution {
			c = spec.MaxContribution
		}
		total += c
		out = append(out, types.AppliedSignalContribution{ID: spec.ID, Value: result.Value, Contribution: c, Reason: result.Reason})
	}
	return total, out, nil
}
