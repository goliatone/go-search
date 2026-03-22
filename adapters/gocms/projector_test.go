package gocms

import (
	"context"
	"testing"

	cmscontent "github.com/goliatone/go-cms/content"
	cmspages "github.com/goliatone/go-cms/pages"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/google/uuid"
)

func TestDocumentProjectorFansOutLocales(t *testing.T) {
	projector := NewDocumentProjector(ProjectorConfig{
		Index:           "documents",
		RegistrationKey: "document",
		SourceType:      "content",
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
	}
}

func TestPageProjectorUsesPageTypeAndLocaleSpecificIDs(t *testing.T) {
	projector := NewPageProjector(ProjectorConfig{
		Index:           "content_shared",
		RegistrationKey: "page",
		SourceType:      "page",
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
	if docs[0].Type != DocumentTypePage {
		t.Fatalf("doc type = %q", docs[0].Type)
	}
	if docs[0].ID == "" || docs[0].SourceID != record.ID.String() {
		t.Fatalf("unexpected doc identity: %+v", docs[0])
	}
}
