package media

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

func TestTranscriptProjectorDenormalizesParentMetadataIntoSegments(t *testing.T) {
	record := TranscriptRecord{
		ID: "track-1",
		Media: MediaRecord{
			ID:        "video-1",
			Title:     "Ocean Wind",
			Summary:   "Archive video about coastal chants",
			URL:       "https://example.org/videos/ocean-wind",
			Thumbnail: "https://example.org/thumbs/ocean-wind.jpg",
			Topic:     "archive",
			Locale:    "en",
		},
		Track: types.TranscriptTrack{
			MediaID:      "video-1",
			Locale:       "en",
			SourceFormat: "srt",
			TrackKind:    "translation",
			SourceLocale: "bo",
		},
		Format: "srt",
		Content: `1
00:00:01,000 --> 00:00:02,500
ocean wind

2
00:00:03,000 --> 00:00:04,500
chanting prayer
`,
	}
	projector := NewTranscriptProjector(TranscriptProjectorConfig{
		Index:        "media",
		SourceType:   "transcript",
		MergeVersion: "v1",
		MaxChars:     80,
		MaxGapMS:     600,
	})
	docs, err := projector.Project(context.Background(), record)
	if err != nil {
		t.Fatalf("project transcript: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected merged segment-only projection, got %d docs", len(docs))
	}
	doc := docs[0]
	if doc.Type != types.DocumentTypeTranscriptSegment {
		t.Fatalf("expected transcript segment, got %s", doc.Type)
	}
	if got := doc.Facets["topic"]; len(got) != 1 || got[0] != "archive" {
		t.Fatalf("expected topic facet to be preserved, got %+v", doc.Facets)
	}
	if got := doc.Fields["parent_thumbnail"]; got != record.Media.Thumbnail {
		t.Fatalf("expected parent thumbnail field, got %+v", doc.Fields)
	}
	if doc.AnchorURL != "https://example.org/videos/ocean-wind#t=1" {
		t.Fatalf("expected anchored playback url, got %s", doc.AnchorURL)
	}
}
