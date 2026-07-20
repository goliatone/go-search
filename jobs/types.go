package jobs

import (
	"time"

	job "github.com/goliatone/go-job"
	qidempotency "github.com/goliatone/go-job/queue/idempotency"
	"github.com/goliatone/go-search/pkg/types"
)

type Operation string

const (
	OperationIndexRecord  Operation = "index_record"
	OperationDeleteRecord Operation = "delete_record"
	OperationReindexIndex Operation = "reindex_index"
)

type JobConfigs struct {
	IndexRecord  job.Config
	DeleteRecord job.Config
	ReindexIndex job.Config
}

type DispatchOptions struct {
	OperationKey     string
	Delay            time.Duration
	RunAt            *time.Time
	IdempotencyKey   string
	DedupPolicy      job.DeduplicationPolicy
	IdempotencyStore qidempotency.Store
	IdempotencyTTL   time.Duration
	CorrelationID    string
	Metadata         map[string]any
	BatchID          string
	BatchPosition    int
}

type DispatchReceipt struct {
	DispatchID    string    `json:"dispatch_id"`
	EnqueuedAt    time.Time `json:"enqueued_at"`
	CommandID     string    `json:"command_id"`
	Operation     Operation `json:"operation"`
	OperationKey  string    `json:"operation_key"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	BatchID       string    `json:"batch_id,omitempty"`
	BatchPosition int       `json:"batch_position,omitempty"`
}

type DispatchProgress struct {
	Current *types.ProgressUpdate  `json:"current,omitempty"`
	History []types.ProgressUpdate `json:"history,omitempty"`
}

type DispatchSummary struct {
	Index           string                `json:"index,omitempty"`
	RegistrationKey string                `json:"registration_key,omitempty"`
	RecordID        string                `json:"record_id,omitempty"`
	BatchSize       int                   `json:"batch_size,omitempty"`
	Generation      *int64                `json:"generation,omitempty"`
	Documents       int                   `json:"documents,omitempty"`
	Completed       int                   `json:"completed,omitempty"`
	Total           int                   `json:"total,omitempty"`
	Activities      []types.ActivityEvent `json:"activities,omitempty"`
}

type DispatchSnapshot struct {
	Revision        int64            `json:"revision"`
	DispatchID      string           `json:"dispatch_id"`
	CommandID       string           `json:"command_id"`
	Operation       Operation        `json:"operation"`
	OperationKey    string           `json:"operation_key"`
	State           string           `json:"state"`
	Attempt         int              `json:"attempt"`
	EnqueuedAt      *time.Time       `json:"enqueued_at,omitempty"`
	UpdatedAt       *time.Time       `json:"updated_at,omitempty"`
	NextRunAt       *time.Time       `json:"next_run_at,omitempty"`
	CorrelationID   string           `json:"correlation_id,omitempty"`
	BatchID         string           `json:"batch_id,omitempty"`
	BatchPosition   int              `json:"batch_position,omitempty"`
	Payload         map[string]any   `json:"payload,omitempty"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
	LastError       string           `json:"last_error,omitempty"`
	TerminalReason  string           `json:"terminal_reason,omitempty"`
	CancelRequested bool             `json:"cancel_requested"`
	Progress        DispatchProgress `json:"progress"`
	Summary         DispatchSummary  `json:"summary"`
}

type DispatchRequest struct {
	Operation    Operation
	IndexRecord  *types.IndexRecordInput
	DeleteRecord *types.DeleteRecordInput
	ReindexIndex *types.ReindexIndexInput
	Options      DispatchOptions
}

type BatchRequest struct {
	BatchID string
	Items   []DispatchRequest
}

type BatchDispatchError struct {
	Position int   `json:"position"`
	Err      error `json:"-"`
}

type BatchReceipt struct {
	BatchID   string               `json:"batch_id"`
	Receipts  []DispatchReceipt    `json:"receipts"`
	Failures  []BatchDispatchError `json:"-"`
	Completed bool                 `json:"completed"`
}

type CancelRequest struct {
	DispatchID string `json:"dispatch_id"`
	Reason     string `json:"reason,omitempty"`
}

type RestartOptions struct {
	DispatchOptions
}

type operationDispatchMetadata struct {
	Operation     Operation `json:"operation"`
	OperationKey  string    `json:"operation_key"`
	BatchID       string    `json:"batch_id,omitempty"`
	BatchPosition int       `json:"batch_position,omitempty"`
}

func (cfg JobConfigs) normalized() JobConfigs {
	if cfg.IndexRecord.Timeout <= 0 {
		cfg.IndexRecord.Timeout = 30 * time.Second
	}
	if cfg.DeleteRecord.Timeout <= 0 {
		cfg.DeleteRecord.Timeout = 30 * time.Second
	}
	if cfg.ReindexIndex.Timeout <= 0 {
		cfg.ReindexIndex.Timeout = 10 * time.Minute
	}
	if cfg.IndexRecord.Metadata == nil {
		cfg.IndexRecord.Metadata = map[string]any{}
	}
	if cfg.DeleteRecord.Metadata == nil {
		cfg.DeleteRecord.Metadata = map[string]any{}
	}
	if cfg.ReindexIndex.Metadata == nil {
		cfg.ReindexIndex.Metadata = map[string]any{}
	}
	cfg.IndexRecord.Metadata["search_operation"] = OperationIndexRecord
	cfg.DeleteRecord.Metadata["search_operation"] = OperationDeleteRecord
	cfg.ReindexIndex.Metadata["search_operation"] = OperationReindexIndex
	return cfg
}
