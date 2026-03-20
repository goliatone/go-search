package indexing

import (
	"context"
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

type Registry struct {
	mu      sync.RWMutex
	indexes map[string]types.IndexDefinition
	records map[string]RecordIndexer
}

func NewRegistry() *Registry {
	return &Registry{
		indexes: map[string]types.IndexDefinition{},
		records: map[string]RecordIndexer{},
	}
}

func (r *Registry) Register(def types.IndexDefinition, indexer RecordIndexer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indexes[def.Name] = def
	if indexer != nil {
		r.records[def.Name] = indexer
	}
	return nil
}

func (r *Registry) GetIndex(name string) (types.IndexDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.indexes[name]
	return def, ok
}

func (r *Registry) MustIndexer(name string) (RecordIndexer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	indexer, ok := r.records[name]
	if !ok {
		return nil, errs.IndexingSourceMissing(name, nil)
	}
	return indexer, nil
}

func (r *Registry) ListIndexes() []types.IndexDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]types.IndexDefinition, 0, len(r.indexes))
	for _, def := range r.indexes {
		out = append(out, def)
	}
	return out
}
