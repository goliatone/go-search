package gocms

import (
	"context"
	"errors"
	"testing"
	"time"

	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	contentadapter "github.com/goliatone/go-search/adapters/content"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/google/uuid"
)

func TestDocumentProjectorFansOutLocales(t *testing.T) {
	publishedAt := time.Date(2024, time.May, 1, 8, 0, 0, 0, time.UTC)
	projector := NewDocumentProjector(ProjectorConfig{
		Index:           "documents",
		RegistrationKey: "document",
		SourceType:      "content",
	})
	record := &cmscontent.Content{
		ID:          uuid.New(),
		Slug:        "guide",
		Status:      "published",
		PublishedAt: &publishedAt,
		Type: &cmscontent.ContentType{
			Slug: "article",
			Capabilities: map[string]any{
				"search": map[string]any{"enabled": true, "index": "documents"},
			},
		},
		Translations: []*cmscontent.ContentTranslation{
			{Locale: &cmscontent.Locale{Code: "en"}, Title: "Guide", Content: map[string]any{"body": "Hello"}},
			{Locale: &cmscontent.Locale{Code: "es"}, Title: "Guia", Content: map[string]any{"body": "Hola"}},
		},
	}
	docs, err := projector.Project(context.Background(), record)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	for _, doc := range docs {
		if doc.Type != types.DocumentTypeDocument {
			t.Fatalf("doc type = %q", doc.Type)
		}
		if doc.SourceID != record.ID.String() {
			t.Fatalf("source id = %q", doc.SourceID)
		}
		if year, ok := doc.Fields["published_year"].(int); !ok || year != 2024 {
			t.Fatalf("expected derived published year, got %#v", doc.Fields["published_year"])
		}
	}
}

func TestPageProjectorUsesPageTypeAndLocaleSpecificIDs(t *testing.T) {
	publishedAt := time.Date(2023, time.January, 10, 8, 0, 0, 0, time.UTC)
	projector := NewPageProjector(ProjectorConfig{
		Index:           "content_shared",
		RegistrationKey: "page",
		SourceType:      "page",
	})
	record := &cmspages.Page{
		ID:          uuid.New(),
		Slug:        "home",
		Status:      "published",
		PublishedAt: &publishedAt,
		Content: &cmscontent.Content{
			ID: uuid.New(),
			Type: &cmscontent.ContentType{
				Slug: "landing_page",
				Capabilities: map[string]any{
					"search": map[string]any{"enabled": true},
				},
			},
		},
		Translations: []*cmspages.PageTranslation{
			{ID: uuid.New(), Locale: "en", Title: "Home", Path: "/"},
		},
	}
	docs, err := projector.Project(context.Background(), record)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Type != DocumentTypePage {
		t.Fatalf("doc type = %q", docs[0].Type)
	}
	if docs[0].ID == "" || docs[0].SourceID != record.ID.String() {
		t.Fatalf("unexpected doc identity: %+v", docs[0])
	}
	if year, ok := docs[0].Fields["published_year"].(int); !ok || year != 2023 {
		t.Fatalf("expected derived published year, got %#v", docs[0].Fields["published_year"])
	}
}

func TestDocumentProjectorContentEnrichersRunInOrder(t *testing.T) {
	projector := NewDocumentProjector(ProjectorConfig{
		Index:           "documents",
		RegistrationKey: "document",
		SourceType:      "content",
		ContentEnrichers: ContentRecordEnrichers{
			ContentRecordEnricherFunc(func(_ context.Context, meta ProjectionContext, _ *cmscontent.Content, rec *contentadapter.Record) error {
				rec.Metadata["step1"] = meta.Locale
				rec.Facets["topic"] = []string{"Archive"}
				return nil
			}),
			ContentRecordEnricherFunc(func(_ context.Context, _ ProjectionContext, _ *cmscontent.Content, rec *contentadapter.Record) error {
				rec.Metadata["step2"] = rec.Metadata["step1"]
				rec.Fields["topic"] = rec.Facets["topic"][0]
				return nil
			}),
		},
	})
	record := &cmscontent.Content{
		ID:     uuid.New(),
		Slug:   "guide",
		Status: "published",
		Type: &cmscontent.ContentType{
			Slug: "article",
			Capabilities: map[string]any{
				"search": map[string]any{"enabled": true, "index": "documents"},
			},
		},
		Translations: []*cmscontent.ContentTranslation{
			{Locale: &cmscontent.Locale{Code: "en"}, Title: "Guide", Content: map[string]any{"body": "Hello"}},
		},
	}
	docs, err := projector.Project(context.Background(), record)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Fields["topic"] != "Archive" || docs[0].Metadata["step2"] != "en" {
		t.Fatalf("expected ordered enrichment, got %+v", docs[0])
	}
}

func TestDocumentProjectorContentEnricherErrorsPropagate(t *testing.T) {
	want := errors.New("boom")
	projector := NewDocumentProjector(ProjectorConfig{
		Index:           "documents",
		RegistrationKey: "document",
		SourceType:      "content",
		ContentEnrichers: ContentRecordEnrichers{
			ContentRecordEnricherFunc(func(_ context.Context, _ ProjectionContext, _ *cmscontent.Content, _ *contentadapter.Record) error {
				return want
			}),
		},
	})
	record := &cmscontent.Content{
		ID:     uuid.New(),
		Slug:   "guide",
		Status: "published",
		Type: &cmscontent.ContentType{
			Slug: "article",
			Capabilities: map[string]any{
				"search": map[string]any{"enabled": true, "index": "documents"},
			},
		},
		Translations: []*cmscontent.ContentTranslation{
			{Locale: &cmscontent.Locale{Code: "en"}, Title: "Guide", Content: map[string]any{"body": "Hello"}},
		},
	}
	_, err := projector.Project(context.Background(), record)
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestPageProjectorNilEnrichersAreNoOp(t *testing.T) {
	projector := NewPageProjector(ProjectorConfig{
		Index:           "content_shared",
		RegistrationKey: "page",
		SourceType:      "page",
		PageEnrichers: PageRecordEnrichers{
			nil,
			PageRecordEnricherFunc(nil),
		},
	})
	record := &cmspages.Page{
		ID:     uuid.New(),
		Slug:   "home",
		Status: "published",
		Content: &cmscontent.Content{
			ID: uuid.New(),
			Type: &cmscontent.ContentType{
				Slug: "landing_page",
				Capabilities: map[string]any{
					"search": map[string]any{"enabled": true},
				},
			},
		},
		Translations: []*cmspages.PageTranslation{
			{ID: uuid.New(), Locale: "en", Title: "Home", Path: "/"},
		},
	}
	docs, err := projector.Project(context.Background(), record)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
}
