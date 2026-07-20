package types

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSearchFoundationZeroValueJSONRemainsAdditive(t *testing.T) {
	payload, err := json.Marshal(SearchResultPage{})
	if err != nil {
		t.Fatal(err)
	}
	body := string(payload)
	if strings.Contains(body, `"counts"`) || strings.Contains(body, `"total_accuracy"`) {
		t.Fatalf("optional foundation fields leaked into zero value: %s", body)
	}
}

func TestEvidenceAndCountAvailabilityContracts(t *testing.T) {
	if err := (MatchEvidenceSummary{Exact: true, Status: EvidenceStatusComplete}).Validate(); err != nil {
		t.Fatalf("complete evidence: %v", err)
	}
	if err := (MatchEvidenceSummary{Status: EvidenceStatusUnavailable}).Validate(); err == nil {
		t.Fatal("expected unavailable evidence without diagnostic to fail")
	}
	if err := (SearchCount{Accuracy: CountAccuracyExact, Value: 0}).Validate(); err != nil {
		t.Fatalf("exact zero: %v", err)
	}
	if err := (SearchCount{Accuracy: CountAccuracyUnavailable}).Validate(); err == nil {
		t.Fatal("expected unavailable count without diagnostic to fail")
	}
}

func TestBoundedSearchSnippetUsesRuneSafeByteLimit(t *testing.T) {
	value := strings.Repeat("界", 400)
	got := BoundedSearchSnippet(&SearchSnippet{Text: value, Highlighted: value})
	if len(got.Text) > MaxEvidenceSnippetBytes || len(got.Highlighted) > MaxEvidenceSnippetBytes {
		t.Fatalf("snippet exceeded byte limit: %d/%d", len(got.Text), len(got.Highlighted))
	}
	if !utf8.ValidString(got.Text) || !utf8.ValidString(got.Highlighted) {
		t.Fatal("snippet was truncated inside a UTF-8 rune")
	}
}
