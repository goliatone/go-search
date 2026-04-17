package content

import (
	"context"
	"maps"
	"strings"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

type ProjectorConfig struct {
	Index      string
	SourceType string
}

type Projector struct {
	cfg ProjectorConfig
}

func NewProjector(cfg ProjectorConfig) *Projector {
	return &Projector{cfg: cfg}
}

func (p *Projector) Project(_ context.Context, record Record) ([]types.Document, error) {
	doc := types.Document{
		ID:            strings.TrimSpace(record.ID),
		Index:         strings.TrimSpace(p.cfg.Index),
		Type:          firstNonEmpty(strings.TrimSpace(record.Type), types.DocumentTypeDocument),
		SourceType:    firstNonEmpty(strings.TrimSpace(record.SourceType), strings.TrimSpace(p.cfg.SourceType)),
		SourceID:      firstNonEmpty(strings.TrimSpace(record.SourceID), strings.TrimSpace(record.ID)),
		SourceVersion: strings.TrimSpace(record.SourceVersion),
		Title:         strings.TrimSpace(record.Title),
		Summary:       strings.TrimSpace(record.Summary),
		Body:          strings.TrimSpace(record.Body),
		URL:           strings.TrimSpace(record.URL),
		Locale:        strings.TrimSpace(record.Locale),
		Fields:        cloneMap(record.Fields),
		Facets:        cloneFacetMap(record.Facets),
		Numeric:       cloneMap(record.Numeric),
		Booleans:      cloneMap(record.Booleans),
		Metadata:      cloneMap(record.Metadata),
	}
	if doc.Fields == nil {
		doc.Fields = map[string]any{}
	}
	if doc.Facets == nil {
		doc.Facets = map[string][]string{}
	}
	if doc.Numeric == nil {
		doc.Numeric = map[string]float64{}
	}
	doc.Fields["entity_type"] = doc.Type
	doc.Facets["entity_type"] = []string{doc.Type}
	if _, ok := doc.Numeric["published_year"]; !ok {
		if year, ok := publishedYear(record.Metadata); ok {
			doc.Numeric["published_year"] = float64(year)
			doc.Fields["published_year"] = year
		}
	}
	return []types.Document{doc}, nil
}

func publishedYear(metadata map[string]any) (int, bool) {
	if metadata == nil {
		return 0, false
	}
	for _, key := range []string{"published_year", "published_at"} {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case int:
			return value, true
		case int64:
			return int(value), true
		case float64:
			return int(value), true
		case time.Time:
			return value.UTC().Year(), true
		case *time.Time:
			if value != nil {
				return value.UTC().Year(), true
			}
		}
	}
	return 0, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cloneMap[T any](in map[string]T) map[string]T {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]T, len(in))
	maps.Copy(out, in)
	return out
}

func cloneFacetMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func cloneRecord(in Record) Record {
	out := in
	out.Fields = cloneMap(in.Fields)
	out.Facets = cloneFacetMap(in.Facets)
	out.Numeric = cloneMap(in.Numeric)
	out.Booleans = cloneMap(in.Booleans)
	out.Metadata = cloneMap(in.Metadata)
	return out
}
