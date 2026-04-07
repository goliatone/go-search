package bunstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/goliatone/go-search/jobs"
	"github.com/uptrace/bun"
)

type Store struct {
	db *bun.DB
}

func New(db *bun.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Upsert(ctx context.Context, snapshot jobs.DispatchSnapshot) error {
	model, err := toModel(snapshot)
	if err != nil {
		return err
	}
	_, err = s.db.NewInsert().
		Model(&model).
		On("CONFLICT (dispatch_id) DO UPDATE").
		Set("operation_key = EXCLUDED.operation_key").
		Set("batch_id = EXCLUDED.batch_id").
		Set("batch_position = EXCLUDED.batch_position").
		Set("state = EXCLUDED.state").
		Set("updated_at = EXCLUDED.updated_at").
		Set("snapshot = EXCLUDED.snapshot").
		Exec(ctx)
	return err
}

func (s *Store) Get(ctx context.Context, dispatchID string) (jobs.DispatchSnapshot, bool, error) {
	var model DispatchModel
	err := s.db.NewSelect().
		Model(&model).
		Where("dispatch_id = ?", dispatchID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return jobs.DispatchSnapshot{}, false, nil
		}
		return jobs.DispatchSnapshot{}, false, err
	}
	snapshot, err := toSnapshot(model)
	if err != nil {
		return jobs.DispatchSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (s *Store) GetByOperationKey(ctx context.Context, operationKey string) (jobs.DispatchSnapshot, bool, error) {
	var model DispatchModel
	err := s.db.NewSelect().
		Model(&model).
		Where("operation_key = ?", operationKey).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return jobs.DispatchSnapshot{}, false, nil
		}
		return jobs.DispatchSnapshot{}, false, err
	}
	snapshot, err := toSnapshot(model)
	if err != nil {
		return jobs.DispatchSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (s *Store) ListBatch(ctx context.Context, batchID string) ([]jobs.DispatchSnapshot, error) {
	models := []DispatchModel{}
	if err := s.db.NewSelect().
		Model(&models).
		Where("batch_id = ?", batchID).
		OrderExpr("batch_position ASC, dispatch_id ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]jobs.DispatchSnapshot, 0, len(models))
	for _, model := range models {
		snapshot, err := toSnapshot(model)
		if err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, nil
}

func toModel(snapshot jobs.DispatchSnapshot) (DispatchModel, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return DispatchModel{}, err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return DispatchModel{}, err
	}
	return DispatchModel{
		DispatchID:    snapshot.DispatchID,
		OperationKey:  snapshot.OperationKey,
		BatchID:       snapshot.BatchID,
		BatchPosition: snapshot.BatchPosition,
		State:         snapshot.State,
		UpdatedAt: func() time.Time {
			if snapshot.UpdatedAt != nil {
				return snapshot.UpdatedAt.UTC()
			}
			return time.Now().UTC()
		}(),
		Snapshot: body,
	}, nil
}

func toSnapshot(model DispatchModel) (jobs.DispatchSnapshot, error) {
	raw, err := json.Marshal(model.Snapshot)
	if err != nil {
		return jobs.DispatchSnapshot{}, err
	}
	var snapshot jobs.DispatchSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return jobs.DispatchSnapshot{}, err
	}
	return snapshot, nil
}
