package gocms

import (
	"context"
	"fmt"
	"strings"

	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	"github.com/google/uuid"
)

type ContentSourceConfig struct {
	Service          cmscontent.Service
	EnvironmentKey   string
	ContentTypeSlugs []string
}

type ContentSource struct {
	service          cmscontent.Service
	environmentKey   string
	contentTypeSlugs map[string]struct{}
}

func NewContentSource(cfg ContentSourceConfig) *ContentSource {
	return &ContentSource{
		service:          cfg.Service,
		environmentKey:   cfg.EnvironmentKey,
		contentTypeSlugs: normalizedSlugSet(cfg.ContentTypeSlugs),
	}
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
	record, err := s.service.Get(ctx, uid, opts...)
	if err != nil {
		return nil, err
	}
	if !s.matchesContentTypeSlug(contentTypeSlug(record)) {
		return nil, fmt.Errorf("content source %q does not match configured content type routes", id)
	}
	return record, nil
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
	items = filterContentItems(items, s.matchesContentTypeSlug)
	return sliceWithCursor(items, limit, cursor, func(item *cmscontent.Content) string {
		if item == nil {
			return ""
		}
		return item.ID.String()
	})
}

type PageSourceConfig struct {
	Service          cmspages.Service
	EnvironmentKey   string
	ContentTypeSlugs []string
}

type PageSource struct {
	service          cmspages.Service
	environmentKey   string
	contentTypeSlugs map[string]struct{}
}

func NewPageSource(cfg PageSourceConfig) *PageSource {
	return &PageSource{
		service:          cfg.Service,
		environmentKey:   cfg.EnvironmentKey,
		contentTypeSlugs: normalizedSlugSet(cfg.ContentTypeSlugs),
	}
}

func (s *PageSource) Get(ctx context.Context, id string) (*cmspages.Page, error) {
	if s == nil || s.service == nil {
		return nil, fmt.Errorf("page source unavailable")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	record, err := s.service.Get(ctx, uid)
	if err != nil {
		return nil, err
	}
	if !s.matchesContentTypeSlug(pageContentTypeSlug(record)) {
		return nil, fmt.Errorf("page source %q does not match configured content type routes", id)
	}
	return record, nil
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
	items = filterPageItems(items, s.matchesContentTypeSlug)
	return sliceWithCursor(items, limit, cursor, func(item *cmspages.Page) string {
		if item == nil {
			return ""
		}
		return item.ID.String()
	})
}

func (s *ContentSource) matchesContentTypeSlug(slug string) bool {
	return matchesSlug(s.contentTypeSlugs, slug)
}

func (s *PageSource) matchesContentTypeSlug(slug string) bool {
	return matchesSlug(s.contentTypeSlugs, slug)
}

func matchesSlug(allowed map[string]struct{}, slug string) bool {
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[strings.ToLower(strings.TrimSpace(slug))]
	return ok
}

func normalizedSlugSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out[value] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func contentTypeSlug(record *cmscontent.Content) string {
	if record == nil || record.Type == nil {
		return ""
	}
	return record.Type.Slug
}

func pageContentTypeSlug(record *cmspages.Page) string {
	if record == nil || record.Content == nil || record.Content.Type == nil {
		return ""
	}
	return record.Content.Type.Slug
}

func filterContentItems(items []*cmscontent.Content, keep func(string) bool) []*cmscontent.Content {
	if len(items) == 0 {
		return nil
	}
	out := make([]*cmscontent.Content, 0, len(items))
	for _, item := range items {
		if keep(contentTypeSlug(item)) {
			out = append(out, item)
		}
	}
	return out
}

func filterPageItems(items []*cmspages.Page, keep func(string) bool) []*cmspages.Page {
	if len(items) == 0 {
		return nil
	}
	out := make([]*cmspages.Page, 0, len(items))
	for _, item := range items {
		if keep(pageContentTypeSlug(item)) {
			out = append(out, item)
		}
	}
	return out
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
