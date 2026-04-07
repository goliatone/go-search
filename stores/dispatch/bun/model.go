package bunstore

import (
	"time"

	"github.com/uptrace/bun"
)

type DispatchModel struct {
	bun.BaseModel `bun:"table:search_job_dispatches,alias:sjd"`
	DispatchID    string         `bun:"dispatch_id,pk"`
	OperationKey  string         `bun:"operation_key,unique,notnull"`
	BatchID       string         `bun:"batch_id,nullzero"`
	BatchPosition int            `bun:"batch_position,notnull"`
	State         string         `bun:"state,notnull"`
	UpdatedAt     time.Time      `bun:"updated_at,notnull"`
	Snapshot      map[string]any `bun:"snapshot,type:jsonb,notnull"`
}
