package types

import "time"

type SearchResultPage struct {
	Hits       []SearchHit
	Groups     []SearchGroup
	Facets     []SearchFacet
	Page       int
	PerPage    int
	Total      int
	DurationMS int64
	Metadata   map[string]any
}

type SearchHit struct {
	ID         string
	Type       string
	Title      string
	Summary    string
	URL        string
	Locale     string
	Score      float64
	BaseScore  float64
	FinalScore float64
	Anchor     *MediaAnchor
	Parent     *SearchParent
	Snippet    *SearchSnippet
	Fields     map[string]any
	Ranking    *AppliedRankingSignals
	Retrieval  *AppliedRetrievalSignals
	Document   *Document
}

type SearchParent struct {
	ID        string
	Type      string
	Title     string
	Thumbnail string
	URL       string
}

type SearchSnippet struct {
	Text        string
	Highlighted string
}

type SearchGroup struct {
	Key      string
	Parent   *SearchParent
	TopHit   *SearchHit
	Hits     []SearchHit
	Count    int
	Metadata map[string]any
}

type SearchFacet struct {
	Field  string
	Values []SearchFacetValue
}

type SearchFacetValue struct {
	Value string
	Count int
}

type SuggestResult struct {
	Items      []SuggestHit
	DurationMS int64
	Metadata   map[string]any
}

type SuggestHit struct {
	ID       string
	Type     string
	Title    string
	URL      string
	Locale   string
	Score    float64
	Parent   *SearchParent
	Document *Document
}

type HealthStatus struct {
	Provider  string
	Healthy   bool
	CheckedAt time.Time
	Message   string
	Indexes   []IndexHealth
	Metadata  map[string]any
}

type IndexHealth struct {
	Name      string
	Ready     bool
	Documents int
	Message   string
	Metadata  map[string]any
}

type StatsResult struct {
	Provider     string
	Capabilities CapabilitySet
	Indexes      []IndexStats
	Metadata     map[string]any
}

type IndexStats struct {
	Name           string
	Documents      int
	Generation     int64
	LastIndexedAt  *time.Time
	Registered     bool
	ProviderStatus string
	Metadata       map[string]any
}
