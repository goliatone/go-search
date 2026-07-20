package types

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type SearchResultPage struct {
	Hits          []SearchHit            `json:"hits"`
	Groups        []SearchGroup          `json:"groups"`
	Facets        []SearchFacet          `json:"facets"`
	Page          int                    `json:"page"`
	PerPage       int                    `json:"per_page"`
	Total         int                    `json:"total"`
	TotalAccuracy TotalAccuracy          `json:"total_accuracy,omitempty"`
	Counts        map[string]SearchCount `json:"counts,omitempty"`
	DurationMS    int64                  `json:"duration_ms"`
	Metadata      map[string]any         `json:"metadata"`
}

type TotalAccuracy string

const (
	TotalAccuracyExact       TotalAccuracy = "exact"
	TotalAccuracyLowerBound  TotalAccuracy = "lower_bound"
	TotalAccuracyApproximate TotalAccuracy = "approximate"
)

// CountAccuracy describes the confidence of an application-defined named count.
type CountAccuracy string

const (
	CountAccuracyExact       CountAccuracy = "exact"
	CountAccuracyLowerBound  CountAccuracy = "lower_bound"
	CountAccuracyApproximate CountAccuracy = "approximate"
	CountAccuracyUnavailable CountAccuracy = "unavailable"
)

// SearchCount is an optional application-defined count alongside the primary total.
// Consumers must ignore Value when Accuracy is unavailable.
type SearchCount struct {
	Value      int           `json:"value"`
	Accuracy   CountAccuracy `json:"accuracy"`
	Diagnostic string        `json:"diagnostic,omitempty"`
}

func (c SearchCount) Validate() error {
	switch c.Accuracy {
	case CountAccuracyExact, CountAccuracyLowerBound, CountAccuracyApproximate:
		if c.Value < 0 {
			return fmt.Errorf("available search count cannot be negative")
		}
		return nil
	case CountAccuracyUnavailable:
		if strings.TrimSpace(c.Diagnostic) == "" {
			return fmt.Errorf("unavailable search count requires a diagnostic")
		}
		return nil
	default:
		return fmt.Errorf("unsupported search count accuracy %q", c.Accuracy)
	}
}

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
	Status     EvidenceStatus          `json:"status,omitempty"`
	Locations  []MatchEvidenceLocation `json:"locations"`
	Diagnostic string                  `json:"diagnostic,omitempty"`
}

type EvidenceStatus string

const (
	EvidenceStatusComplete    EvidenceStatus = "complete"
	EvidenceStatusPartial     EvidenceStatus = "partial"
	EvidenceStatusUnsupported EvidenceStatus = "unsupported"
	EvidenceStatusUnavailable EvidenceStatus = "unavailable"
)

func (s MatchEvidenceSummary) Validate() error {
	status := s.Status
	if status == "" && s.Exact {
		status = EvidenceStatusComplete
	}
	switch status {
	case EvidenceStatusComplete:
		if !s.Exact {
			return fmt.Errorf("complete evidence must be exact")
		}
		return nil
	case EvidenceStatusPartial, EvidenceStatusUnsupported, EvidenceStatusUnavailable:
		if s.Exact {
			return fmt.Errorf("%s evidence cannot be exact", status)
		}
		if strings.TrimSpace(s.Diagnostic) == "" {
			return fmt.Errorf("%s evidence requires a diagnostic", status)
		}
		return nil
	default:
		return fmt.Errorf("unsupported evidence status %q", s.Status)
	}
}

type MatchEvidenceLocation struct {
	Location string                `json:"location"`
	Count    int                   `json:"count"`
	Samples  []MatchEvidenceSample `json:"samples,omitempty"`
}
type MatchEvidenceSample struct {
	DocumentID   string         `json:"document_id"`
	Field        string         `json:"field,omitempty"`
	Locale       string         `json:"locale,omitempty"`
	Snippet      *SearchSnippet `json:"snippet,omitempty"`
	ChunkOrdinal *int           `json:"chunk_ordinal,omitempty"`
	Anchor       *MediaAnchor   `json:"anchor,omitempty"`
}
type EvidenceRequest struct {
	Search                SearchRequest `json:"search"`
	ResultIDs             []string      `json:"result_ids"`
	MaxSamplesPerLocation int           `json:"max_samples_per_location"`
	Guard                 ScopeGuard    `json:"-"`
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

const MaxEvidenceSnippetBytes = 1024

// BoundedSearchSnippet defensively copies a snippet and truncates each string at
// a valid UTF-8 rune boundary.
func BoundedSearchSnippet(in *SearchSnippet) *SearchSnippet {
	if in == nil {
		return nil
	}
	return &SearchSnippet{
		Text:        truncateUTF8(in.Text, MaxEvidenceSnippetBytes),
		Highlighted: truncateUTF8(in.Highlighted, MaxEvidenceSnippetBytes),
	}
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
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
