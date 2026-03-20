package command

import (
	"context"
	"errors"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/providers/memory"
)

type failingGenerationStore struct{}

func (failingGenerationStore) Get(context.Context, string) (int64, error) {
	return 0, errors.New("boom")
}

func (failingGenerationStore) Bump(context.Context, string) (int64, error) {
	return 0, errors.New("boom")
}

func TestUpsertDocumentsReturnsGenerationStoreError(t *testing.T) {
	cmd, err := NewUpsertDocuments(UpsertDocumentsConfig{
		Provider:        memory.New(memory.Config{}),
		GenerationStore: failingGenerationStore{},
	})
	if err != nil {
		t.Fatalf("new upsert command: %v", err)
	}
	err = cmd.Execute(context.Background(), types.UpsertDocumentsInput{
		Index: "media",
		Documents: []types.Document{
			{ID: "doc-1", Title: "Ocean Wind"},
		},
	})
	if err == nil {
		t.Fatalf("expected generation bump failure")
	}
}
