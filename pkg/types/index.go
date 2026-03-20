package types

type IndexDefinition struct {
	Name               string
	Label              string
	DefaultQueryFields []string
	SearchableFields   []string
	FacetFields        []string
	SortableFields     []string
	FilterableFields   []string
	HighlightFields    []string
	DefaultSort        []Sort
	GroupByDefault     string
	ProviderHints      map[string]any
	Semantic           *SemanticIndexConfig
}

type SemanticIndexConfig struct {
	Enabled        bool
	DefaultMode    SearchMode
	Embedder       string
	QueryByDefault bool
	Fields         []EmbeddingField
}

type EmbeddingField struct {
	Name           string
	SourceFields   []string
	Chunking       *ChunkingConfig
	LocaleStrategy string
	Model          string
	Dimensions     int
	DistanceMetric string
	Queryable      bool
	Stored         bool
	ProviderHints  map[string]any
}

type ChunkingConfig struct {
	MaxCharacters int
	Overlap       int
}

type EnsureIndexInput struct {
	Definition IndexDefinition
}

func (EnsureIndexInput) Type() string { return "search::ensure_index" }

type UpsertDocumentsInput struct {
	Index     string
	Documents []Document
}

func (UpsertDocumentsInput) Type() string { return "search::upsert_documents" }

type DeleteDocumentsInput struct {
	Index string
	IDs   []string
}

func (DeleteDocumentsInput) Type() string { return "search::delete_documents" }

type IndexRecordInput struct {
	Index    string
	RecordID string
}

func (IndexRecordInput) Type() string { return "search::index_record" }

type DeleteRecordInput struct {
	Index    string
	RecordID string
}

func (DeleteRecordInput) Type() string { return "search::delete_record" }

type ReindexIndexInput struct {
	Index     string
	BatchSize int
}

func (ReindexIndexInput) Type() string { return "search::reindex_index" }
