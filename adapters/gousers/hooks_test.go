package gousers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

type recordingMutator struct {
	indexed []string
	deleted []string
}

func (r *recordingMutator) IndexRecord(_ context.Context, index, registrationKey, recordID string) ([]types.Document, error) {
	r.indexed = append(r.indexed, index+":"+registrationKey+":"+recordID)
	return nil, nil
}

func (r *recordingMutator) DeleteRecord(_ context.Context, index, registrationKey, recordID string) error {
	r.deleted = append(r.deleted, index+":"+registrationKey+":"+recordID)
	return nil
}

type recordingActivitySink struct {
	records []userstypes.ActivityRecord
}

func (r *recordingActivitySink) Log(_ context.Context, record userstypes.ActivityRecord) error {
	r.records = append(r.records, record)
	return nil
}

type failingActivitySink struct{}

func (failingActivitySink) Log(context.Context, userstypes.ActivityRecord) error {
	return errors.New("sink unavailable")
}

type recordingLogger struct {
	errors []map[string]any
}

func (l *recordingLogger) Debug(string, map[string]any) {}
func (l *recordingLogger) Info(string, map[string]any)  {}
func (l *recordingLogger) Warn(string, map[string]any)  {}
func (l *recordingLogger) Error(_ string, metadata map[string]any) {
	l.errors = append(l.errors, metadata)
}

type recordingMetricsHook struct {
	counts map[string]int64
}

func (m *recordingMetricsHook) Observe(context.Context, string, float64, map[string]string) {}

func (m *recordingMetricsHook) Count(_ context.Context, metric string, delta int64, _ map[string]string) {
	if m.counts == nil {
		m.counts = map[string]int64{}
	}
	m.counts[metric] += delta
}

func TestLifecycleHooksReindexOnLifecycleChange(t *testing.T) {
	indexer := &recordingMutator{}
	hooks := NewLifecycleHooks(LifecycleHooksConfig{Indexer: indexer, Index: "users", RegistrationKey: "user"}).Hooks()
	userID := uuid.New()
	hooks.AfterLifecycle(context.Background(), userstypes.LifecycleEvent{UserID: userID})
	if len(indexer.indexed) != 1 || indexer.indexed[0] != "users:user:"+userID.String() {
		t.Fatalf("indexed = %#v", indexer.indexed)
	}
}

func TestLifecycleHooksDeleteOnArchive(t *testing.T) {
	indexer := &recordingMutator{}
	hooks := NewLifecycleHooks(LifecycleHooksConfig{Indexer: indexer, Index: "users", RegistrationKey: "user"}).Hooks()
	userID := uuid.New()
	hooks.AfterLifecycle(context.Background(), userstypes.LifecycleEvent{
		UserID:  userID,
		ToState: userstypes.LifecycleStateArchived,
	})
	if len(indexer.deleted) != 1 || indexer.deleted[0] != "users:user:"+userID.String() {
		t.Fatalf("deleted = %#v", indexer.deleted)
	}
}

func TestLifecycleHooksReindexOnUserActivity(t *testing.T) {
	indexer := &recordingMutator{}
	hooks := NewLifecycleHooks(LifecycleHooksConfig{Indexer: indexer, Index: "users", RegistrationKey: "user"}).Hooks()
	userID := uuid.New()
	hooks.AfterActivity(context.Background(), userstypes.ActivityRecord{
		Verb:       "user.updated",
		ObjectType: "user",
		ObjectID:   userID.String(),
	})
	if len(indexer.indexed) != 1 || indexer.indexed[0] != "users:user:"+userID.String() {
		t.Fatalf("indexed = %#v", indexer.indexed)
	}
}

func TestActivitySinkHookMapsSearchEventToUsersActivity(t *testing.T) {
	sink := &recordingActivitySink{}
	hook := ActivitySinkHook{Sink: sink}
	tenantID := uuid.New()
	orgID := uuid.New()
	userID := uuid.New()
	hook.Notify(context.Background(), types.ActivityEvent{
		Channel:    "search",
		Verb:       "indexed",
		ObjectType: "user",
		ObjectID:   userID.String(),
		RecordID:   userID.String(),
		TenantID:   tenantID.String(),
		OrgID:      orgID.String(),
		OccurredAt: time.Now().UnixMilli(),
		Metadata: map[string]any{
			"index":   "users",
			"user_id": userID.String(),
		},
	})
	if len(sink.records) != 1 {
		t.Fatalf("records = %#v", sink.records)
	}
	if sink.records[0].Channel != "search" || sink.records[0].TenantID != tenantID || sink.records[0].OrgID != orgID || sink.records[0].UserID != userID {
		t.Fatalf("record = %#v", sink.records[0])
	}
}

func TestActivitySinkHookReportsSinkFailures(t *testing.T) {
	logger := &recordingLogger{}
	metrics := &recordingMetricsHook{}
	hook := ActivitySinkHook{
		Sink:    failingActivitySink{},
		Logger:  logger,
		Metrics: []types.MetricsHook{metrics},
	}

	hook.Notify(context.Background(), types.ActivityEvent{
		Channel:    "search",
		Verb:       "deleted",
		ObjectType: "user",
		ObjectID:   uuid.NewString(),
		RecordID:   uuid.NewString(),
		OccurredAt: time.Now().UnixMilli(),
	})

	if metrics.counts["search.adapter.gousers.activity_sink.error.count"] != 1 {
		t.Fatalf("metrics = %#v", metrics.counts)
	}
	if len(logger.errors) != 1 {
		t.Fatalf("logger errors = %#v", logger.errors)
	}
}
