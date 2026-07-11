package types

type IndexDefinition struct {
	DefaultWeightedQueryFields []QueryField         `json:"default_weighted_query_fields,omitempty"`
	Name                       string               `json:"name"`
	Label                      string               `json:"label"`
	DefaultQueryFields         []string             `json:"default_query_fields"`
	SearchableFields           []string             `json:"searchable_fields"`
	FacetFields                []string             `json:"facet_fields"`
	SortableFields             []string             `json:"sortable_fields"`
	FilterableFields           []string             `json:"filterable_fields"`
	HighlightFields            []string             `json:"highlight_fields"`
	DefaultSort                []Sort               `json:"default_sort"`
	GroupByDefault             string               `json:"group_by_default"`
	ProviderHints              map[string]any       `json:"provider_hints"`
	Semantic                   *SemanticIndexConfig `json:"semantic"`
}

type QueryField struct {
	Field  string `json:"field"`
	Weight int    `json:"weight"`
}

type SemanticIndexConfig struct {
	Enabled        bool             `json:"enabled"`
	DefaultMode    SearchMode       `json:"default_mode"`
	Embedder       string           `json:"embedder"`
	QueryByDefault bool             `json:"query_by_default"`
	Fields         []EmbeddingField `json:"fields"`
}

type EmbeddingField struct {
	Name           string          `json:"name"`
	SourceFields   []string        `json:"source_fields"`
	Chunking       *ChunkingConfig `json:"chunking"`
	LocaleStrategy string          `json:"locale_strategy"`
	Model          string          `json:"model"`
	Dimensions     int             `json:"dimensions"`
	DistanceMetric string          `json:"distance_metric"`
	Queryable      bool            `json:"queryable"`
	Stored         bool            `json:"stored"`
	ProviderHints  map[string]any  `json:"provider_hints"`
}

type ChunkingConfig struct {
	MaxCharacters int `json:"max_characters"`
	Overlap       int `json:"overlap"`
}

type EnsureIndexInput struct {
	Definition IndexDefinition `json:"definition"`
}

func (EnsureIndexInput) Type() string { return "search::ensure_index" }

type UpsertDocumentsInput struct {
	Index     string     `json:"index"`
	Documents []Document `json:"documents"`
}

func (UpsertDocumentsInput) Type() string { return "search::upsert_documents" }

type DeleteDocumentsInput struct {
	Index string   `json:"index"`
	IDs   []string `json:"ids"`
}

func (DeleteDocumentsInput) Type() string { return "search::delete_documents" }

type IndexRecordInput struct {
	Index           string `json:"index"`
	RegistrationKey string `json:"registration_key,omitempty"`
	RecordID        string `json:"record_id"`
}

func (IndexRecordInput) Type() string { return "search::index_record" }

type DeleteRecordInput struct {
	Index           string `json:"index"`
	RegistrationKey string `json:"registration_key,omitempty"`
	RecordID        string `json:"record_id"`
}

func (DeleteRecordInput) Type() string { return "search::delete_record" }

type ReindexIndexInput struct {
	Index           string `json:"index"`
	RegistrationKey string `json:"registration_key,omitempty"`
	BatchSize       int    `json:"batch_size"`
}

func (ReindexIndexInput) Type() string { return "search::reindex_index" }
