package types

import "testing"

func TestBuildFacetAddsHierarchyMetadataAndSelection(t *testing.T) {
	facet := BuildFacet(FacetRequest{
		Field:       "topic_hierarchy",
		Kind:        FacetKindHierarchical,
		Disjunctive: true,
	}, map[string]int{
		"Teaching Topics":        4,
		"Teaching Topics > Tara": 2,
	}, []string{"Teaching Topics > Tara"})

	if facet.Kind != FacetKindHierarchical || !facet.Disjunctive {
		t.Fatalf("facet = %+v", facet)
	}
	if len(facet.Values) < 2 {
		t.Fatalf("values = %+v", facet.Values)
	}
	for _, value := range facet.Values {
		if value.Value == "Teaching Topics > Tara" {
			if value.Label != "Tara" || value.Level != 1 || value.ParentValue != "Teaching Topics" || !value.Selected {
				t.Fatalf("unexpected hierarchical value: %+v", value)
			}
			return
		}
	}
	t.Fatalf("expected Tara value in %+v", facet.Values)
}

func TestRemoveFacetFilterDropsOnlyTargetField(t *testing.T) {
	expr := AndExpr{Terms: []FilterExpr{
		TermExpr{Field: "topic", Op: FilterOpEQ, Value: "archive"},
		TermExpr{Field: "locale", Op: FilterOpEQ, Value: "en"},
	}}
	filtered := RemoveFacetFilter(expr, "topic")
	term, ok := filtered.(TermExpr)
	if !ok {
		t.Fatalf("filtered expr = %#v", filtered)
	}
	if term.Field != "locale" {
		t.Fatalf("remaining term = %#v", term)
	}
}
