package bunstore

import (
	"context"
	"database/sql"
	"time"

	repository "github.com/goliatone/go-repository-bun"
	"github.com/uptrace/bun"
)

type Store struct {
	db *bun.DB
}

var _ repository.DBProvider = (*Store)(nil)

func New(db *bun.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *bun.DB {
	return s.db
}

func (s *Store) Get(ctx context.Context, index string) (int64, error) {
	model := new(GenerationModel)
	err := s.db.NewSelect().Model(model).Where("index_name = ?", index).Scan(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return model.Generation, nil
}

func (s *Store) Bump(ctx context.Context, index string) (int64, error) {
	current, err := s.Get(ctx, index)
	if err != nil {
		return 0, err
	}
	next := current + 1
	model := GenerationModel{
		IndexName:     index,
		Generation:    next,
		LastIndexedAt: time.Now().Unix(),
	}
	_, err = s.db.NewInsert().
		Model(&model).
		On("CONFLICT (index_name) DO UPDATE").
		Set("generation = EXCLUDED.generation").
		Set("last_indexed_at = EXCLUDED.last_indexed_at").
		Exec(ctx)
	return next, err
}
