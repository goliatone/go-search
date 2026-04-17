package gocms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	contentadapter "github.com/goliatone/go-search/adapters/content"
	searchlocale "github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

const DocumentTypePage = "page"

type ProjectorConfig struct {
	Index            string
	RegistrationKey  string
	SourceType       string
	DocumentType     string
	ContentEnrichers ContentRecordEnrichers
	PageEnrichers    PageRecordEnrichers
}

type ProjectionContext struct {
	Index           string
	RegistrationKey string
	SourceType      string
	DocumentType    string
	Locale          string
	SearchIndex     string
	ContentTypeSlug string
}

type ContentRecordEnricher interface {
	EnrichContentRecord(context.Context, ProjectionContext, *cmscontent.Content, *contentadapter.Record) error
}

type ContentRecordEnricherFunc func(context.Context, ProjectionContext, *cmscontent.Content, *contentadapter.Record) error

func (fn ContentRecordEnricherFunc) EnrichContentRecord(ctx context.Context, meta ProjectionContext, src *cmscontent.Content, rec *contentadapter.Record) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, meta, src, rec)
}

type ContentRecordEnrichers []ContentRecordEnricher

func (h ContentRecordEnrichers) Enabled() bool {
	return len(h) > 0
}

func (h ContentRecordEnrichers) Enrich(ctx context.Context, meta ProjectionContext, src *cmscontent.Content, rec *contentadapter.Record) error {
	if len(h) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	for _, hook := range h {
		if hook == nil {
			continue
		}
		if err := hook.EnrichContentRecord(ctx, meta, src, rec); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

type PageRecordEnricher interface {
	EnrichPageRecord(context.Context, ProjectionContext, *cmspages.Page, *contentadapter.Record) error
}

type PageRecordEnricherFunc func(context.Context, ProjectionContext, *cmspages.Page, *contentadapter.Record) error

func (fn PageRecordEnricherFunc) EnrichPageRecord(ctx context.Context, meta ProjectionContext, src *cmspages.Page, rec *contentadapter.Record) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, meta, src, rec)
}

type PageRecordEnrichers []PageRecordEnricher

func (h PageRecordEnrichers) Enabled() bool {
	return len(h) > 0
}

