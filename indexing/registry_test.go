package indexing

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

type registryIndexerStub struct {
	index string
	key   string
	typ   string
}

func (s registryIndexerStub) IndexName() string { return s.index }
func (s registryIndexerStub) Definition() types.IndexDefinition {
	return types.IndexDefinition{Name: s.index}
}
func (s registryIndexerStub) RegistrationKey() string { return s.key }
func (s registryIndexerStub) SourceType() string      { return s.typ }
func (s registryIndexerStub) IndexRecord(_ context.Context, _ string) ([]types.Document, error) {
	return nil, nil
}
func (s registryIndexerStub) DeleteSourceIDs(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (s registryIndexerStub) ListRecordIDs(_ context.Context, _ int, _ string) ([]string, string, error) {
	return nil, "", nil
}

func TestRegistryListsRegistrationsInStableKeyOrder(t *testing.T) {
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "content"}
	if err := registry.Register(def, registryIndexerStub{index: "content", key: "blog_article", typ: "blog_article"}); err != nil {
		t.Fatalf("register blog: %v", err)
	}
	if err := registry.Register(def, registryIndexerStub{index: "content", key: "video", typ: "video"}); err != nil {
		t.Fatalf("register video: %v", err)
	}
	items := registry.ListRegistrations("content")
	if len(items) != 2 {
		t.Fatalf("registration count = %d", len(items))
	}
	if items[0].RegistrationKey != "blog_article" || items[1].RegistrationKey != "video" {
		t.Fatalf("unexpected registration order: %+v", items)
	}
}

func TestRegistryResolveRegistrationRequiresKeyWhenAmbiguous(t *testing.T) {
	registry := NewRegistry()
	def := types.IndexDefinition{Name: "content"}
	_ = registry.Register(def, registryIndexerStub{index: "content", key: "document", typ: "document"})
	_ = registry.Register(def, registryIndexerStub{index: "content", key: "video", typ: "video"})
	if _, err := registry.ResolveRegistration("content", ""); err == nil {
		t.Fatalf("expected ambiguous registration resolution to fail")
	}
	item, err := registry.ResolveRegistration("content", "video")
	if err != nil {
		t.Fatalf("resolve registration: %v", err)
	}
	if item.RegistrationKey != "video" {
		t.Fatalf("registration key = %q", item.RegistrationKey)
	}
}

func TestRegistryDeepClonesIndexDefinitions(t *testing.T) {
	registry := NewRegistry()
	def := types.IndexDefinition{
		Name:             "content",
		SearchableFields: []string{"title"},
		ProviderHints:    map[string]any{"provider": map[string]any{"mode": "original"}},
		Semantic: &types.SemanticIndexConfig{Fields: []types.EmbeddingField{{
			Name: "embedding", SourceFields: []string{"body"}, Chunking: &types.ChunkingConfig{MaxCharacters: 100},
		}}},
	}
	if err := registry.Register(def, nil); err != nil {
		t.Fatal(err)
	}
	def.SearchableFields[0] = "mutated"
	def.ProviderHints["provider"].(map[string]any)["mode"] = "mutated"
	def.Semantic.Fields[0].SourceFields[0] = "mutated"
	stored, _ := registry.GetIndex("content")
	if stored.SearchableFields[0] != "title" || stored.ProviderHints["provider"].(map[string]any)["mode"] != "original" || stored.Semantic.Fields[0].SourceFields[0] != "body" {
		t.Fatalf("registration alias leaked: %+v", stored)
	}
	stored.ProviderHints["provider"].(map[string]any)["mode"] = "mutated"
	again, _ := registry.GetIndex("content")
	if again.ProviderHints["provider"].(map[string]any)["mode"] != "original" {
		t.Fatalf("lookup alias leaked: %+v", again.ProviderHints)
	}
}
