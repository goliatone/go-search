package subtitle

import (
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

func TestParseAndMergeSRT(t *testing.T) {
	input := `1
00:00:01,000 --> 00:00:02,000
ocean wind

2
00:00:02,200 --> 00:00:03,000
chanting prayer
`
	cues, err := ParseSRT(input)
	if err != nil {
		t.Fatalf("parse srt: %v", err)
	}
	merged := MergeCues(cues, MergeConfig{MaxCharacters: 80, MaxGapMS: 500, Version: "v1"})
	if len(merged) != 1 {
		t.Fatalf("expected one merged cue, got %d", len(merged))
	}
	if got := SegmentDocumentID("transcript", "track-1", "en", "v1", merged[0]); got == "" {
		t.Fatalf("expected deterministic segment id")
	}
}

func TestParseVTT(t *testing.T) {
	input := `WEBVTT

00:00:01.000 --> 00:00:02.000
ocean wind
`
	cues, err := ParseVTT(input)
	if err != nil {
		t.Fatalf("parse vtt: %v", err)
	}
	if len(cues) != 1 {
		t.Fatalf("expected one cue, got %d", len(cues))
	}
}

func TestSegmentDocumentIDCanonicalizesLocale(t *testing.T) {
	cue := Cue{Start: 1000, End: 2000, Text: "ocean wind"}
	left := SegmentDocumentID("transcript", "track-1", "EN_us", "v1", cue)
	right := SegmentDocumentID("transcript", "track-1", "en-US", "v1", cue)
	if left != right {
		t.Fatalf("expected canonical locale ids to match, got %q != %q", left, right)
	}
}

func TestBuildSegmentDocumentsCanonicalizesLocaleFields(t *testing.T) {
	docs := BuildSegmentDocuments([]Cue{{Start: 1000, End: 2000, Text: "ocean wind"}}, DocumentOptions{
		Index:      "media",
		SourceType: "transcript",
		SourceID:   "track-1",
		ParentID:   "video-1",
		Locale:     " EN_us ",
		Version:    "v1",
		BaseURL:    "https://example.org/video-1",
		ParentURL:  "https://example.org/video-1",
		Track: types.TranscriptTrack{
			Locale:       "EN_us",
			SourceFormat: "srt",
			TrackKind:    "translation",
		},
	})
	if len(docs) != 1 {
		t.Fatalf("expected one document, got %d", len(docs))
	}
	doc := docs[0]
	if doc.Locale != "en-US" {
		t.Fatalf("document locale = %q", doc.Locale)
	}
	if got := doc.Metadata["track_locale"]; got != "en-US" {
		t.Fatalf("track locale metadata = %#v", got)
	}
}

func TestBuildSegmentDocumentsCopiesIdentityOrdinalAndVisibility(t *testing.T) {
	docs := BuildSegmentDocuments([]Cue{{Start: 1000, End: 2000, Text: "one"}, {Start: 3000, End: 4000, Text: "two"}}, DocumentOptions{
		SourceType: "transcript", SourceID: "track-1", Version: "v2", Locale: "en",
		ParentID: "video-1", ParentType: "teaching", ResultID: "event-1", ResultType: "event",
		MatchLocation: "transcript", MatchField: "body", ParentSummary: "Public summary",
		Visibility: types.Visibility{Public: true, Status: "published"},
	})
	if len(docs) != 2 || docs[0].ChunkOrdinal == nil || *docs[0].ChunkOrdinal != 0 || docs[1].ChunkOrdinal == nil || *docs[1].ChunkOrdinal != 1 {
		t.Fatalf("ordinals: %#v", docs)
	}
	for _, doc := range docs {
		if doc.ResultID != "event-1" || doc.ResultType != "event" || doc.MatchLocation != "transcript" || doc.MatchField != "body" {
			t.Fatalf("identity contract missing: %#v", doc)
		}
		if !doc.Visibility.Public || doc.Summary != "Public summary" || doc.Summary == doc.Body {
			t.Fatalf("visibility or entity summary contract missing: %#v", doc)
		}
	}
}

func TestBuildSegmentDocumentsDefaultsToNonPublic(t *testing.T) {
	docs := BuildSegmentDocuments([]Cue{{Text: "cue"}}, DocumentOptions{SourceType: "transcript", SourceID: "track", Version: "v1"})
	if len(docs) != 1 || docs[0].Visibility.Public {
		t.Fatalf("ambiguous visibility became public: %#v", docs)
	}
}
