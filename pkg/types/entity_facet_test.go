package types

import "testing"

func TestBuildAndMergeEntityFacetsDeduplicateGlobalIdentities(t *testing.T) {
	req := FacetRequest{Field: "topic", CountBy: FacetCountByResultID, IdentityLimit: 10}
	left := BuildEntityFacet(req, map[string]map[string]struct{}{"practice": {"event:1": {}, "event:2": {}}}, []string{"missing"})
	right := BuildEntityFacet(req, map[string]map[string]struct{}{"practice": {"event:1": {}, "event:3": {}}}, nil)
	merged := MergeEntityFacets(req, left, right)
	if merged.Accuracy != FacetCountAccuracyExact {
		t.Fatalf("accuracy = %q", merged.Accuracy)
	}
	for _, value := range merged.Values {
		if value.Value == "practice" && (value.Count != 3 || len(value.EntityIDs) != 3) {
			t.Fatalf("practice = %#v", value)
		}
	}
	if len(left.Values) != 2 || left.Values[0].Value != "missing" && left.Values[1].Value != "missing" {
		t.Fatalf("selected zero missing: %#v", left.Values)
	}
}

func BenchmarkBuildEntityFacetBounded(b *testing.B) {
	identities := map[string]map[string]struct{}{}
	for bucket := 0; bucket < 20; bucket++ {
		value := string(rune('a' + bucket))
		identities[value] = map[string]struct{}{}
		for entity := 0; entity < 250; entity++ {
			identities[value][value+":"+string(rune(entity))] = struct{}{}
		}
	}
	req := FacetRequest{Field: "topic", CountBy: FacetCountByResultID, IdentityLimit: 250}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BuildEntityFacet(req, identities, nil)
	}
}

func TestBuildEntityFacetReportsBoundedIdentityAccuracy(t *testing.T) {
	facet := BuildEntityFacet(FacetRequest{Field: "topic", CountBy: FacetCountByResultID, IdentityLimit: 1}, map[string]map[string]struct{}{"practice": {"event:1": {}, "event:2": {}}}, nil)
	if facet.Accuracy != FacetCountAccuracyExact || facet.Values[0].EntityIDsComplete || facet.Values[0].Count != 2 || len(facet.Values[0].EntityIDs) != 1 {
		t.Fatalf("facet = %#v", facet)
	}
	merged := MergeEntityFacets(FacetRequest{Field: "topic", CountBy: FacetCountByResultID}, facet)
	if merged.Accuracy != FacetCountAccuracyLowerBound {
		t.Fatalf("merged accuracy = %q", merged.Accuracy)
	}
}
