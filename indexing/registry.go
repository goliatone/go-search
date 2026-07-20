package indexing

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type RecordIndexer interface {
	IndexName() string
	Definition() types.IndexDefinition
	SourceType() string
	IndexRecord(ctx context.Context, recordID string) ([]types.Document, error)
	DeleteSourceIDs(ctx context.Context, recordID string) ([]string, error)
	ListRecordIDs(ctx context.Context, limit int, cursor string) ([]string, string, error)
}

type ActivityEventResolver interface {
	ResolveActivityEvent(ctx context.Context, verb, recordID string, docs []types.Document, metadata map[string]any) (types.ActivityEvent, error)
}

type registrationKeyer interface {
	RegistrationKey() string
}

type RegisteredSource struct {
	Index           string
	RegistrationKey string
	SourceType      string
	Definition      types.IndexDefinition
	Indexer         RecordIndexer
}

type Registry struct {
	mu            sync.RWMutex
	indexes       map[string]types.IndexDefinition
	registrations map[string][]RegisteredSource
}

func NewRegistry() *Registry {
	return &Registry{
		indexes:       map[string]types.IndexDefinition{},
		registrations: map[string][]RegisteredSource{},
	}
}

func (r *Registry) Register(def types.IndexDefinition, indexer RecordIndexer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	def = def.Clone()
	r.indexes[def.Name] = def
	if indexer == nil {
		return nil
	}
	registration := RegisteredSource{
		Index:           def.Name,
		RegistrationKey: registrationKeyFor(indexer),
		SourceType:      strings.TrimSpace(indexer.SourceType()),
		Definition:      def,
		Indexer:         indexer,
	}
	if registration.RegistrationKey == "" {
		return errs.InvalidInput("registration key is required", map[string]any{"index": def.Name})
	}
	items := append([]RegisteredSource(nil), r.registrations[def.Name]...)
	replaced := false
	for i := range items {
		if items[i].RegistrationKey != registration.RegistrationKey {
			continue
		}
		items[i] = registration
		replaced = true
		break
	}
	if !replaced {
		items = append(items, registration)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].RegistrationKey == items[j].RegistrationKey {
			return items[i].SourceType < items[j].SourceType
		}
		return items[i].RegistrationKey < items[j].RegistrationKey
	})
	r.registrations[def.Name] = items
	return nil
}

func (r *Registry) GetIndex(name string) (types.IndexDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.indexes[name]
	return def.Clone(), ok
}

func (r *Registry) MustIndexer(name string, registrationKey string) (RecordIndexer, error) {
	registration, err := r.ResolveRegistration(name, registrationKey)
	if err != nil {
		return nil, err
	}
	return registration.Indexer, nil
}

func (r *Registry) ResolveRegistration(index string, registrationKey string) (RegisteredSource, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.registrations[index]
	if len(items) == 0 {
		return RegisteredSource{}, errs.IndexingSourceMissing(index, nil)
	}
	key := strings.TrimSpace(registrationKey)
	if key == "" {
		if len(items) == 1 {
			return cloneRegisteredSource(items[0]), nil
		}
		return RegisteredSource{}, errs.InvalidInput("registration key is required when an index has multiple registrations", map[string]any{
			"index":              index,
			"registration_count": len(items),
		})
	}
	for _, item := range items {
		if item.RegistrationKey == key {
			return cloneRegisteredSource(item), nil
		}
	}
	return RegisteredSource{}, errs.IndexingSourceMissing(index, map[string]any{"registration_key": key})
}

func (r *Registry) ListRegistrations(index string) []RegisteredSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.registrations[index]
	if len(items) == 0 {
		return nil
	}
	out := make([]RegisteredSource, len(items))
	for i := range items {
		out[i] = cloneRegisteredSource(items[i])
	}
	return out
}

func (r *Registry) ListIndexes() []types.IndexDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]types.IndexDefinition, 0, len(r.indexes))
	for _, def := range r.indexes {
		out = append(out, def.Clone())
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func cloneRegisteredSource(in RegisteredSource) RegisteredSource {
	out := in
	out.Definition = in.Definition.Clone()
	return out
}

func registrationKeyFor(indexer RecordIndexer) string {
	if keyer, ok := indexer.(registrationKeyer); ok {
		return strings.TrimSpace(keyer.RegistrationKey())
	}
	return strings.TrimSpace(indexer.SourceType())
}
