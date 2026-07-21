package subtitle

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestBuildUnitDocumentsProjectsTimedUntimedAndMixedUnits(t *testing.T) {
	start0, end0 := int64(0), int64(1000)
	start1, end1 := int64(1200), int64(2000)
	start2, end2 := int64(3000), int64(4000)
	units := []NormalizedUnit{
		{ID: "cue-1", Order: 0, Text: "first timed", StartMS: &start0, EndMS: &end0},
		{ID: "cue-2", Order: 1, Text: "second timed", StartMS: &start1, EndMS: &end1},
		{ID: "paragraph-1", Order: 2, Text: "first untimed"},
		{ID: "paragraph-2", Order: 3, Text: "second untimed"},
		{ID: "cue-3", Order: 4, Text: "third timed", StartMS: &start2, EndMS: &end2},
	}
	docs, err := BuildUnitDocuments(units, MergeConfig{MaxCharacters: 80, MaxGapMS: 500, Version: "v2"}, DocumentOptions{
		SourceType: "transcript",
		SourceID:   "track-1",
		Locale:     "en",
		BaseURL:    "https://example.org/session/1",
		ParentURL:  "https://example.org/session/1",
	})
	if err != nil {
		t.Fatalf("build unit documents: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("expected timed, untimed, timed segments, got %#v", docs)
	}
	if docs[0].Body != "first timed second timed" || docs[0].StartMS == nil || *docs[0].StartMS != 0 || docs[0].EndMS == nil || *docs[0].EndMS != 2000 {
		t.Fatalf("timed merge = %#v", docs[0])
	}
	if docs[0].AnchorURL != "https://example.org/session/1#t=0" || docs[0].Numeric["start_ms"] != 0 || docs[0].Numeric["end_ms"] != 2000 {
		t.Fatalf("timed evidence = %#v", docs[0])
	}
	if docs[1].Body != "first untimed second untimed" || docs[1].StartMS != nil || docs[1].EndMS != nil || docs[1].AnchorURL != "" {
		t.Fatalf("untimed merge = %#v", docs[1])
	}
	if _, exists := docs[1].Numeric["start_ms"]; exists {
		t.Fatalf("untimed start numeric = %#v", docs[1].Numeric)
	}
	if _, exists := docs[1].Numeric["end_ms"]; exists {
		t.Fatalf("untimed end numeric = %#v", docs[1].Numeric)
	}
	for ordinal, doc := range docs {
		if doc.ChunkOrdinal == nil || *doc.ChunkOrdinal != ordinal {
			t.Fatalf("chunk %d ordinal = %#v", ordinal, doc.ChunkOrdinal)
		}
	}
}

func TestBuildUnitDocumentsIsDeterministicAndRuneBounded(t *testing.T) {
	units := []NormalizedUnit{{ID: "unicode-1", Order: 0, Text: "菩提心བསྐྱེད"}}
	cfg := MergeConfig{MaxCharacters: 4, Version: "unicode-v1"}
	opts := DocumentOptions{SourceType: "transcript", SourceID: "track-unicode", Locale: "bo"}
	left, err := BuildUnitDocuments(units, cfg, opts)
	if err != nil {
		t.Fatalf("build first projection: %v", err)
	}
	right, err := BuildUnitDocuments(units, cfg, opts)
	if err != nil {
		t.Fatalf("build second projection: %v", err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("normalized projection is not deterministic:\nleft=%#v\nright=%#v", left, right)
	}
	var rebuilt strings.Builder
	for ordinal, doc := range left {
		if utf8.RuneCountInString(doc.Body) > cfg.MaxCharacters {
			t.Fatalf("chunk %d exceeds rune limit: %q", ordinal, doc.Body)
		}
		if doc.ChunkOrdinal == nil || *doc.ChunkOrdinal != ordinal || doc.ID == "" {
			t.Fatalf("chunk %d identity/ordinal = %#v", ordinal, doc)
		}
		if doc.SourceVersion != cfg.Version {
			t.Fatalf("chunk %d source version = %q", ordinal, doc.SourceVersion)
		}
		rebuilt.WriteString(doc.Body)
	}
	if rebuilt.String() != units[0].Text {
		t.Fatalf("complete text coverage = %q, want %q", rebuilt.String(), units[0].Text)
	}
	otherLocale, err := BuildUnitDocuments(units, cfg, DocumentOptions{SourceType: "transcript", SourceID: "track-unicode", Locale: "zh"})
	if err != nil || len(otherLocale) != len(left) {
		t.Fatalf("build other locale: docs=%#v err=%v", otherLocale, err)
	}
	if otherLocale[0].ID == left[0].ID {
		t.Fatalf("normalized document identity does not include locale: %q", left[0].ID)
	}
}

func TestBuildUnitDocumentsSkipsBlankUnitsDeterministically(t *testing.T) {
	docs, err := BuildUnitDocuments([]NormalizedUnit{
		{Order: 0, Text: " \n\t "},
		{ID: "kept", Order: 1, Text: "searchable"},
	}, MergeConfig{Version: "v1"}, DocumentOptions{SourceType: "transcript", SourceID: "track-1", Locale: "en"})
	if err != nil {
		t.Fatalf("build units: %v", err)
	}
	if len(docs) != 1 || docs[0].Body != "searchable" || docs[0].ChunkOrdinal == nil || *docs[0].ChunkOrdinal != 0 {
		t.Fatalf("blank unit handling = %#v", docs)
	}
}

func TestBuildUnitDocumentsRejectsInvalidUnits(t *testing.T) {
	negative, zero, one, two := int64(-1), int64(0), int64(1), int64(2)
	tests := map[string][]NormalizedUnit{
		"negative order":     {{ID: "one", Order: -1, Text: "one"}},
		"nonincreasing":      {{ID: "one", Order: 1, Text: "one"}, {ID: "two", Order: 1, Text: "two"}},
		"empty identity":     {{Order: 0, Text: "one"}},
		"duplicate identity": {{ID: "same", Order: 0, Text: "one"}, {ID: "same", Order: 1, Text: "two"}},
		"partial timing":     {{ID: "one", Order: 0, Text: "one", StartMS: &zero}},
		"negative timing":    {{ID: "one", Order: 0, Text: "one", StartMS: &negative, EndMS: &one}},
		"empty timing":       {{ID: "one", Order: 0, Text: "one", StartMS: &one, EndMS: &one}},
		"reversed timing":    {{ID: "one", Order: 0, Text: "one", StartMS: &two, EndMS: &one}},
		"timing order":       {{ID: "one", Order: 0, Text: "one", StartMS: &one, EndMS: &two}, {ID: "two", Order: 1, Text: "two", StartMS: &zero, EndMS: &one}},
	}
	for name, units := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildUnitDocuments(units, MergeConfig{Version: "v1"}, DocumentOptions{SourceType: "transcript", SourceID: "track-1", Locale: "en"}); err == nil {
				t.Fatalf("expected invalid units to fail: %#v", units)
			}
		})
	}
}

