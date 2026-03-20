package gocms

import (
	"context"
	"testing"

	cmsactivity "github.com/goliatone/go-cms/pkg/activity"
	"github.com/goliatone/go-search/pkg/types"
)

type recordingIndexer struct {
	indexed []string
	deleted []string
}

func (r *recordingIndexer) IndexRecord(_ context.Context, index string, recordID string) ([]types.Document, error) {
	r.indexed = append(r.indexed, index+":"+recordID)
	return nil, nil
}

func (r *recordingIndexer) DeleteRecord(_ context.Context, index string, recordID string) error {
	r.deleted = append(r.deleted, index+":"+recordID)
	return nil
}

func TestActivityHookIndexesPublishedSearchableContent(t *testing.T) {
	indexer := &recordingIndexer{}
	hook := NewActivityHook(ActivityHookConfig{Indexer: indexer, ContentIndex: "content"})
	err := hook.Notify(context.Background(), cmsactivity.Event{
		Verb:       "publish",
		ObjectType: "content",
		ObjectID:   "abc",
		Metadata: map[string]any{
			"status":         "published",
			"search_enabled": true,
		},
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(indexer.indexed) != 1 || indexer.indexed[0] != "content:abc" {
		t.Fatalf("indexed = %#v", indexer.indexed)
	}
}

func TestActivityHookDeletesUnpublishedRecords(t *testing.T) {
	indexer := &recordingIndexer{}
	hook := NewActivityHook(ActivityHookConfig{Indexer: indexer, ContentIndex: "content"})
	err := hook.Notify(context.Background(), cmsactivity.Event{
		Verb:       "update",
		ObjectType: "content",
		ObjectID:   "abc",
		Metadata: map[string]any{
			"status":         "draft",
			"search_enabled": true,
		},
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(indexer.deleted) != 1 || indexer.deleted[0] != "content:abc" {
		t.Fatalf("deleted = %#v", indexer.deleted)
	}
}
