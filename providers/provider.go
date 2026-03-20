package providers

import (
	"context"

	"github.com/goliatone/go-search/pkg/types"
)

type Provider interface {
	Name() string
	Capabilities(ctx context.Context) (types.CapabilitySet, error)
	EnsureIndex(ctx context.Context, def types.IndexDefinition) error
	Search(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error)
	Suggest(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error)
	UpsertDocuments(ctx context.Context, index string, docs []types.Document) error
	DeleteDocuments(ctx context.Context, index string, ids []string) error
	DeleteBySource(ctx context.Context, index string, sourceIDs []string) error
	Health(ctx context.Context) (types.HealthStatus, error)
}
