package gocms

import (
	"context"
	"fmt"

	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	"github.com/google/uuid"
)

type ContentSourceConfig struct {
	Service        cmscontent.Service
	EnvironmentKey string
}

type ContentSource struct {
	service        cmscontent.Service
	environmentKey string
}

func NewContentSource(cfg ContentSourceConfig) *ContentSource {
	return &ContentSource{service: cfg.Service, environmentKey: cfg.EnvironmentKey}
}

func (s *ContentSource) Get(ctx context.Context, id string) (*cmscontent.Content, error) {
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("content source unavailable")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	opts := []cmscontent.ContentGetOption{cmscontent.WithTranslations(), cmscontent.WithDerivedFields()}
	return s.service.Get(ctx, uid, opts...)
}

func (s *ContentSource) List(ctx context.Context, limit int, cursor string) ([]*cmscontent.Content, string, error) {
	if s == nil || s.service == nil {
		return nil, "", fmt.Errorf("content source unavailable")
	}
	opts := []cmscontent.ContentListOption{cmscontent.WithTranslations(), cmscontent.WithDerivedFields()}
	if s.environmentKey != "" {
		opts = append(opts, cmscontent.ContentListOption(s.environmentKey))
	}
	items, err := s.service.List(ctx, opts...)
	if err != nil {
		return nil, "", err
	}
	return sliceWithCursor(items, limit, cursor, func(item *cmscontent.Content) string {
		if item == nil {
			return ""
		}
		return item.ID.String()
	})
}

type PageSourceConfig struct {
	Service        cmspages.Service
	EnvironmentKey string
}

type PageSource struct {
	service        cmspages.Service
	environmentKey string
}

func NewPageSource(cfg PageSourceConfig) *PageSource {
	return &PageSource{service: cfg.Service, environmentKey: cfg.EnvironmentKey}
}

func (s *PageSource) Get(ctx context.Context, id string) (*cmspages.Page, error) {
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("page source unavailable")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return s.service.Get(ctx, uid)
}

func (s *PageSource) List(ctx context.Context, limit int, cursor string) ([]*cmspages.Page, string, error) {
	if s == nil || s.service == nil {
		return nil, "", fmt.Errorf("page source unavailable")
	}
	var (
		items []*cmspages.Page
		err   error
	)
	if s.environmentKey != "" {
		items, err = s.service.List(ctx, s.environmentKey)
	} else {
		items, err = s.service.List(ctx)
	}
	if err != nil {
		return nil, "", err
	}
	return sliceWithCursor(items, limit, cursor, func(item *cmspages.Page) string {
		if item == nil {
			return ""
		}
		return item.ID.String()
	})
}

func sliceWithCursor[T any](items []T, limit int, cursor string, idGetter func(T) string) ([]T, string, error) {
	start := 0
	if cursor != "" {
		for i, item := range items {
			if idGetter(item) == cursor {
				start = i + 1
				break
			}
		}
	}
	if start >= len(items) {
		return nil, "", nil
	}
	end := len(items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	out := make([]T, 0, end-start)
	for _, item := range items[start:end] {
		out = append(out, item)
	}
	next := ""
	if end < len(items) {
		next = idGetter(items[end-1])
	}
	return out, next, nil
}
