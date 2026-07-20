package subtitle

import (
	"fmt"
	"maps"
	"strings"

	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

type DocumentOptions struct {
	Index           string
	SourceType      string
	SourceID        string
	ParentID        string
	ParentType      string
	ResultID        string
	ResultType      string
	MatchLocation   string
	MatchField      string
	Locale          string
	Version         string
	BaseURL         string
	ParentTitle     string
	ParentSummary   string
	ParentURL       string
	ParentThumbnail string
	ParentFields    map[string]any
	ParentFacets    map[string][]string
	ParentNumeric   map[string]float64
	ParentMetadata  map[string]any
	Metadata        map[string]any
	Visibility      types.Visibility
	AnchorURL       func(startMS int64) string
	Track           types.TranscriptTrack
}

func BuildSegmentDocuments(cues []Cue, opts DocumentOptions) []types.Document {
	canonicalLocale := locale.Normalize(opts.Locale)
	trackLocale := locale.Normalize(opts.Track.Locale)
	out := make([]types.Document, 0, len(cues))
	for ordinal, cue := range cues {
		start := cue.Start
		end := cue.End
		id := SegmentDocumentID(opts.SourceType, opts.SourceID, canonicalLocale, opts.Version, cue)
		doc := types.Document{
			ID:            id,
			Index:         opts.Index,
			Type:          types.DocumentTypeTranscriptSegment,
			ParentID:      opts.ParentID,
			ResultID:      opts.ResultID,
			ResultType:    opts.ResultType,
			MatchLocation: opts.MatchLocation,
			MatchField:    opts.MatchField,
			ChunkOrdinal:  intPtr(ordinal),
			SourceType:    opts.SourceType,
			SourceID:      opts.SourceID,
			SourceVersion: opts.Version,
			Title:         opts.ParentTitle,
			Summary:       opts.ParentSummary,
			Body:          strings.TrimSpace(cue.Text),
			URL:           opts.ParentURL,
			AnchorURL:     resolveAnchorURL(opts, cue.Start),
			Locale:        canonicalLocale,
			StartMS:       &start,
			EndMS:         &end,
			Fields: map[string]any{
				"track_kind":    opts.Track.TrackKind,
				"source_format": opts.Track.SourceFormat,
				"parent_title":  opts.ParentTitle,
				"parent_url":    opts.ParentURL,
				"parent_type":   firstNonEmpty(opts.ParentType, types.DocumentTypeVideo),
			},
			Facets:     cloneFacets(opts.ParentFacets),
			Numeric:    map[string]float64{"start_ms": float64(cue.Start), "end_ms": float64(cue.End)},
			Visibility: opts.Visibility.Clone(),
		}
		if opts.ParentThumbnail != "" {
			doc.Fields["parent_thumbnail"] = opts.ParentThumbnail
		}
		maps.Copy(doc.Fields, opts.ParentFields)
		maps.Copy(doc.Numeric, opts.ParentNumeric)
		doc.Metadata = clone(opts.ParentMetadata)
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		maps.Copy(doc.Metadata, opts.Metadata)
		doc.Metadata["track_locale"] = trackLocale
		out = append(out, doc)
	}
	return out
}

func resolveAnchorURL(opts DocumentOptions, startMS int64) string {
	if opts.AnchorURL != nil {
		return strings.TrimSpace(opts.AnchorURL(startMS))
	}
	return anchorURL(opts.BaseURL, startMS)
}

func intPtr(value int) *int { return &value }

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func clone(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func cloneFacets(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func anchorURL(baseURL string, startMS int64) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s#t=%d", baseURL, startMS/1000)
}
