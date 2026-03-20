package command

import (
	"context"

	gcommand "github.com/goliatone/go-command"
	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
)

type IndexRecordConfig struct {
	Indexer *indexing.Indexer
}

type IndexRecord struct {
	indexer *indexing.Indexer
}

var _ gcommand.Commander[types.IndexRecordInput] = (*IndexRecord)(nil)

func NewIndexRecord(cfg IndexRecordConfig) (*IndexRecord, error) {
	if cfg.Indexer == nil {
		return nil, errs.ConfigurationError("indexer is required", nil)
	}
	return &IndexRecord{indexer: cfg.Indexer}, nil
}

func (c *IndexRecord) Execute(ctx context.Context, msg types.IndexRecordInput) error {
	if _, err := c.indexer.IndexRecord(ctx, msg.Index, msg.RecordID); err != nil {
		return err
	}
	return nil
}