func (h PageRecordEnrichers) Enrich(ctx context.Context, meta ProjectionContext, src *cmspages.Page, rec *contentadapter.Record) error {
	if len(h) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	for _, hook := range h {
		if hook == nil {
			continue
		}
		if err := hook.EnrichPageRecord(ctx, meta, src, rec); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

type DocumentProjector struct {
	cfg      ProjectorConfig
	delegate *contentadapter.Projector
}

func NewDocumentProjector(cfg ProjectorConfig) *DocumentProjector {
	return newProjector(cfg, types.DocumentTypeDocument)
}

type BlogArticleProjector struct {
	cfg      ProjectorConfig
	delegate *contentadapter.Projector
}

func NewBlogArticleProjector(cfg ProjectorConfig) *BlogArticleProjector {
	normalized := normalizeProjectorConfig(cfg, types.DocumentTypeBlogArticle)
	return &BlogArticleProjector{
		cfg: normalized,
		delegate: contentadapter.NewProjector(contentadapter.ProjectorConfig{
			Index:      normalized.Index,
			SourceType: normalized.SourceType,
		}),
	}
}

type PageProjector struct {
	cfg      ProjectorConfig
	delegate *contentadapter.Projector
}

func NewPageProjector(cfg ProjectorConfig) *PageProjector {
	normalized := normalizeProjectorConfig(cfg, DocumentTypePage)
	return &PageProjector{
		cfg: normalized,
		delegate: contentadapter.NewProjector(contentadapter.ProjectorConfig{
			Index:      normalized.Index,
			SourceType: normalized.SourceType,
		}),
	}
}

func newProjector(cfg ProjectorConfig, documentType string) *DocumentProjector {
	normalized := normalizeProjectorConfig(cfg, documentType)
	return &DocumentProjector{
		cfg: normalized,
		delegate: contentadapter.NewProjector(contentadapter.ProjectorConfig{
			Index:      normalized.Index,
			SourceType: normalized.SourceType,
		}),
	}
}

func normalizeProjectorConfig(cfg ProjectorConfig, documentType string) ProjectorConfig {
	cfg.Index = strings.TrimSpace(cfg.Index)
	cfg.RegistrationKey = strings.TrimSpace(cfg.RegistrationKey)
	cfg.SourceType = strings.TrimSpace(cfg.SourceType)
	if cfg.SourceType == "" {
		cfg.SourceType = cfg.RegistrationKey
	}
	if cfg.SourceType == "" {
		cfg.SourceType = strings.TrimSpace(documentType)
	}
	cfg.DocumentType = strings.TrimSpace(cfg.DocumentType)
	if cfg.DocumentType == "" {
		cfg.DocumentType = documentType
	}
	if len(cfg.ContentEnrichers) > 0 {
		cfg.ContentEnrichers = append(ContentRecordEnrichers(nil), cfg.ContentEnrichers...)
	}
	if len(cfg.PageEnrichers) > 0 {
		cfg.PageEnrichers = append(PageRecordEnrichers(nil), cfg.PageEnrichers...)
	}
	return cfg
}

func (p *DocumentProjector) Project(ctx context.Context, record *cmscontent.Content) ([]types.Document, error) {
	return projectContent(ctx, p.delegate, p.cfg, record)
}

func (p *BlogArticleProjector) Project(ctx context.Context, record *cmscontent.Content) ([]types.Document, error) {
	return projectContent(ctx, p.delegate, p.cfg, record)
}

func (p *PageProjector) Project(ctx context.Context, record *cmspages.Page) ([]types.Document, error) {
	if record == nil || !strings.EqualFold(strings.TrimSpace(record.Status), "published") {
		return nil, nil
	}
	records := make([]contentadapter.Record, 0, len(record.Translations))
	for _, tr := range record.Translations {
		if tr == nil {
			continue
		}
		locale := canonicalLocale(tr.Locale)
		if locale == "" {
			continue
		}
		fields := map[string]any{
			"slug":      record.Slug,
			"status":    record.Status,
			"path":      tr.Path,
			"entity_id": record.ID.String(),
		}
		if record.Content != nil {
			fields["content_id"] = record.ContentID.String()
			fields["content_type_id"] = record.Content.ContentTypeID.String()
			if record.Content.Type != nil {
				fields["content_type_slug"] = record.Content.Type.Slug
			}
		}
		item := contentadapter.Record{
			ID:         deterministicDocumentID(record.ID.String(), p.cfg.RegistrationKey, locale),
			Type:       p.cfg.DocumentType,
			SourceType: p.cfg.SourceType,
			SourceID:   record.ID.String(),
			Title:      strings.TrimSpace(tr.Title),
			Summary:    derefString(tr.Summary),
			Body:       pageBody(record, tr.Locale),
			URL:        strings.TrimSpace(tr.Path),
			Locale:     locale,
			Fields:     fields,
			Facets: map[string][]string{
				"locale": {locale},
			},
			Metadata: map[string]any{
				"page_id": record.ID.String(),
			},
		}
		if record.PublishedAt != nil {
			item.Metadata["published_at"] = record.PublishedAt.UTC()
		}
		enriched, err := applyPageEnrichers(ctx, p.cfg, record, item, locale)
		if err != nil {
			return nil, err
		}
		records = append(records, enriched)
	}
	return projectRecords(ctx, p.delegate, records)
}

func projectContent(ctx context.Context, delegate *contentadapter.Projector, cfg ProjectorConfig, record *cmscontent.Content) ([]types.Document, error) {
	if record == nil || !strings.EqualFold(strings.TrimSpace(record.Status), "published") {
		return nil, nil
	}
	searchEnabled, searchIndex := contentSearchState(record)
	if !searchEnabled {
		return nil, nil
	}
	records := make([]contentadapter.Record, 0, len(record.Translations))
	for _, tr := range record.Translations {
		if tr == nil || tr.Locale == nil {
			continue
		}
		locale := canonicalLocale(tr.Locale.Code)
		if locale == "" {
			continue
		}
		fields := map[string]any{
			"slug":            record.Slug,
			"status":          record.Status,
			"content_type_id": record.ContentTypeID.String(),
		}
		if record.Type != nil {
			fields["content_type_slug"] = record.Type.Slug
		}
		item := contentadapter.Record{
			ID:         deterministicDocumentID(record.ID.String(), cfg.RegistrationKey, locale),
			Type:       cfg.DocumentType,
			SourceType: cfg.SourceType,
			SourceID:   record.ID.String(),
			Title:      strings.TrimSpace(tr.Title),
			Summary:    derefString(tr.Summary),
			Body:       renderBody(tr.Content),
			URL:        contentURL(record),
			Locale:     locale,
			Fields:     fields,
			Facets: map[string][]string{
				"locale": {locale},
			},
			Metadata: map[string]any{
				"content_id": record.ID.String(),
			},
		}
		if record.PublishedAt != nil {
			item.Metadata["published_at"] = record.PublishedAt.UTC()
		}
		enriched, err := applyContentEnrichers(ctx, cfg, record, item, locale, searchIndex)
		if err != nil {
			return nil, err
		}
		records = append(records, enriched)
	}
	return projectRecords(ctx, delegate, records)
}

func projectRecords(ctx context.Context, delegate *contentadapter.Projector, records []contentadapter.Record) ([]types.Document, error) {
	out := make([]types.Document, 0, len(records))
	for _, item := range records {
		docs, err := delegate.Project(ctx, item)
		if err != nil {
			return nil, err
		}
		out = append(out, docs...)
	}
	return out, nil
}

func contentSearchState(record *cmscontent.Content) (bool, string) {
	if record == nil || record.Type == nil {
		return false, ""
	}
	contracts := cmscontent.ParseContentTypeCapabilityContracts(record.Type.Capabilities)
	indexName := strings.TrimSpace(fmt.Sprint(contracts.Search["index"]))
	if raw, ok := contracts.Search["enabled"].(bool); ok {
		return raw, indexName
	}
	return indexName != "" || len(contracts.Search) > 0, indexName
}

func deterministicDocumentID(rootID, registrationKey, locale string) string {
	return strings.TrimSpace(rootID) + ":" + strings.TrimSpace(registrationKey) + ":" + canonicalLocale(locale)
}

func canonicalLocale(locale string) string {
	return searchlocale.Normalize(locale)
}

func contentURL(record *cmscontent.Content) string {
	if record == nil {
		return ""
	}
	if path, ok := record.Metadata["path"].(string); ok && strings.TrimSpace(path) != "" {
		return strings.TrimSpace(path)
	}
	if slug := strings.TrimSpace(record.Slug); slug != "" {
		return "/" + slug
	}
	return ""
}

func pageBody(record *cmspages.Page, locale string) string {
	if record == nil || record.Content == nil {
		return ""
	}
	for _, tr := range record.Content.Translations {
		if tr == nil || tr.Locale == nil {
			continue
		}
		if canonicalLocale(tr.Locale.Code) == canonicalLocale(locale) {
			return renderBody(tr.Content)
		}
	}
	return ""
}

func renderBody(content map[string]any) string {
	if len(content) == 0 {
		return ""
	}
	payload, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(payload)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func applyContentEnrichers(ctx context.Context, cfg ProjectorConfig, src *cmscontent.Content, record contentadapter.Record, locale, searchIndex string) (contentadapter.Record, error) {
	if !cfg.ContentEnrichers.Enabled() {
		return record, nil
	}
	enriched := cloneAdapterRecord(record)
	if err := cfg.ContentEnrichers.Enrich(ctx, buildContentProjectionContext(cfg, src, locale, searchIndex), src, &enriched); err != nil {
		return contentadapter.Record{}, err
	}
	return enriched, nil
}

func applyPageEnrichers(ctx context.Context, cfg ProjectorConfig, src *cmspages.Page, record contentadapter.Record, locale string) (contentadapter.Record, error) {
	if !cfg.PageEnrichers.Enabled() {
		return record, nil
	}
	enriched := cloneAdapterRecord(record)
	if err := cfg.PageEnrichers.Enrich(ctx, buildPageProjectionContext(cfg, src, locale), src, &enriched); err != nil {
		return contentadapter.Record{}, err
	}
	return enriched, nil
}

func buildContentProjectionContext(cfg ProjectorConfig, src *cmscontent.Content, locale, searchIndex string) ProjectionContext {
	ctx := ProjectionContext{
		Index:           cfg.Index,
		RegistrationKey: cfg.RegistrationKey,
		SourceType:      cfg.SourceType,
		DocumentType:    cfg.DocumentType,
		Locale:          canonicalLocale(locale),
		SearchIndex:     strings.TrimSpace(searchIndex),
	}
	if src != nil && src.Type != nil {
		ctx.ContentTypeSlug = strings.TrimSpace(src.Type.Slug)
	}
	return ctx
}

func buildPageProjectionContext(cfg ProjectorConfig, src *cmspages.Page, locale string) ProjectionContext {
	ctx := ProjectionContext{
		Index:           cfg.Index,
		RegistrationKey: cfg.RegistrationKey,
		SourceType:      cfg.SourceType,
		DocumentType:    cfg.DocumentType,
		Locale:          canonicalLocale(locale),
	}
	if src != nil && src.Content != nil && src.Content.Type != nil {
		ctx.ContentTypeSlug = strings.TrimSpace(src.Content.Type.Slug)
	}
	return ctx
}

func cloneAdapterRecord(in contentadapter.Record) contentadapter.Record {
	out := in
	out.Fields = cloneMap(in.Fields)
	out.Facets = cloneFacetMap(in.Facets)
	out.Numeric = cloneMap(in.Numeric)
	out.Booleans = cloneMap(in.Booleans)
	out.Scope = cloneMap(in.Scope)
	out.Metadata = cloneMap(in.Metadata)
	return out
}

func cloneMap[T any](src map[string]T) map[string]T {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]T, len(src))
	maps.Copy(dst, src)
	return dst
}

func cloneFacetMap(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
