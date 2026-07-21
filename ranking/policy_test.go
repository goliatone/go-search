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

func TestGroupHitsByResultIDCollapsesAcrossIndexes(t *testing.T) {
	hits := []types.SearchHit{
		{ID: "event-hit", ResultID: "event:shared", FinalScore: 10, Document: &types.Document{Index: "site_content"}},
		{ID: "transcript-hit", ResultID: "event:shared", FinalScore: 9, Document: &types.Document{Index: "archive_media"}},
	}

	groups := GroupHitsBy(hits, "result_id")
	if len(groups) != 1 || groups[0].Key != "event:shared" || groups[0].Count != 2 {
		t.Fatalf("result entity groups = %+v", groups)
	}
}
