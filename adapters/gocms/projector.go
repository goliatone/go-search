package gocms

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	contentadapter "github.com/goliatone/go-search/adapters/content"
	searchlocale "github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

const DocumentTypePage = "page"

type ProjectorConfig struct {
	Index           string
	RegistrationKey string
	SourceType      string
	DocumentType    string
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
		records = append(records, contentadapter.Record{
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
		})
	}
	return projectRecords(ctx, p.delegate, records)
}

func projectContent(ctx context.Context, delegate *contentadapter.Projector, cfg ProjectorConfig, record *cmscontent.Content) ([]types.Document, error) {
	if record == nil || !strings.EqualFold(strings.TrimSpace(record.Status), "published") {
		return nil, nil
	}
	searchEnabled, _ := contentSearchState(record)
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
		records = append(records, contentadapter.Record{
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
		})
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
