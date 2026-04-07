package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-job/queue"
	"github.com/goliatone/go-job/queue/cancellation"
	queuecmd "github.com/goliatone/go-job/queue/command"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type Client struct {
	enqueuer     queue.ScheduledEnqueuer
	statusReader queue.DispatchStatusReader
	cancelStore  cancellation.Store
	store        DispatchStore
	registry     *queuecmd.Registry
	tracker      *Tracker
	now          func() time.Time
}

type ClientConfig struct {
	Enqueuer     queue.ScheduledEnqueuer
	StatusReader queue.DispatchStatusReader
	CancelStore  cancellation.Store
	Store        DispatchStore
	Registry     *queuecmd.Registry
	Tracker      *Tracker
}

func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Enqueuer == nil {
		return nil, errs.ConfigurationError("jobs enqueuer is required", nil)
	}
	if cfg.Registry == nil {
		return nil, errs.ConfigurationError("jobs registry is required", nil)
	}
	if cfg.Tracker == nil {
		cfg.Tracker = NewTracker()
	}
	if cfg.Store != nil {
		cfg.Tracker.SetStore(cfg.Store)
	}
	return &Client{
		enqueuer:     cfg.Enqueuer,
		statusReader: cfg.StatusReader,
		cancelStore:  cfg.CancelStore,
		store:        cfg.Store,
		registry:     cfg.Registry,
		tracker:      cfg.Tracker,
		now:          time.Now,
	}, nil
}

func (c *Client) EnqueueIndexRecord(ctx context.Context, msg types.IndexRecordInput, opts DispatchOptions) (DispatchReceipt, error) {
	return c.enqueue(ctx, OperationIndexRecord, gcommand.GetMessageType(types.IndexRecordInput{}), msg, opts)
}

func (c *Client) EnqueueDeleteRecord(ctx context.Context, msg types.DeleteRecordInput, opts DispatchOptions) (DispatchReceipt, error) {
	return c.enqueue(ctx, OperationDeleteRecord, gcommand.GetMessageType(types.DeleteRecordInput{}), msg, opts)
}

func (c *Client) EnqueueReindexIndex(ctx context.Context, msg types.ReindexIndexInput, opts DispatchOptions) (DispatchReceipt, error) {
	return c.enqueue(ctx, OperationReindexIndex, gcommand.GetMessageType(types.ReindexIndexInput{}), msg, opts)
}

func (c *Client) EnqueueBatch(ctx context.Context, req BatchRequest) (BatchReceipt, error) {
	batchID := strings.TrimSpace(req.BatchID)
	if batchID == "" {
		batchID = newOpaqueKey("batch")
	}
	result := BatchReceipt{
		BatchID:  batchID,
		Receipts: make([]DispatchReceipt, 0, len(req.Items)),
	}
	for i, item := range req.Items {
		opts := item.Options
		opts.BatchID = batchID
		opts.BatchPosition = i

		var (
			receipt DispatchReceipt
			err     error
		)
		switch item.Operation {
		case OperationIndexRecord:
			if item.IndexRecord == nil {
				err = fmt.Errorf("batch item %d missing index record payload", i)
				break
			}
			receipt, err = c.EnqueueIndexRecord(ctx, *item.IndexRecord, opts)
		case OperationDeleteRecord:
			if item.DeleteRecord == nil {
				err = fmt.Errorf("batch item %d missing delete record payload", i)
				break
			}
			receipt, err = c.EnqueueDeleteRecord(ctx, *item.DeleteRecord, opts)
		case OperationReindexIndex:
			if item.ReindexIndex == nil {
				err = fmt.Errorf("batch item %d missing reindex payload", i)
				break
			}
			receipt, err = c.EnqueueReindexIndex(ctx, *item.ReindexIndex, opts)
		default:
			err = fmt.Errorf("batch item %d has unsupported operation %q", i, item.Operation)
		}
		if err != nil {
			result.Failures = append(result.Failures, BatchDispatchError{Position: i, Err: err})
			continue
		}
		result.Receipts = append(result.Receipts, receipt)
	}
	result.Completed = len(result.Failures) == 0
	if len(result.Failures) > 0 {
		return result, fmt.Errorf("batch enqueue completed with %d failures", len(result.Failures))
	}
	return result, nil
}

func (c *Client) Get(ctx context.Context, dispatchID string) (DispatchSnapshot, bool, error) {
	if c == nil {
		return DispatchSnapshot{}, false, nil
	}
	return c.tracker.Get(ctx, strings.TrimSpace(dispatchID), c.statusReader)
}

func (c *Client) ListBatch(ctx context.Context, batchID string) ([]DispatchSnapshot, error) {
	if c == nil {
		return nil, nil
	}
	return c.tracker.ListBatch(ctx, strings.TrimSpace(batchID), c.statusReader)
}

