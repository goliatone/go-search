package gousers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/goliatone/go-search/pkg/types"
)

type recordingMutator struct {
	indexed []string
}

func (r *recordingMutator) IndexRecord(_ context.Context, index string, recordID string) ([]types.Document, error) {
	r.indexed = append(r.indexed, index+":"+recordID)
	return nil, nil
}

func (r *recordingMutator) DeleteRecord(context.Context, string, string) error {
	return nil
}

type recordingActivitySink struct {
	records []userstypes.ActivityRecord
}

func (r *recordingActivitySink) Log(_ context.Context, record userstypes.ActivityRecord) error {
	r.records = append(r.records, record)
	return nil
}

func TestLifecycleHooksReindexOnLifecycleChange(t *testing.T) {
	indexer := &recordingMutator{}
	hooks := NewLifecycleHooks(LifecycleHooksConfig{Indexer: indexer, Index: "users"}).Hooks()
	userID := uuid.New()
	hooks.AfterLifecycle(context.Background(), userstypes.LifecycleEvent{UserID: userID})
	if len(indexer.indexed) != 1 || indexer.indexed[0] != "users:"+userID.String() {
		t.Fatalf("indexed = %#v", indexer.indexed)
	}
}

func TestActivitySinkHookMapsSearchEventToUsersActivity(t *testing.T) {
	sink := &recordingActivitySink{}
	hook := ActivitySinkHook{Sink: sink}
	tenantID := uuid.New()
	hook.Notify(context.Background(), types.ActivityEvent{
		Channel:    "search",
		Verb:       "indexed",
		ObjectType: "documents",
		ObjectID:   "media",
		TenantID:   tenantID.String(),
		OccurredAt: time.Now().UnixMilli(),
		Metadata:   map[string]any{"index": "media"},
	})
	if len(sink.records) != 1 {
		t.Fatalf("records = %#v", sink.records)
	}
	if sink.records[0].Channel != "search" || sink.records[0].TenantID != tenantID {
		t.Fatalf("record = %#v", sink.records[0])
	}
}
