package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type DeleteRecordConfig struct {
	Indexer *indexing.Indexer
}

type DeleteRecord struct {
	indexer *indexing.Indexer
}

var _ gcommand.Commander[types.DeleteRecordInput] = (*DeleteRecord)(nil)

func NewDeleteRecord(cfg DeleteRecordConfig) (*DeleteRecord, error) {
	if cfg.Indexer == nil {
		return nil, errs.ConfigurationError("indexer is required", nil)
	}
	return &DeleteRecord{indexer: cfg.Indexer}, nil
}

func (c *DeleteRecord) Execute(ctx context.Context, msg types.DeleteRecordInput) error {
	if err := c.indexer.DeleteRecord(ctx, msg.Index, msg.RecordID); err != nil {
		return err
	}
	return nil
}