func (c *Client) Cancel(ctx context.Context, req CancelRequest) error {
	if c == nil {
		return errs.ConfigurationError("jobs client is required", nil)
	}
	if c.cancelStore == nil {
		return errs.ConfigurationError("jobs cancellation store is required", nil)
	}
	snapshot, ok, err := c.Get(ctx, req.DispatchID)
	if err != nil {
		return err
	}
	if !ok {
		return errs.InvalidInput("dispatch not found", map[string]any{"dispatch_id": req.DispatchID})
	}
	if err := c.cancelStore.Request(ctx, cancellation.Request{
		Key:         snapshot.OperationKey,
		Reason:      strings.TrimSpace(req.Reason),
		RequestedAt: c.now().UTC(),
	}); err != nil {
		return err
	}
	c.tracker.MarkCancelRequested(ctx, snapshot.DispatchID)
	return nil
}

func (c *Client) Restart(ctx context.Context, dispatchID string, opts RestartOptions) (DispatchReceipt, error) {
	snapshot, ok, err := c.Get(ctx, dispatchID)
	if err != nil {
		return DispatchReceipt{}, err
	}
	if !ok {
		return DispatchReceipt{}, errs.InvalidInput("dispatch not found", map[string]any{"dispatch_id": dispatchID})
	}
	switch snapshot.Operation {
	case OperationIndexRecord:
		msg, err := decodePayload[types.IndexRecordInput](snapshot.Payload)
		if err != nil {
			return DispatchReceipt{}, err
		}
		return c.EnqueueIndexRecord(ctx, msg, opts.DispatchOptions)
	case OperationDeleteRecord:
		msg, err := decodePayload[types.DeleteRecordInput](snapshot.Payload)
		if err != nil {
			return DispatchReceipt{}, err
		}
		return c.EnqueueDeleteRecord(ctx, msg, opts.DispatchOptions)
	case OperationReindexIndex:
		msg, err := decodePayload[types.ReindexIndexInput](snapshot.Payload)
		if err != nil {
			return DispatchReceipt{}, err
		}
		return c.EnqueueReindexIndex(ctx, msg, opts.DispatchOptions)
	default:
		return DispatchReceipt{}, errs.InvalidInput("unsupported restart operation", map[string]any{"operation": snapshot.Operation})
	}
}

func (c *Client) enqueue(ctx context.Context, operation Operation, commandID string, payload any, opts DispatchOptions) (DispatchReceipt, error) {
	if c == nil {
		return DispatchReceipt{}, errs.ConfigurationError("jobs client is required", nil)
	}
	operationKey := strings.TrimSpace(opts.OperationKey)
	if operationKey == "" {
		operationKey = newOpaqueKey("search-op")
	}
	dedupKey := strings.TrimSpace(opts.IdempotencyKey)
	metadata := cloneStringAnyMap(opts.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["operation"] = operation
	metadata["operation_key"] = operationKey
	if opts.BatchID != "" {
		metadata["batch_id"] = opts.BatchID
		metadata["batch_position"] = opts.BatchPosition
	}

	params, err := queuecmd.ParametersFromPayload(payload)
	if err != nil {
		return DispatchReceipt{}, err
	}
	draft := DispatchSnapshot{
		CommandID:     commandID,
		Operation:     operation,
		OperationKey:  operationKey,
		State:         string(queue.DispatchStateAccepted),
		CorrelationID: strings.TrimSpace(opts.CorrelationID),
		BatchID:       strings.TrimSpace(opts.BatchID),
		BatchPosition: opts.BatchPosition,
		Payload:       params,
		Metadata:      cloneStringAnyMap(opts.Metadata),
		Summary:       seedSummary(params),
	}
	if err := c.tracker.Prepare(ctx, draft); err != nil {
		return DispatchReceipt{}, err
	}

	receipt, err := queuecmd.EnqueuePayloadWithOptions(ctx, c.enqueuer, c.registry, commandID, payload, queuecmd.EnqueueOptions{
		Delay:            opts.Delay,
		RunAt:            opts.RunAt,
		IdempotencyKey:   dedupKey,
		DedupPolicy:      opts.DedupPolicy,
		CorrelationID:    opts.CorrelationID,
		Metadata:         metadata,
		IdempotencyStore: opts.IdempotencyStore,
		IdempotencyTTL:   opts.IdempotencyTTL,
	})
	if err != nil {
		c.tracker.Abandon(operationKey)
		return DispatchReceipt{}, err
	}

	result := DispatchReceipt{
		DispatchID:    receipt.DispatchID,
		EnqueuedAt:    receipt.EnqueuedAt.UTC(),
		CommandID:     commandID,
		Operation:     operation,
		OperationKey:  operationKey,
		CorrelationID: strings.TrimSpace(opts.CorrelationID),
		BatchID:       strings.TrimSpace(opts.BatchID),
		BatchPosition: opts.BatchPosition,
	}
	if err := c.tracker.Bind(ctx, operationKey, result); err != nil {
		return result, err
	}
	return result, nil
}

func newOpaqueKey(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}
