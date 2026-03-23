package gocms

import (
	"context"
	"testing"

	cmslifecycle "github.com/goliatone/go-cms/pkg/lifecycle"
	"github.com/goliatone/go-search/pkg/types"
)

type recordingIndexer struct {
	indexed []string
	deleted []string
}

func (r *recordingIndexer) IndexRecord(_ context.Context, index, registrationKey, recordID string) ([]types.Document, error) {
	r.indexed = append(r.indexed, index+":"+registrationKey+":"+recordID)
	return nil, nil
}

func (r *recordingIndexer) DeleteRecord(_ context.Context, index, registrationKey, recordID string) error {
	r.deleted = append(r.deleted, index+":"+registrationKey+":"+recordID)
	return nil
}

func TestLifecycleHookIndexesSearchableEvent(t *testing.T) {
	indexer := &recordingIndexer{}
	hook := NewLifecycleHook(LifecycleHookConfig{
		Indexer: indexer,
		Routes: []Route{
			{
				ResourceType:    "content",
				ContentTypeSlug: "article",
				Index:           "content_shared",
				RegistrationKey: "document",
			},
			{
				ResourceType:    "content",
				ContentTypeSlug: "article",
				Index:           "documents",
				RegistrationKey: "document",
			},
		},
	})
	err := hook.Notify(context.Background(), cmslifecycle.Event{
		ResourceType:    "content",
		RecordID:        "abc",
		Transition:      "publish",
		ContentTypeSlug: "article",
		SearchEnabled:   true,
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(indexer.indexed) != 2 || indexer.indexed[0] != "content_shared:document:abc" || indexer.indexed[1] != "documents:document:abc" {
		t.Fatalf("indexed = %#v", indexer.indexed)
	}
}

func TestLifecycleHookDeletesWhenSearchDisabled(t *testing.T) {
	indexer := &recordingIndexer{}
	hook := NewLifecycleHook(LifecycleHookConfig{
		Indexer: indexer,
		Routes: []Route{{
			ResourceType: "page",
			Index:        "pages",
		}},
	})
	err := hook.Notify(context.Background(), cmslifecycle.Event{
		ResourceType:  "page",
		RecordID:      "page-1",
		Transition:    "update",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(indexer.deleted) != 1 || indexer.deleted[0] != "pages::page-1" {
		t.Fatalf("deleted = %#v", indexer.deleted)
	}
}

func TestLifecycleHookReindexesTranslationDelete(t *testing.T) {
	indexer := &recordingIndexer{}
	hook := NewLifecycleHook(LifecycleHookConfig{
		Indexer: indexer,
		Routes: []Route{{
			ResourceType:    "content",
			ContentTypeSlug: "article",
			Index:           "content_shared",
			RegistrationKey: "document",
		}},
	})
	err := hook.Notify(context.Background(), cmslifecycle.Event{
		ResourceType:    "content",
		RecordID:        "abc",
		Transition:      "translation_delete",
		ContentTypeSlug: "article",
		SearchEnabled:   true,
	})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(indexer.indexed) != 1 || indexer.indexed[0] != "content_shared:document:abc" {
		t.Fatalf("indexed = %#v", indexer.indexed)
	}
	if len(indexer.deleted) != 0 {
		t.Fatalf("deleted = %#v", indexer.deleted)
	}
}
