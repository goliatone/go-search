package subtitle

import (
	"maps"
	"strings"

	"github.com/goliatone/go-search/pkg/types"
)

type DocumentOptions struct {
	Index        string
	SourceType   string
	SourceID     string
	ParentID     string
	Locale       string
	Version      string
	BaseURL      string
	ParentTitle  string
	ParentURL    string
	ParentFields map[string]any
	Track        types.TranscriptTrack
}

func BuildSegmentDocuments(cues []Cue, opts DocumentOptions) []types.Document {
	out := make([]types.Document, 0, len(cues))
	for _, cue := range cues {
		start := cue.Start
		end := cue.End
		id := SegmentDocumentID(opts.SourceType, opts.SourceID, opts.Locale, opts.Version, cue)
		doc := types.Document{
			ID:         id,
			Index:      opts.Index,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   opts.ParentID,
			SourceType: opts.SourceType,
			SourceID:   opts.SourceID,
			Title:      opts.ParentTitle,
			Body:       strings.TrimSpace(cue.Text),
			URL:        opts.ParentURL,
			AnchorURL:  opts.BaseURL,
			Locale:     opts.Locale,
			StartMS:    &start,
			EndMS:      &end,
			Fields: map[string]any{
				"track_kind":    opts.Track.TrackKind,
				"source_format": opts.Track.SourceFormat,
			},
			Metadata: clone(opts.ParentFields),
		}
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
