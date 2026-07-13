package typesense

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

func TestHealthDefinitionsReportsMissingIndexWithoutRegistrationOrMutation(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "collection not found"})
	}))
	t.Cleanup(server.Close)

	provider, err := New(Config{ServerURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	health, err := provider.HealthDefinitions(context.Background(), []types.IndexDefinition{{
		Name:               "missing",
		DefaultQueryFields: []string{"title"},
	}})
	if err != nil {
		t.Fatalf("HealthDefinitions() error = %v", err)
	}
	if health.Healthy || len(health.Indexes) != 1 || health.Indexes[0].Ready {
		t.Fatalf("HealthDefinitions() = %#v", health)
	}
	if health.Indexes[0].Message != "collection not found" {
		t.Fatalf("message = %q", health.Indexes[0].Message)
	}
	if len(methods) != 1 || methods[0] != http.MethodGet {
		t.Fatalf("health requests = %#v, want one read-only GET", methods)
	}
	if len(provider.indexes) != 0 {
		t.Fatalf("health registered provider indexes: %#v", provider.indexes)
	}
}

func TestHealthDefinitionsReportsSchemaMismatchWithActiveCollectionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":          "physical-v1",
			"fields":        []any{},
			"num_documents": 7,
		})
	}))
	t.Cleanup(server.Close)

	provider, err := New(Config{ServerURL: server.URL, APIKey: "test-key", CollectionNamer: func(string) string { return "stable_alias" }})
	if err != nil {
		t.Fatal(err)
	}
	health, err := provider.HealthDefinitions(context.Background(), []types.IndexDefinition{{
		Name:               "site_content",
		DefaultQueryFields: []string{"title"},
	}})
	if err != nil {
		t.Fatalf("HealthDefinitions() error = %v", err)
	}
	if health.Healthy || len(health.Indexes) != 1 || health.Indexes[0].Ready {
		t.Fatalf("HealthDefinitions() = %#v", health)
	}
	index := health.Indexes[0]
	if index.Documents != 7 || index.Metadata["collection_name"] != "stable_alias" || index.Metadata["active_collection_name"] != "physical-v1" {
		t.Fatalf("index health = %#v", index)
	}
	if index.Metadata["schema_match"] != false || index.Metadata["actual_schema_hash"] == index.Metadata["expected_schema_hash"] {
		t.Fatalf("schema metadata = %#v", index.Metadata)
	}
}
