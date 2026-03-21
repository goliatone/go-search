package media

import "testing"

func TestHierarchicalFacetValuesBuildsPrefixPaths(t *testing.T) {
	got := HierarchicalFacetValues([]string{"Teaching Topics", "Tara"})
	if len(got) != 2 {
		t.Fatalf("hierarchy = %#v", got)
	}
	if got[0] != "Teaching Topics" || got[1] != "Teaching Topics > Tara" {
		t.Fatalf("hierarchy = %#v", got)
	}
}

func TestTopicLandingPresetProvidesCanonicalHierarchyFilter(t *testing.T) {
	preset, ok := TopicLandingPreset("tara")
	if !ok {
		t.Fatalf("expected tara preset")
	}
	values := preset.FacetFilter[FacetFieldTopicHierarchy]
	if len(values) != 1 || values[0] != "Teaching Topics > Tara" {
		t.Fatalf("facet filter = %#v", preset.FacetFilter)
	}
}
