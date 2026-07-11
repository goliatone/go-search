package ranking

import (
	"github.com/goliatone/go-search/pkg/types"
	"testing"
)

func TestFuseRRFIgnoresProviderScoreScaleAndUsesStableTies(t *testing.T) {
	a, b := 1000000.0, .01
	got := FuseRRF([]RankedList{{Index: "a", Hits: []types.SearchHit{{ID: "x", Retrieval: &types.AppliedRetrievalSignals{ProviderScore: &a}}, {ID: "y"}}}, {Index: "b", Hits: []types.SearchHit{{ID: "y", Retrieval: &types.AppliedRetrievalSignals{ProviderScore: &b}}, {ID: "x"}}}}, 60)
	if len(got) != 2 || got[0].ID != "x" {
		t.Fatalf("got=%#v", got)
	}
	if len(got[0].Retrieval.Contributions) != 2 {
		t.Fatal("missing evidence")
	}
}
func TestFuseRRFIndexWeight(t *testing.T) {
	got := FuseRRF([]RankedList{{Index: "a", Weight: 2, Hits: []types.SearchHit{{ID: "x"}}}, {Index: "b", Hits: []types.SearchHit{{ID: "y"}}}}, 60)
	if got[0].ID != "x" {
		t.Fatalf("got=%#v", got)
	}
}
