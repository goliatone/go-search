package postgres

import (
	"strings"
	"time"

	"github.com/goliatone/go-search/pkg/types"
	"github.com/uptrace/bun"
)

type documentModel struct {
	bun.BaseModel `bun:"table:search_documents,alias:sd"`

	IndexName        string              `bun:"index_name,pk"`
	RegistrationKey  string              `bun:"registration_key,pk"`
	DocumentID       string              `bun:"document_id,pk"`
	DocumentType     string              `bun:"document_type,notnull"`
	ParentID         string              `bun:"parent_id,nullzero"`
	SourceType       string              `bun:"source_type,nullzero"`
	SourceID         string              `bun:"source_id,nullzero"`
	SourceVersion    string              `bun:"source_version,nullzero"`
	SearchConfig     string              `bun:"search_config,notnull"`
	Title            string              `bun:"title,nullzero"`
	Summary          string              `bun:"summary,nullzero"`
	Body             string              `bun:"body,nullzero"`
	URL              string              `bun:"url,nullzero"`
	AnchorURL        string              `bun:"anchor_url,nullzero"`
	Locale           string              `bun:"locale,nullzero"`
	Score            float64             `bun:"score"`
	CreatedAtUnix    *int64              `bun:"created_at_unix,nullzero"`
	UpdatedAtUnix    *int64              `bun:"updated_at_unix,nullzero"`
	PublishedAtUnix  *int64              `bun:"published_at_unix,nullzero"`
	StartMS          *int64              `bun:"start_ms,nullzero"`
	EndMS            *int64              `bun:"end_ms,nullzero"`
	Fields           map[string]any      `bun:"fields,type:jsonb"`
	Facets           map[string][]string `bun:"facets,type:jsonb"`
	Numeric          map[string]float64  `bun:"numeric,type:jsonb"`
	Booleans         map[string]bool     `bun:"booleans,type:jsonb"`
	ScopeTenantID    string              `bun:"scope_tenant_id,nullzero"`
	ScopeOrgID       string              `bun:"scope_org_id,nullzero"`
	ScopeLabels      map[string]string   `bun:"scope_labels,type:jsonb"`
	VisibilityPublic bool                `bun:"visibility_public"`
	VisibilityRoles  []string            `bun:"visibility_roles,array"`
	VisibilityPerms  []string            `bun:"visibility_permissions,array"`
	VisibilityStatus string              `bun:"visibility_status,nullzero"`
	Metadata         map[string]any      `bun:"metadata,type:jsonb"`

	SearchableText string  `bun:"searchable_text,scanonly"`
	SearchVector   string  `bun:"search_vector,scanonly"`
	SearchRank     float64 `bun:"search_rank,scanonly"`
	TrigramScore   float64 `bun:"trigram_score,scanonly"`
	CombinedScore  float64 `bun:"combined_score,scanonly"`
}

