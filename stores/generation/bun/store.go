package bunstore

import (
	"context"
	"database/sql"

	repository "github.com/goliatone/go-repository-bun"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/uptrace/bun"
)

type Store struct {
	db    *bun.DB
	clock types.Clock
}

var _ repository.DBProvider = (*Store)(nil)

type Config struct {
	DB    *bun.DB
	Clock types.Clock
}

func New(cfg Config) *Store {
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &Store{db: cfg.DB, clock: cfg.Clock}
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
	model := GenerationModel{
		IndexName:     index,
		Generation:    1,
		LastIndexedAt: s.clock.Now().Unix(),
	}
	err := s.db.NewInsert().
		Model(&model).
		On("CONFLICT (index_name) DO UPDATE").
		Set("generation = search_generations.generation + 1").
		Set("last_indexed_at = EXCLUDED.last_indexed_at").
		Returning("generation").
		Scan(ctx)
	return model.Generation, err
}
