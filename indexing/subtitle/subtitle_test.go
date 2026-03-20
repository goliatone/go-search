package subtitle

import "testing"

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
