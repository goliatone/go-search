package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type ReindexIndexConfig struct {
	Indexer         *indexing.Indexer
	GenerationStore types.GenerationStore
}

type ReindexIndex struct {
	indexer         *indexing.Indexer
	generationStore types.GenerationStore
}

var _ gcommand.Commander[types.ReindexIndexInput] = (*ReindexIndex)(nil)

func NewReindexIndex(cfg ReindexIndexConfig) (*ReindexIndex, error) {
	if cfg.Indexer == nil {
		return nil, errs.ConfigurationError("indexer is required", nil)
	}
	return &ReindexIndex{indexer: cfg.Indexer, generationStore: cfg.GenerationStore}, nil
}

func (c *ReindexIndex) Execute(ctx context.Context, msg types.ReindexIndexInput) error {
	if err := c.indexer.ReindexIndex(ctx, msg.Index, msg.BatchSize); err != nil {
		return err
	}
	bumpGeneration(ctx, c.generationStore, msg.Index)
	return nil
}
