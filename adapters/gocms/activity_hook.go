package gocms

import (
	"context"
	"fmt"
	"strings"

	cmsactivity "github.com/goliatone/go-cms/pkg/activity"
	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
)

type RecordMutator interface {
	IndexRecord(ctx context.Context, index string, recordID string) ([]types.Document, error)
	DeleteRecord(ctx context.Context, index string, recordID string) error
}

type ActivityHookConfig struct {
	Indexer      RecordMutator
	ContentIndex string
	PageIndex    string
	Logger       types.Logger
	Metrics      []types.MetricsHook
}

type ActivityHook struct {
	indexer      RecordMutator
	contentIndex string
	pageIndex    string
	logger       types.Logger
	metrics      []types.MetricsHook
}

func NewActivityHook(cfg ActivityHookConfig) *ActivityHook {
	return &ActivityHook{
		indexer:      cfg.Indexer,
		contentIndex: strings.TrimSpace(cfg.ContentIndex),
		pageIndex:    strings.TrimSpace(cfg.PageIndex),
		logger:       cfg.Logger,
		metrics:      cfg.Metrics,
	}
}

func (h *ActivityHook) Notify(ctx context.Context, event cmsactivity.Event) error {
	if h == nil || h.indexer == nil {
		return nil
	}
	index := h.indexFor(event.ObjectType)
	recordID := strings.TrimSpace(event.ObjectID)
	if index == "" || recordID == "" {
		return nil
	}
	switch actionFor(event) {
	case "delete":
		observe.Count(ctx, h.metrics, h.logger, "search.adapter.gocms.delete.count", 1, map[string]string{"index": index})
		return h.indexer.DeleteRecord(ctx, index, recordID)
	case "index":
		if !shouldIndexEvent(event) {
			observe.Count(ctx, h.metrics, h.logger, "search.adapter.gocms.delete.count", 1, map[string]string{"index": index})
			return h.indexer.DeleteRecord(ctx, index, recordID)
		}
		observe.Count(ctx, h.metrics, h.logger, "search.adapter.gocms.index.count", 1, map[string]string{"index": index})
		_, err := h.indexer.IndexRecord(ctx, index, recordID)
		return err
	default:
		return nil
	}
}

func (h *ActivityHook) indexFor(objectType string) string {
	switch strings.TrimSpace(objectType) {
	case "content", "content_translation":
		return h.contentIndex
	case "page", "page_translation":
		return h.pageIndex
	default:
		return ""
	}
}

func actionFor(event cmsactivity.Event) string {
	switch strings.TrimSpace(event.Verb) {
	case "delete", "unpublish":
		return "delete"
	case "create", "update", "publish", "schedule":
		return "index"
	default:
		if strings.HasPrefix(strings.TrimSpace(event.ObjectType), "content_") || strings.HasPrefix(strings.TrimSpace(event.ObjectType), "page_") {
			return "index"
		}
		return ""
	}
}

func shouldIndexEvent(event cmsactivity.Event) bool {
	status := strings.TrimSpace(asString(event.Metadata["status"]))
	if status != "" && !strings.EqualFold(status, "published") {
		return false
	}
	if raw, ok := event.Metadata["search_enabled"]; ok {
		if enabled, ok := raw.(bool); ok {
			return enabled
		}
	}
	return true
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch raw := value.(type) {
	case string:
		return raw
	default:
		return fmt.Sprint(raw)
	}
}
