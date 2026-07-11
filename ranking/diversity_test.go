package ranking

import (
	"github.com/goliatone/go-search/pkg/types"
	"testing"
)

func TestApplyDiversityDedupesAndCapsFamilies(t *testing.T) {
	hits := []types.SearchHit{{ID: "a", URL: "https://x.test/a?utm=1", FinalScore: 3, Fields: map[string]any{"family": "f"}}, {ID: "dup", URL: "https://x.test/a#x", FinalScore: 2.9, Fields: map[string]any{"family": "z"}}, {ID: "b", URL: "https://x.test/b", FinalScore: 2, Fields: map[string]any{"family": "f"}}, {ID: "c", URL: "https://x.test/c", FinalScore: 1, Fields: map[string]any{"family": "f"}}}
	detailed := ApplyDiversityDetailed(hits, DiversityConfig{FamilyField: "family", MaxPerFamily: 2, RepeatedPenalty: .1})
	got := detailed.Hits
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("got=%#v", got)
	}
	if got[1].FinalScore != 1.9 {
		t.Fatalf("score=%v", got[1].FinalScore)
	}
	if len(detailed.Suppressed) != 2 {
		t.Fatalf("suppressed=%#v", detailed.Suppressed)
	}
}