func TestBuildUnitDocumentsRequiresSourceIdentity(t *testing.T) {
	unit := []NormalizedUnit{{ID: "unit-1", Order: 0, Text: "searchable"}}
	tests := map[string]struct {
		opts    DocumentOptions
		wantErr string
	}{
		"source type": {opts: DocumentOptions{SourceID: "track-1", SourceType: " \t "}, wantErr: "source type is required"},
		"source id":   {opts: DocumentOptions{SourceType: "transcript", SourceID: " \n "}, wantErr: "source id is required"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := BuildUnitDocuments(unit, MergeConfig{Version: "v1"}, test.opts)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("blank %s error = %v", name, err)
			}
		})
	}
}

func TestBuildUnitDocumentsCanonicalizesSourceIdentity(t *testing.T) {
	units := []NormalizedUnit{{ID: "unit-1", Order: 0, Text: "searchable"}}
	cfg := MergeConfig{Version: "v1"}
	canonical, err := BuildUnitDocuments(units, cfg, DocumentOptions{SourceType: "transcript", SourceID: "track-1"})
	if err != nil {
		t.Fatalf("build canonical documents: %v", err)
	}
	spaced, err := BuildUnitDocuments(units, cfg, DocumentOptions{SourceType: " transcript ", SourceID: " track-1 "})
	if err != nil {
		t.Fatalf("build spaced documents: %v", err)
	}
	if len(canonical) != 1 || len(spaced) != 1 || canonical[0].ID != spaced[0].ID {
		t.Fatalf("source identity canonicalization: canonical=%#v spaced=%#v", canonical, spaced)
	}
	if spaced[0].SourceType != "transcript" || spaced[0].SourceID != "track-1" {
		t.Fatalf("emitted source identity = %q/%q", spaced[0].SourceType, spaced[0].SourceID)
	}
}

func TestBuildSegmentDocumentsPreservesLegacyIDAndTiming(t *testing.T) {
	docs := BuildSegmentDocuments([]Cue{{Start: 1000, End: 2000, Text: "legacy"}}, DocumentOptions{
		SourceType: "transcript", SourceID: "track-1", Locale: "en", Version: "v1", BaseURL: "/session/1",
	})
	if len(docs) != 1 {
		t.Fatalf("legacy documents = %#v", docs)
	}
	doc := docs[0]
	if doc.ID != "c51724d08fc119afacf449b5b9a58ed066b6f13545ec685d0fff5df4c22db227" {
		t.Fatalf("legacy document id = %q", doc.ID)
	}
	if doc.StartMS == nil || *doc.StartMS != 1000 || doc.EndMS == nil || *doc.EndMS != 2000 || doc.AnchorURL != "/session/1#t=1" {
		t.Fatalf("legacy timing = %#v", doc)
	}
}
