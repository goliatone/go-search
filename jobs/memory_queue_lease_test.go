package jobs

import (
	"context"
	"testing"
	"time"

	job "github.com/goliatone/go-job"
)

func TestMemoryQueueReclaimsExpiredLeaseWithFencingToken(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	queue := NewMemoryQueueWithConfig(MemoryQueueConfig{LeaseTTL: 10 * time.Second, Now: func() time.Time { return now }})
	ctx := context.Background()
	if _, err := queue.Enqueue(ctx, &job.ExecutionMessage{}); err != nil {
		t.Fatal(err)
	}
	first, err := queue.Dequeue(ctx)
	if err != nil || first == nil {
		t.Fatalf("first dequeue: delivery=%v err=%v", first, err)
	}
	firstLease := first.(*memoryDelivery)
	if firstLease.Attempts() != 1 {
		t.Fatalf("first attempts = %d", firstLease.Attempts())
	}
	now = now.Add(11 * time.Second)
	second, err := queue.Dequeue(ctx)
	if err != nil || second == nil {
		t.Fatalf("reclaimed dequeue: delivery=%v err=%v", second, err)
	}
	secondLease := second.(*memoryDelivery)
	if secondLease.Attempts() != 2 {
		t.Fatalf("reclaimed attempts = %d", secondLease.Attempts())
	}
	if err := first.Ack(ctx); err == nil {
		t.Fatal("expired delivery acknowledged with stale fencing token")
	}
	if err := second.Ack(ctx); err != nil {
		t.Fatalf("ack reclaimed delivery: %v", err)
	}
}

func TestMemoryQueueRejectsInvalidAndExpiredLeaseExtensions(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	queue := NewMemoryQueueWithConfig(MemoryQueueConfig{LeaseTTL: time.Second, Now: func() time.Time { return now }})
	ctx := context.Background()
	_, _ = queue.Enqueue(ctx, &job.ExecutionMessage{})
	delivery, err := queue.Dequeue(ctx)
	if err != nil || delivery == nil {
		t.Fatalf("dequeue: delivery=%v err=%v", delivery, err)
	}
	lease := delivery.(*memoryDelivery)
	if err := lease.ExtendLease(ctx, 0); err == nil {
		t.Fatal("zero lease extension was accepted")
	}
	now = now.Add(2 * time.Second)
	if err := lease.ExtendLease(ctx, time.Second); err == nil {
		t.Fatal("expired lease was extended")
	}
}
