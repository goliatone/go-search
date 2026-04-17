package content

import (
	"context"
	"testing"
	"time"
)

func TestProjectorDerivesPublishedYearWithoutNumericMap(t *testing.T) {
	publishedAt := time.Date(2024, time.May, 1, 8, 0, 0, 0, time.UTC)
	projector := NewProjector(ProjectorConfig{Index: "documents", SourceType: "content"})

	docs, err := projector.Project(context.Background(), Record{
		ID:         "doc-1",
		Type:       "document",
		SourceType: "content",
		Metadata: map[string]any{
			"published_at": publishedAt,
		},
	})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if year, ok := docs[0].Fields["published_year"].(int); !ok || year != 2024 {
		t.Fatalf("expected published_year field, got %#v", docs[0].Fields["published_year"])
	}
	if docs[0].Numeric["published_year"] != 2024 {
		t.Fatalf("expected published_year numeric field, got %#v", docs[0].Numeric["published_year"])
	}
}
