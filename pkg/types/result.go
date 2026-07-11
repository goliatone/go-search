package types

import "time"

type SearchResultPage struct {
	Hits          []SearchHit    `json:"hits"`
	Groups        []SearchGroup  `json:"groups"`
	Facets        []SearchFacet  `json:"facets"`
	Page          int            `json:"page"`
	PerPage       int            `json:"per_page"`
	Total         int            `json:"total"`
	TotalAccuracy TotalAccuracy  `json:"total_accuracy,omitempty"`
	DurationMS    int64          `json:"duration_ms"`
	Metadata      map[string]any `json:"metadata"`
}

type TotalAccuracy string

const (
	TotalAccuracyExact       TotalAccuracy = "exact"
	TotalAccuracyLowerBound  TotalAccuracy = "lower_bound"
	TotalAccuracyApproximate TotalAccuracy = "approximate"
)

type SearchHit struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Title      string                   `json:"title"`
	Summary    string                   `json:"summary"`
	URL        string                   `json:"url"`
	Locale     string                   `json:"locale"`
	Score      float64                  `json:"score"`
	BaseScore  float64                  `json:"base_score"`
	FinalScore float64                  `json:"final_score"`
	Anchor     *MediaAnchor             `json:"anchor"`
	Parent     *SearchParent            `json:"parent"`
	Snippet    *SearchSnippet           `json:"snippet"`
	Fields     map[string]any           `json:"fields"`
	Ranking    *AppliedRankingSignals   `json:"ranking"`
	Retrieval  *AppliedRetrievalSignals `json:"retrieval"`
	Document   *Document                `json:"document"`
	ResultID   string                   `json:"result_id,omitempty"`
	ResultType string                   `json:"result_type,omitempty"`
	Evidence   *MatchEvidenceSummary    `json:"match_evidence,omitempty"`
}

type MatchEvidenceSummary struct {
	Exact      bool                    `json:"exact"`
	Locations  []MatchEvidenceLocation `json:"locations"`
	Diagnostic string                  `json:"diagnostic,omitempty"`
}
type MatchEvidenceLocation struct {
	Location string                `json:"location"`
	Count    int                   `json:"count"`
	Samples  []MatchEvidenceSample `json:"samples,omitempty"`
}
type MatchEvidenceSample struct {
	DocumentID   string       `json:"document_id"`
	Field        string       `json:"field,omitempty"`
	ChunkOrdinal *int         `json:"chunk_ordinal,omitempty"`
	Anchor       *MediaAnchor `json:"anchor,omitempty"`
}
type EvidenceRequest struct {
	Search                SearchRequest `json:"search"`
	ResultIDs             []string      `json:"result_ids"`
	MaxSamplesPerLocation int           `json:"max_samples_per_location"`
}

type SearchParent struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
	URL       string `json:"url"`
}

type SearchSnippet struct {
	Text        string `json:"text"`
	Highlighted string `json:"highlighted"`
}

type SearchGroup struct {
	Key      string         `json:"key"`
	Parent   *SearchParent  `json:"parent"`
	TopHit   *SearchHit     `json:"top_hit"`
	Hits     []SearchHit    `json:"hits"`
	Count    int            `json:"count"`
	Metadata map[string]any `json:"metadata"`
}

type SearchFacet struct {
	Field       string             `json:"field"`
	Kind        FacetKind          `json:"kind,omitempty"`
	Disjunctive bool               `json:"disjunctive,omitempty"`
	Values      []SearchFacetValue `json:"values"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
	Accuracy    FacetCountAccuracy `json:"accuracy,omitempty"`
}

type FacetCountAccuracy string

const (
	FacetCountAccuracyExact       FacetCountAccuracy = "exact"
	FacetCountAccuracyApproximate FacetCountAccuracy = "approximate"
	FacetCountAccuracyLowerBound  FacetCountAccuracy = "lower_bound"
	FacetCountAccuracyUnavailable FacetCountAccuracy = "unavailable"
)

type SearchFacetValue struct {
	Value             string         `json:"value"`
	Label             string         `json:"label,omitempty"`
	Count             int            `json:"count"`
	Path              []string       `json:"path,omitempty"`
	Level             int            `json:"level,omitempty"`
	ParentValue       string         `json:"parent_value,omitempty"`
	Selected          bool           `json:"selected,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	EntityIDs         []string       `json:"entity_ids,omitempty"`
	EntityIDsComplete bool           `json:"entity_ids_complete,omitempty"`
}

type SuggestResult struct {
	Items      []SuggestHit   `json:"items"`
	DurationMS int64          `json:"duration_ms"`
	Metadata   map[string]any `json:"metadata"`
}

type SuggestHit struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Title    string        `json:"title"`
	URL      string        `json:"url"`
	Locale   string        `json:"locale"`
	Score    float64       `json:"score"`
	Parent   *SearchParent `json:"parent"`
	Document *Document     `json:"document"`
}

type HealthStatus struct {
	Provider  string         `json:"provider"`
	Healthy   bool           `json:"healthy"`
	CheckedAt time.Time      `json:"checked_at"`
	Message   string         `json:"message"`
	Indexes   []IndexHealth  `json:"indexes"`
	Metadata  map[string]any `json:"metadata"`
}

type IndexHealth struct {
	Name      string         `json:"name"`
	Ready     bool           `json:"ready"`
	Documents int            `json:"documents"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
}

type StatsResult struct {
	Provider     string         `json:"provider"`
	Capabilities CapabilitySet  `json:"capabilities"`
	Indexes      []IndexStats   `json:"indexes"`
	Metadata     map[string]any `json:"metadata"`
}

type IndexStats struct {
	Name           string         `json:"name"`
	Documents      int            `json:"documents"`
	Generation     int64          `json:"generation"`
	LastIndexedAt  *time.Time     `json:"last_indexed_at"`
	Registered     bool           `json:"registered"`
	ProviderStatus string         `json:"provider_status"`
	Metadata       map[string]any `json:"metadata"`
}
