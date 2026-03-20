package types

import "maps"

import "time"

const (
	DocumentTypeVideo             = "video"
	DocumentTypeTranscriptSegment = "transcript_segment"
)

type Document struct {
	ID            string
	Index         string
	Type          string
	ParentID      string
	SourceType    string
	SourceID      string
	SourceVersion string
	Title         string
	Summary       string
	Body          string
	URL           string
	AnchorURL     string
	Locale        string
	Score         float64
	CreatedAt     *time.Time
	UpdatedAt     *time.Time
	PublishedAt   *time.Time
	StartMS       *int64
	EndMS         *int64
	Fields        map[string]any
	Facets        map[string][]string
	Numeric       map[string]float64
	Booleans      map[string]bool
	Scope         Scope
	Visibility    Visibility
	Metadata      map[string]any
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
	ParentID   string
	ParentType string
	StartMS    int64
	EndMS      int64
	Label      string
	URL        string
}

type TranscriptTrack struct {
	MediaID      string
	Locale       string
	SourceFormat string
	TrackKind    string
	SourceLocale string
	IsMachine    bool
	Metadata     map[string]any
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
