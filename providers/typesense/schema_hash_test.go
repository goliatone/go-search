package typesense

import (
	"testing"

	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

func TestCollectionSchemaHashTreatsDefaultIndexedFieldsLikeTypesenseResponse(t *testing.T) {
	schema := &tsapi.CollectionSchema{
		Name: "search_docs",
		Fields: []tsapi.Field{
			{
				Name:     "id",
				Type:     "string",
				Index:    new(true),
				Optional: new(false),
			},
			{
				Name:     "document_id",
				Type:     "string",
				Optional: new(true),
			},
			{
				Name:     "__exists_document_id",
				Type:     "bool",
				Facet:    new(true),
				Optional: new(true),
			},
			{
				Name:       "published_year",
				Type:       "int64",
				Facet:      new(true),
				Optional:   new(true),
				RangeIndex: new(true),
			},
			{
				Name:     "title",
				Type:     "string",
				Index:    new(true),
				Optional: new(true),
			},
		},
	}

	response := &tsapi.CollectionResponse{
		Name: "search_docs",
		Fields: []tsapi.Field{
			{
				Name:     "document_id",
				Type:     "string",
				Index:    new(true),
				Optional: new(true),
			},
			{
				Name:     "__exists_document_id",
				Type:     "bool",
				Facet:    new(true),
				Sort:     new(true),
				Index:    new(true),
				Optional: new(true),
			},
			{
				Name:       "published_year",
				Type:       "int64",
				Facet:      new(true),
				Sort:       new(true),
				Index:      new(true),
				Optional:   new(true),
				RangeIndex: new(true),
			},
			{
				Name:     "title",
				Type:     "string",
				Index:    new(true),
				Optional: new(true),
			},
		},
	}

	if got, want := collectionSchemaHash(schema), collectionResponseHash(response); got != want {
		t.Fatalf("expected schema hash %q to match response hash %q", got, want)
	}
}

func TestCollectionSchemaHashIgnoresPhysicalCollectionName(t *testing.T) {
	schema := &tsapi.CollectionSchema{
		Name:   "site_content__active",
		Fields: []tsapi.Field{{Name: "title", Type: "string", Index: new(true), Optional: new(true)}},
	}
	response := &tsapi.CollectionResponse{
		Name:   "site_content__gen_20260712",
		Fields: []tsapi.Field{{Name: "title", Type: "string", Index: new(true), Optional: new(true)}},
	}
	if got, want := collectionSchemaHash(schema), collectionResponseHash(response); got != want {
		t.Fatalf("logical schema hash %q must ignore physical collection name; response hash = %q", got, want)
	}
}