func toModel(index string, doc types.Document, defaultSearchConfig string) documentModel {
	model := documentModel{
		IndexName:        index,
		RegistrationKey:  strings.TrimSpace(doc.RegistrationKey),
		DocumentID:       doc.ID,
		DocumentType:     doc.Type,
		ParentID:         doc.ParentID,
		SourceType:       doc.SourceType,
		SourceID:         doc.SourceID,
		SourceVersion:    doc.SourceVersion,
		SearchConfig:     resolveDocumentSearchConfig(doc, defaultSearchConfig),
		Title:            doc.Title,
		Summary:          doc.Summary,
		Body:             doc.Body,
		URL:              doc.URL,
		AnchorURL:        doc.AnchorURL,
		Locale:           doc.Locale,
		Score:            doc.Score,
		StartMS:          doc.StartMS,
		EndMS:            doc.EndMS,
		Fields:           doc.Clone().Fields,
		Facets:           doc.Clone().Facets,
		Numeric:          doc.Clone().Numeric,
		Booleans:         doc.Clone().Booleans,
		ScopeTenantID:    doc.Scope.TenantID,
		ScopeOrgID:       doc.Scope.OrgID,
		ScopeLabels:      doc.Scope.Clone().Labels,
		VisibilityPublic: doc.Visibility.Public,
		VisibilityRoles:  normalizeStringSlice(doc.Visibility.Roles),
		VisibilityPerms:  normalizeStringSlice(doc.Visibility.Permissions),
		VisibilityStatus: doc.Visibility.Status,
		Metadata:         doc.Clone().Metadata,
	}
	if model.Fields == nil {
		model.Fields = map[string]any{}
	}
	if model.Facets == nil {
		model.Facets = map[string][]string{}
	}
	if model.Numeric == nil {
		model.Numeric = map[string]float64{}
	}
	if model.Booleans == nil {
		model.Booleans = map[string]bool{}
	}
	if model.ScopeLabels == nil {
		model.ScopeLabels = map[string]string{}
	}
	if model.Metadata == nil {
		model.Metadata = map[string]any{}
	}
	if doc.CreatedAt != nil {
		v := doc.CreatedAt.Unix()
		model.CreatedAtUnix = &v
	}
	if doc.UpdatedAt != nil {
		v := doc.UpdatedAt.Unix()
		model.UpdatedAtUnix = &v
	}
	if doc.PublishedAt != nil {
		v := doc.PublishedAt.Unix()
		model.PublishedAtUnix = &v
	}
	return model
}

func (m documentModel) toDocument() types.Document {
	doc := types.Document{
		ID:              m.DocumentID,
		Index:           m.IndexName,
		RegistrationKey: m.RegistrationKey,
		Type:            m.DocumentType,
		ParentID:        m.ParentID,
		SourceType:      m.SourceType,
		SourceID:        m.SourceID,
		SourceVersion:   m.SourceVersion,
		Title:           m.Title,
		Summary:         m.Summary,
		Body:            m.Body,
		URL:             m.URL,
		AnchorURL:       m.AnchorURL,
		Locale:          m.Locale,
		Score:           m.Score,
		StartMS:         m.StartMS,
		EndMS:           m.EndMS,
		Fields:          m.Fields,
		Facets:          m.Facets,
		Numeric:         m.Numeric,
		Booleans:        m.Booleans,
		Scope: types.Scope{
			TenantID: m.ScopeTenantID,
			OrgID:    m.ScopeOrgID,
			Labels:   m.ScopeLabels,
		},
		Visibility: types.Visibility{
			Public:      m.VisibilityPublic,
			Roles:       append([]string(nil), m.VisibilityRoles...),
			Permissions: append([]string(nil), m.VisibilityPerms...),
			Status:      m.VisibilityStatus,
		},
		Metadata: m.Metadata,
	}
	if m.CreatedAtUnix != nil {
		t := time.Unix(*m.CreatedAtUnix, 0).UTC()
		doc.CreatedAt = &t
	}
	if m.UpdatedAtUnix != nil {
		t := time.Unix(*m.UpdatedAtUnix, 0).UTC()
		doc.UpdatedAt = &t
	}
	if m.PublishedAtUnix != nil {
		t := time.Unix(*m.PublishedAtUnix, 0).UTC()
		doc.PublishedAt = &t
	}
	return doc
}

func resolveDocumentSearchConfig(doc types.Document, fallback string) string {
	for _, key := range []string{"search_config", "locale_analyzer"} {
		if value, ok := doc.Metadata[key]; ok {
			if out := normalizeSearchConfig(toString(value), fallback); out != "" {
				return out
			}
		}
	}
	return normalizeSearchConfig("", fallback)
}

func normalizeSearchConfig(candidate, fallback string) string {
	candidate = strings.TrimSpace(strings.ToLower(candidate))
	if isSafeSearchConfig(candidate) {
		return candidate
	}
	fallback = strings.TrimSpace(strings.ToLower(fallback))
	if isSafeSearchConfig(fallback) {
		return fallback
	}
	return "simple"
}

func normalizeStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func isSafeSearchConfig(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}
