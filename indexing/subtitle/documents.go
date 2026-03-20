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
	Locale          string
	Version         string
	BaseURL         string
	ParentTitle     string
	ParentSummary   string
	ParentURL       string
	ParentThumbnail string
	ParentFields    map[string]any
	ParentFacets    map[string][]string
	Track           types.TranscriptTrack
}

func BuildSegmentDocuments(cues []Cue, opts DocumentOptions) []types.Document {
	canonicalLocale := locale.Normalize(opts.Locale)
	trackLocale := locale.Normalize(opts.Track.Locale)
	out := make([]types.Document, 0, len(cues))
	for _, cue := range cues {
		start := cue.Start
		end := cue.End
		id := SegmentDocumentID(opts.SourceType, opts.SourceID, canonicalLocale, opts.Version, cue)
		doc := types.Document{
			ID:         id,
			Index:      opts.Index,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   opts.ParentID,
			SourceType: opts.SourceType,
			SourceID:   opts.SourceID,
			Title:      opts.ParentTitle,
			Summary:    opts.ParentSummary,
			Body:       strings.TrimSpace(cue.Text),
			URL:        opts.ParentURL,
			AnchorURL:  anchorURL(opts.BaseURL, cue.Start),
			Locale:     canonicalLocale,
			StartMS:    &start,
			EndMS:      &end,
			Fields: map[string]any{
				"track_kind":    opts.Track.TrackKind,
				"source_format": opts.Track.SourceFormat,
				"parent_title":  opts.ParentTitle,
				"parent_url":    opts.ParentURL,
			},
			Facets:  cloneFacets(opts.ParentFacets),
			Numeric: map[string]float64{"start_ms": float64(cue.Start), "end_ms": float64(cue.End)},
			Metadata: map[string]any{
				"track_locale": trackLocale,
			},
		}
		if opts.ParentThumbnail != "" {
			doc.Fields["parent_thumbnail"] = opts.ParentThumbnail
		}
		maps.Copy(doc.Fields, opts.ParentFields)
		doc.Metadata = clone(opts.ParentFields)
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		doc.Metadata["track_locale"] = trackLocale
		out = append(out, doc)
	}
	return out
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
