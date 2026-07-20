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

func TestBuildMediaProjectionNeverFabricatesDomainFacts(t *testing.T) {
	projection, err := BuildMediaProjection(MediaRecord{Topic: "architecture"}, "en", DurationBucketPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.People) != 0 || projection.Location != "" || projection.Series != "" || projection.PublishedYear != 0 || projection.DurationSeconds != 0 {
		t.Fatalf("projection fabricated facts: %#v", projection)
	}
}

func TestDurationBucketPolicyUsesContiguousHalfOpenBounds(t *testing.T) {
	policy := DurationBucketPolicy{Buckets: []DurationBucketRange{
		{Key: "under_30", MinSeconds: 0, MaxSeconds: intRef(1800)},
		{Key: "30_60", MinSeconds: 1800, MaxSeconds: intRef(3660)},
		{Key: "61_90", MinSeconds: 3660, MaxSeconds: intRef(5460)},
		{Key: "over_90", MinSeconds: 5460},
	}}
	for seconds, want := range map[int]string{1799: "under_30", 1800: "30_60", 3659: "30_60", 3660: "61_90", 5459: "61_90", 5460: "over_90"} {
		got, err := DurationBucketWithPolicy(seconds, policy)
		if err != nil || got != want {
			t.Fatalf("seconds=%d got=%q err=%v want=%q", seconds, got, err, want)
		}
	}
}
