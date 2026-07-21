package typesense

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/goliatone/go-search/pkg/types"
)

func TestReplaceDocumentsRestoresOldSetWhenStaleDeletionFails(t *testing.T) {
	const (
		collection      = "media"
		registrationKey = "archive"
		sourceID        = "transcript-1"
	)
	storageID := func(documentID string) string {
		return storageDocumentIDFor(registrationKey, documentID)
	}
	oldDocument := func(documentID, body string) map[string]any {
		return map[string]any{
			"id": storageID(documentID), "document_id": documentID,
			"registration_key": registrationKey, "source_id": sourceID,
			"body": body,
		}
	}
	documents := map[string]map[string]any{
		storageID("old-1"): oldDocument("old-1", "old first"),
		storageID("old-2"): oldDocument("old-2", "old second"),
	}
	order := []string{storageID("old-1"), storageID("old-2"), storageID("new")}
	deleteAttempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/collections/"+collection+"/documents")
		switch {
		case r.Method == http.MethodPost && path == "/import":
			scanner := bufio.NewScanner(r.Body)
			for scanner.Scan() {
				var document map[string]any
				if err := json.Unmarshal(scanner.Bytes(), &document); err != nil {
					t.Fatal(err)
				}
				id := stringify(document["id"])
				documents[id] = document
				_, _ = w.Write([]byte(`{"success":true,"id":` + mustJSON(t, id) + "}\n"))
			}
			return
		case r.Method == http.MethodGet && path == "/search":
			hits := make([]map[string]any, 0, len(documents))
			for _, id := range order {
				if document := documents[id]; document != nil {
					hits = append(hits, map[string]any{"document": document})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"found": len(hits), "hits": hits, "page": 1, "search_time_ms": 1})
			return
		case r.Method == http.MethodPost && path == "":
			var document map[string]any
			if err := json.NewDecoder(r.Body).Decode(&document); err != nil {
				t.Fatal(err)
			}
			documents[stringify(document["id"])] = document
			_ = json.NewEncoder(w).Encode(document)
			return
		case strings.HasPrefix(path, "/"):
			id, err := url.PathUnescape(strings.TrimPrefix(path, "/"))
			if err != nil {
				t.Fatal(err)
			}
			document := documents[id]
			switch r.Method {
			case http.MethodGet:
				if document == nil {
					writeTypesenseTestError(w, http.StatusNotFound, "not found")
					return
				}
				_ = json.NewEncoder(w).Encode(document)
				return
			case http.MethodDelete:
				deleteAttempts++
				if id == storageID("old-2") {
					writeTypesenseTestError(w, http.StatusInternalServerError, "injected stale delete failure")
					return
				}
				if document == nil {
					writeTypesenseTestError(w, http.StatusNotFound, "not found")
					return
				}
				delete(documents, id)
				_ = json.NewEncoder(w).Encode(document)
				return
			}
		}
		writeTypesenseTestError(w, http.StatusNotFound, "unexpected request")
	}))
	t.Cleanup(server.Close)

	provider, err := New(Config{ServerURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := managedIndex{collectionName: collection, def: types.IndexDefinition{Name: collection}}
	err = replaceDocuments(t.Context(), provider.client, runtime, registrationKey, []string{sourceID}, []types.Document{{
		ID: "new", Index: collection, SourceID: sourceID, Body: "new replacement",
	}})
	if err == nil || !strings.Contains(err.Error(), "injected stale delete failure") {
		t.Fatalf("replace error = %v", err)
	}
	if deleteAttempts < 2 {
		t.Fatalf("delete attempts = %d", deleteAttempts)
	}
	if documents[storageID("new")] != nil {
		t.Fatalf("replacement document survived rollback: %#v", documents[storageID("new")])
	}
	for id, body := range map[string]string{"old-1": "old first", "old-2": "old second"} {
		document := documents[storageID(id)]
		if document == nil || stringify(document["body"]) != body {
			t.Fatalf("old document %s was not restored: %#v", id, document)
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func writeTypesenseTestError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"message": message})
}
