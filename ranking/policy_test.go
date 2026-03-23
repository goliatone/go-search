package ranking

import (
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

func TestGroupHitsNamespacesKeysByIndex(t *testing.T) {
	hits := []types.SearchHit{
		{
			ID:         "segment-1",
			Title:      "Shared Parent",
			FinalScore: 10,
			Parent:     &types.SearchParent{ID: "shared-parent", Title: "Shared Parent"},
			Document:   &types.Document{Index: "videos"},
		},
		{
			ID:         "segment-2",
			Title:      "Shared Parent",
			FinalScore: 9,
			Parent:     &types.SearchParent{ID: "shared-parent", Title: "Shared Parent"},
			Document:   &types.Document{Index: "documents"},
		},
	}

	groups := GroupHits(hits)
	if len(groups) != 2 {
		t.Fatalf("expected two groups for colliding parent ids across indexes, got %+v", groups)
	}
	if groups[0].Key != "shared-parent" || groups[1].Key != "shared-parent" {
		t.Fatalf("expected external group key to remain parent id, got %+v", groups)
	}
	leftKey := groups[0].Metadata["group_key"]
	rightKey := groups[1].Metadata["group_key"]
	if leftKey == rightKey {
		t.Fatalf("expected internal group keys to be namespaced, got %+v", groups)
	}
}
