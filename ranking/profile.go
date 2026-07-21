package ranking

import (
	"fmt"
	"maps"
	"math"
	"strings"
	"sync"

	"github.com/goliatone/go-search/pkg/types"
)

const MaxIndexWeight = 100.0

type CandidateConfig struct{ Multiplier, MaxPerIndex, MaxRefillRounds int }
type IndexProfile struct {
	QueryFields []types.QueryField
	Weight      float64
}
type ImplementationRef struct {
	ID      string
	Options map[string]any
}
type RankingProfile struct {
	Name, Version string
	Indexes       map[string]IndexProfile
	Candidates    CandidateConfig
	Fusion        ImplementationRef
	Signals       []ImplementationRef
}

func (p RankingProfile) Validate() error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Version) == "" {
		return fmt.Errorf("profile name and version are required")
	}
	if p.Candidates.Multiplier < 1 || p.Candidates.MaxPerIndex < 1 || p.Candidates.MaxRefillRounds < 1 {
		return fmt.Errorf("candidate bounds must be positive")
	}
	for index, cfg := range p.Indexes {
		if strings.TrimSpace(index) == "" {
			return fmt.Errorf("index name is required")
		}
		if math.IsNaN(cfg.Weight) || math.IsInf(cfg.Weight, 0) || cfg.Weight < 0 || cfg.Weight > MaxIndexWeight {
			return fmt.Errorf("index weight for %q must be finite, non-negative, and at most %.4g", index, MaxIndexWeight)
		}
		seen := map[string]bool{}
		for _, field := range cfg.QueryFields {
			field.Field = strings.TrimSpace(field.Field)
			if field.Field == "" || field.Weight < 1 || seen[field.Field] {
				return fmt.Errorf("invalid or duplicate query field %q for index %q", field.Field, index)
			}
			seen[field.Field] = true
		}
	}
	return nil
}

type ProfileRegistry interface {
	Resolve(string) (RankingProfile, bool)
	Default() RankingProfile
}
type Registry struct {
	mu              sync.RWMutex
	profiles        map[string]RankingProfile
	implementations map[string]struct{}
	defaultName     string
	sealed          bool
}

func NewRegistry(defaultName string) *Registry {
	return &Registry{profiles: map[string]RankingProfile{}, implementations: map[string]struct{}{}, defaultName: strings.TrimSpace(defaultName)}
}
func (r *Registry) RegisterImplementation(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("implementation id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("registry is sealed")
	}
	if _, ok := r.implementations[id]; ok {
		return fmt.Errorf("duplicate implementation %q", id)
	}
	r.implementations[id] = struct{}{}
	return nil
}
func (r *Registry) Register(p RankingProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("registry is sealed")
	}
	if _, ok := r.profiles[p.Name]; ok {
		return fmt.Errorf("duplicate profile %q", p.Name)
	}
	for _, ref := range append([]ImplementationRef{p.Fusion}, p.Signals...) {
		if ref.ID != "" {
			if _, ok := r.implementations[ref.ID]; !ok {
				return fmt.Errorf("unknown implementation %q", ref.ID)
			}
		}
	}
	r.profiles[p.Name] = cloneProfile(p)
	return nil
}
func (r *Registry) Seal() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.defaultName != "" {
		if _, ok := r.profiles[r.defaultName]; !ok {
			return fmt.Errorf("default profile %q is not registered", r.defaultName)
		}
	}
	r.sealed = true
	return nil
}
func (r *Registry) Resolve(name string) (RankingProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[name]
	return cloneProfile(p), ok
}
func (r *Registry) Default() RankingProfile { p, _ := r.Resolve(r.defaultName); return p }
func cloneProfile(p RankingProfile) RankingProfile {
	out := p
	out.Indexes = make(map[string]IndexProfile, len(p.Indexes))
	for name, cfg := range p.Indexes {
		cfg.QueryFields = append([]types.QueryField(nil), cfg.QueryFields...)
		out.Indexes[name] = cfg
	}
	out.Fusion.Options = cloneProfileOptions(p.Fusion.Options)
	out.Signals = make([]ImplementationRef, len(p.Signals))
	for i, signal := range p.Signals {
		signal.Options = cloneProfileOptions(signal.Options)
		out.Signals[i] = signal
	}
	return out
}

func cloneProfileOptions(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneProfileOptionValue(value)
	}
	return out
}

func cloneProfileOptionValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneProfileOptions(typed)
	case map[string]string:
		out := make(map[string]string, len(typed))
		maps.Copy(out, typed)
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneProfileOptionValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	default:
		return value
	}
}

func LegacyQueryFields(def types.IndexDefinition) []types.QueryField {
	if len(def.DefaultWeightedQueryFields) > 0 {
		return append([]types.QueryField(nil), def.DefaultWeightedQueryFields...)
	}
	out := make([]types.QueryField, 0, len(def.DefaultQueryFields))
	for _, f := range def.DefaultQueryFields {
		out = append(out, types.QueryField{Field: f, Weight: 1})
	}
	return out
}
