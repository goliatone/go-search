package types

import "maps"

import "time"

const (
	DocumentTypeVideo             = "video"
	DocumentTypeTranscriptSegment = "transcript_segment"
)

type Document struct {
	ID            string              `json:"id"`
	Index         string              `json:"index"`
	Type          string              `json:"type"`
	ParentID      string              `json:"parent_id"`
	SourceType    string              `json:"source_type"`
	SourceID      string              `json:"source_id"`
	SourceVersion string              `json:"source_version"`
	Title         string              `json:"title"`
	Summary       string              `json:"summary"`
	Body          string              `json:"body"`
	URL           string              `json:"url"`
	AnchorURL     string              `json:"anchor_url"`
	Locale        string              `json:"locale"`
	Score         float64             `json:"score"`
	CreatedAt     *time.Time          `json:"created_at"`
	UpdatedAt     *time.Time          `json:"updated_at"`
	PublishedAt   *time.Time          `json:"published_at"`
	StartMS       *int64              `json:"start_ms"`
	EndMS         *int64              `json:"end_ms"`
	Fields        map[string]any      `json:"fields"`
	Facets        map[string][]string `json:"facets"`
	Numeric       map[string]float64  `json:"numeric"`
	Booleans      map[string]bool     `json:"booleans"`
	Scope         Scope               `json:"scope"`
	Visibility    Visibility          `json:"visibility"`
	Metadata      map[string]any      `json:"metadata"`
}

func (d Document) Clone() Document {
	out := d
	out.Fields = cloneMap(d.Fields)
	out.Facets = cloneFacetMap(d.Facets)
	out.Numeric = cloneMap(d.Numeric)
	out.Booleans = cloneMap(d.Booleans)
	out.Metadata = cloneMap(d.Metadata)
	out.Scope = d.Scope.Clone()
	out.Visibility = d.Visibility.Clone()
	return out
}

type MediaAnchor struct {
	ParentID   string `json:"parent_id"`
	ParentType string `json:"parent_type"`
	StartMS    int64  `json:"start_ms"`
	EndMS      int64  `json:"end_ms"`
	Label      string `json:"label"`
	URL        string `json:"url"`
}

type TranscriptTrack struct {
	MediaID      string         `json:"media_id"`
	Locale       string         `json:"locale"`
	SourceFormat string         `json:"source_format"`
	TrackKind    string         `json:"track_kind"`
	SourceLocale string         `json:"source_locale"`
	IsMachine    bool           `json:"is_machine"`
	Metadata     map[string]any `json:"metadata"`
}

func cloneMap[T any](in map[string]T) map[string]T {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]T, len(in))
	maps.Copy(out, in)
	return out
}

func cloneFacetMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, vals := range in {
		out[k] = append([]string(nil), vals...)
	}
	return out
}
