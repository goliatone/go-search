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
	ReplaceDocuments(ctx context.Context, index, registrationKey string, sourceIDs []string, docs []types.Document) error
	DeleteDocuments(ctx context.Context, index string, ids []string) error
	DeleteBySource(ctx context.Context, index, registrationKey string, sourceIDs []string) error
	Health(ctx context.Context, req types.HealthRequest) (types.HealthStatus, error)
}

type SearchBatcher interface {
	SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error)
}

// DefinitionHealthProvider inspects canonical index definitions without
// registering, creating, or otherwise mutating provider state. It is useful for
// operational health checks that run in a process which did not call
// EnsureIndex, such as a standalone CLI invocation.
type DefinitionHealthProvider interface {
	HealthDefinitions(ctx context.Context, definitions []types.IndexDefinition) (types.HealthStatus, error)
}

type EvidenceAggregator interface {
	AggregateEvidence(context.Context, types.EvidenceRequest) (map[string]*types.MatchEvidenceSummary, error)
}

type RegistrationResetter interface {
	ResetRegistration(ctx context.Context, index, registrationKey string) error
}

// RecordReplacementProvider preserves the enumerated source record identity
// through provider decorators. It is optional for normal indexing, but enables
// generation-repair code to prove that every listed record was projected even
// when multiple records share the same canonical source identity.
type RecordReplacementProvider interface {
	ReplaceRecordDocuments(ctx context.Context, index, registrationKey, recordID string, sourceIDs []string, docs []types.Document) error
}
