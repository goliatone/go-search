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
	WeightedQueryFields  bool                   `json:"weighted_query_fields"`
	TextMatchControls    bool                   `json:"text_match_controls"`
	EntityGrouping       bool                   `json:"entity_grouping"`
	ExactEntityCounts    bool                   `json:"exact_entity_counts"`
	EntityFacetCounts    bool                   `json:"entity_facet_counts"`
	CrossIndexFacetUnion bool                   `json:"cross_index_facet_union"`
	BatchedEvidence      bool                   `json:"batched_evidence"`
	SupportedSearchModes []SearchMode           `json:"supported_search_modes"`
	Limitations          []CapabilityLimitation `json:"limitations,omitempty"`
	Metadata             map[string]any         `json:"metadata"`
}

type CapabilityLimitation struct {
	Capability string         `json:"capability"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
