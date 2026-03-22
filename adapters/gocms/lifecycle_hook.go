package gocms

import (
	"context"
	"strings"

	cmslifecycle "github.com/goliatone/go-cms/pkg/lifecycle"
	"github.com/goliatone/go-search/internal/observe"
	"github.com/goliatone/go-search/pkg/types"
)

type RecordMutator interface {
	IndexRecord(ctx context.Context, index, registrationKey, recordID string) ([]types.Document, error)
	DeleteRecord(ctx context.Context, index, registrationKey, recordID string) error
}

type MediaParentSyncFunc func(ctx context.Context, event cmslifecycle.Event) error

type Route struct {
	ResourceType    string
	ContentTypeSlug string
	SearchIndex     string
	Index           string
	RegistrationKey string
}

type LifecycleHookConfig struct {
	Indexer         RecordMutator
	Routes          []Route
	MediaParentSync MediaParentSyncFunc
	Logger          types.Logger
	Metrics         []types.MetricsHook
}

type LifecycleHook struct {
	indexer         RecordMutator
	routes          []Route
	mediaParentSync MediaParentSyncFunc
	logger          types.Logger
	metrics         []types.MetricsHook
}

func NewLifecycleHook(cfg LifecycleHookConfig) *LifecycleHook {
	routes := make([]Route, 0, len(cfg.Routes))
	for _, route := range cfg.Routes {
		if strings.TrimSpace(route.Index) == "" {
			continue
		}
		routes = append(routes, Route{
			ResourceType:    strings.TrimSpace(route.ResourceType),
			ContentTypeSlug: strings.TrimSpace(route.ContentTypeSlug),
			SearchIndex:     strings.TrimSpace(route.SearchIndex),
			Index:           strings.TrimSpace(route.Index),
			RegistrationKey: strings.TrimSpace(route.RegistrationKey),
		})
	}
	return &LifecycleHook{
		indexer:         cfg.Indexer,
		routes:          routes,
		mediaParentSync: cfg.MediaParentSync,
		logger:          cfg.Logger,
		metrics:         cfg.Metrics,
	}
}

func (h *LifecycleHook) Notify(ctx context.Context, event cmslifecycle.Event) error {
	if h == nil || h.indexer == nil {
		return nil
	}
	route, ok := h.routeFor(event)
	if !ok {
		return nil
	}
	recordID := strings.TrimSpace(event.RecordID)
	if recordID == "" {
		return nil
	}
	if shouldDelete(event) {
		observe.Count(ctx, h.metrics, h.logger, "search.adapter.gocms.delete.count", 1, map[string]string{"index": route.Index})
		if err := h.indexer.DeleteRecord(ctx, route.Index, route.RegistrationKey, recordID); err != nil {
			return err
		}
	} else {
		observe.Count(ctx, h.metrics, h.logger, "search.adapter.gocms.index.count", 1, map[string]string{"index": route.Index})
		if _, err := h.indexer.IndexRecord(ctx, route.Index, route.RegistrationKey, recordID); err != nil {
			return err
		}
	}
	if h.mediaParentSync != nil {
		if err := h.mediaParentSync(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (h *LifecycleHook) routeFor(event cmslifecycle.Event) (Route, bool) {
	resourceType := strings.TrimSpace(event.ResourceType)
	contentTypeSlug := strings.TrimSpace(event.ContentTypeSlug)
	searchIndex := strings.TrimSpace(event.SearchIndex)
	for _, route := range h.routes {
		if route.ResourceType != "" && route.ResourceType != resourceType {
			continue
		}
		if route.ContentTypeSlug != "" && route.ContentTypeSlug != contentTypeSlug {
			continue
		}
		if route.SearchIndex != "" && route.SearchIndex != searchIndex {
			continue
		}
		return route, true
	}
	return Route{}, false
}

func shouldDelete(event cmslifecycle.Event) bool {
	switch strings.TrimSpace(event.Transition) {
	case "delete", "unpublish", "translation_delete":
		return true
	}
	return !event.SearchEnabled
}
