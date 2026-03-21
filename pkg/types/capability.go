package types

type CapabilitySet struct {
	Facets               bool                   `json:"facets"`
	HierarchicalFacets   bool                   `json:"hierarchical_facets"`
	RangeFacets          bool                   `json:"range_facets"`
	DisjunctiveFacets    bool                   `json:"disjunctive_facets"`
	PrefixSearch         bool                   `json:"prefix_search"`
	TypoTolerance        bool                   `json:"typo_tolerance"`
	Highlighting         bool                   `json:"highlighting"`
	Snippets             bool                   `json:"snippets"`
	Grouping             bool                   `json:"grouping"`
	SemanticSearch       bool                   `json:"semantic_search"`
	HybridSearch         bool                   `json:"hybrid_search"`
	AutoEmbedding        bool                   `json:"auto_embedding"`
	ExternalEmbeddings   bool                   `json:"external_embeddings"`
	DistanceThreshold    bool                   `json:"distance_threshold"`
	MultilingualEmbeds   bool                   `json:"multilingual_embeds"`
	SupportedSearchModes []SearchMode           `json:"supported_search_modes"`
	Limitations          []CapabilityLimitation `json:"limitations,omitempty"`
	Metadata             map[string]any         `json:"metadata"`
}

type CapabilityLimitation struct {
	Capability string         `json:"capability"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
