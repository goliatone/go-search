package types

type CapabilitySet struct {
	Facets               bool
	DisjunctiveFacets    bool
	PrefixSearch         bool
	TypoTolerance        bool
	Highlighting         bool
	Snippets             bool
	Grouping             bool
	SemanticSearch       bool
	HybridSearch         bool
	AutoEmbedding        bool
	ExternalEmbeddings   bool
	DistanceThreshold    bool
	MultilingualEmbeds   bool
	SupportedSearchModes []SearchMode
	Metadata             map[string]any
}
