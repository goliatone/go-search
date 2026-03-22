package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type ReindexIndexConfig struct {
	Indexer *indexing.Indexer
}

type ReindexIndex struct {
	indexer *indexing.Indexer
}

var _ gcommand.Commander[types.ReindexIndexInput] = (*ReindexIndex)(nil)

func NewReindexIndex(cfg ReindexIndexConfig) (*ReindexIndex, error) {
	if cfg.Indexer == nil {
		return nil, errs.ConfigurationError("indexer is required", nil)
	}
	return &ReindexIndex{indexer: cfg.Indexer}, nil
}

func (c *ReindexIndex) Execute(ctx context.Context, msg types.ReindexIndexInput) error {
	if err := c.indexer.ReindexIndex(ctx, msg.Index, msg.RegistrationKey, msg.BatchSize); err != nil {
		return err
	}
	return nil
}
