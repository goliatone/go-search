package media

import (
	"context"
	"testing"
	"time"

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

func TestTranscriptProjectorCanonicalizesLocaleWrites(t *testing.T) {
	record := TranscriptRecord{
		ID: "track-1",
		Media: MediaRecord{
			ID:    "video-1",
			Title: "Ocean Wind",
			URL:   "https://example.org/videos/ocean-wind",
			Topic: "archive",
		},
		Track: types.TranscriptTrack{
			MediaID:      "video-1",
			Locale:       " EN_us ",
			SourceFormat: "srt",
			TrackKind:    "translation",
		},
		Format: "srt",
		Content: `1
00:00:01,000 --> 00:00:02,500
ocean wind
`,
	}
	projector := NewTranscriptProjector(TranscriptProjectorConfig{
		Index:        "media",
		SourceType:   "transcript",
		MergeVersion: "v1",
	})
	docs, err := projector.Project(context.Background(), record)
	if err != nil {
		t.Fatalf("project transcript: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected one document, got %d", len(docs))
	}
	if docs[0].Locale != "en-US" {
		t.Fatalf("document locale = %q", docs[0].Locale)
	}
	if got := docs[0].Facets["locale"]; len(got) != 1 || got[0] != "en-US" {
		t.Fatalf("locale facet = %#v", got)
	}
	if got := docs[0].Metadata["track_locale"]; got != "en-US" {
		t.Fatalf("track locale metadata = %#v", got)
	}
}

func TestTranscriptProjectorEmitsArchiveFacetMetadata(t *testing.T) {
	publishedAt := time.Date(2024, time.March, 20, 0, 0, 0, 0, time.UTC)
	record := TranscriptRecord{
		ID: "track-archive-1",
		Media: MediaRecord{
			ID:              "video-archive-1",
			Title:           "Tara Teachings",
			Summary:         "Archive entry for Tara teachings.",
			URL:             "https://example.org/videos/tara",
			Thumbnail:       "https://example.org/thumbs/tara.jpg",
			Topic:           "Tara",
			TopicPath:       []string{"Teaching Topics", "Tara"},
			CategoryPath:    []string{"Teaching Categories", "Commentary"},
			People:          []string{"Jane Teacher"},
			Subjects:        []string{"Compassion"},
			Texts:           []string{"Praise to Tara"},
			Deities:         []string{"Tara"},
			Location:        "Boulder",
			Sangha:          "Green Tara Sangha",
			Format:          "Teaching",
			Series:          "Tara Cycle",
			DurationSeconds: 2700,
			PublishedAt:     &publishedAt,
			Badge:           "Featured",
		},
		Track: types.TranscriptTrack{
			MediaID:      "video-archive-1",
			Locale:       "en",
			SourceFormat: "srt",
			TrackKind:    "translation",
		},
		Format: "srt",
		Content: `1
00:00:01,000 --> 00:00:02,500
praise to tara
`,
	}
	projector := NewTranscriptProjector(TranscriptProjectorConfig{
		Index:      "media",
		SourceType: "transcript",
	})

	docs, err := projector.Project(context.Background(), record)
	if err != nil {
		t.Fatalf("project transcript: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected one document, got %d", len(docs))
	}
	doc := docs[0]
	if got := doc.Facets[FacetFieldTopicHierarchy]; len(got) != 2 || got[1] != "Teaching Topics > Tara" {
		t.Fatalf("topic hierarchy = %#v", got)
	}
	if got := doc.Facets[FacetFieldCategoryHierarchy]; len(got) != 2 || got[1] != "Teaching Categories > Commentary" {
		t.Fatalf("category hierarchy = %#v", got)
	}
	if got := doc.Facets[FacetFieldDecade]; len(got) != 1 || got[0] != "2020s" {
		t.Fatalf("decade facet = %#v", got)
	}
	if got := doc.Facets[FacetFieldDurationBucket]; len(got) != 1 || got[0] != "30-60 min" {
		t.Fatalf("duration facet = %#v", got)
	}
	if got := doc.Numeric[FieldDurationSeconds]; got != 2700 {
		t.Fatalf("duration numeric = %v", got)
	}
	if got := doc.Fields[FieldResultBadge]; got != "Featured" {
		t.Fatalf("result badge = %#v", got)
	}
}
