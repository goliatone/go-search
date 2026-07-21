package typesense

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestReplaceDocumentsRejectsIncompleteImportResponsesBeforeStaleDeletion(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{name: "truncated", response: `{"success":true}` + "\n"},
		{name: "nil result", response: `{"success":true}` + "\nnull\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const (
				collection      = "media"
				registrationKey = "archive"
				sourceID        = "transcript-1"
			)
			storageID := func(documentID string) string {
				return storageDocumentIDFor(registrationKey, documentID)
			}
			documents := map[string]map[string]any{
				storageID("old"): {
					"id": storageID("old"), "document_id": "old",
					"registration_key": registrationKey, "source_id": sourceID,
					"body": "old body",
				},
			}
			staleDeleteAttempts := 0

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
						documents[stringify(document["id"])] = document
					}
					_, _ = w.Write([]byte(tt.response))
					return
				case r.Method == http.MethodGet && path == "/search":
					hits := make([]map[string]any, 0, len(documents))
					for _, document := range documents {
						hits = append(hits, map[string]any{"document": document})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"found": len(hits), "hits": hits, "page": 1, "search_time_ms": 1})
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
						if id == storageID("old") {
							staleDeleteAttempts++
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
			err = replaceDocuments(t.Context(), provider.client, runtime, registrationKey, []string{sourceID}, []types.Document{
				{ID: "new-1", Index: collection, SourceID: sourceID, Body: "first"},
				{ID: "new-2", Index: collection, SourceID: sourceID, Body: "second"},
			})
			if err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
				t.Fatalf("replace error = %v", err)
			}
			if staleDeleteAttempts != 0 {
				t.Fatalf("stale deletion ran after incomplete import: %d", staleDeleteAttempts)
			}
			if documents[storageID("new-1")] != nil || documents[storageID("new-2")] != nil {
				t.Fatalf("incomplete import survived rollback: %#v", documents)
			}
			if document := documents[storageID("old")]; document == nil || stringify(document["body"]) != "old body" {
				t.Fatalf("old document changed: %#v", document)
			}
		})
	}
}

func TestProviderSerializesConcurrentMutationsPerCollection(t *testing.T) {
	const (
		collection      = "media"
		registrationKey = "archive"
		sourceID        = "transcript-1"
	)
	var documentsMu sync.Mutex
	documents := map[string]map[string]any{}
	importEntered := make(chan string, 2)
	releaseFirst := make(chan struct{})
	importCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/collections/"+collection+"/documents")
		switch {
		case r.Method == http.MethodPost && path == "/import":
			scanner := bufio.NewScanner(r.Body)
			payload := []map[string]any{}
			for scanner.Scan() {
				var document map[string]any
				if err := json.Unmarshal(scanner.Bytes(), &document); err != nil {
					t.Fatal(err)
				}
				payload = append(payload, document)
			}
			importCount++
			position := importCount
			importEntered <- stringify(payload[0]["document_id"])
			if position == 1 {
				<-releaseFirst
			}
			documentsMu.Lock()
			for _, document := range payload {
				documents[stringify(document["id"])] = document
			}
			documentsMu.Unlock()
			for _, document := range payload {
				_, _ = w.Write([]byte(`{"success":true,"id":` + mustJSON(t, stringify(document["id"])) + "}\n"))
			}
			return
		case r.Method == http.MethodGet && path == "/search":
			documentsMu.Lock()
			hits := make([]map[string]any, 0, len(documents))
			for _, document := range documents {
				hits = append(hits, map[string]any{"document": document})
			}
			documentsMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"found": len(hits), "hits": hits, "page": 1, "search_time_ms": 1})
			return
		case strings.HasPrefix(path, "/"):
			id, err := url.PathUnescape(strings.TrimPrefix(path, "/"))
			if err != nil {
				t.Fatal(err)
			}
			documentsMu.Lock()
			document := documents[id]
			switch r.Method {
			case http.MethodGet:
				documentsMu.Unlock()
				if document == nil {
					writeTypesenseTestError(w, http.StatusNotFound, "not found")
					return
				}
				_ = json.NewEncoder(w).Encode(document)
				return
			case http.MethodDelete:
				if document != nil {
					delete(documents, id)
				}
				documentsMu.Unlock()
				if document == nil {
					writeTypesenseTestError(w, http.StatusNotFound, "not found")
					return
				}
				_ = json.NewEncoder(w).Encode(document)
				return
			default:
				documentsMu.Unlock()
			}
		}
		writeTypesenseTestError(w, http.StatusNotFound, "unexpected request")
	}))
	t.Cleanup(server.Close)

	provider, err := New(Config{ServerURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	provider.indexes[collection] = managedIndex{collectionName: collection, def: types.IndexDefinition{Name: collection}}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- provider.ReplaceDocuments(t.Context(), collection, registrationKey, []string{sourceID}, []types.Document{{
			ID: "first", Index: collection, SourceID: sourceID, Body: "first",
		}})
	}()
	if got := <-importEntered; got != "first" {
		t.Fatalf("first import = %q", got)
	}
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- provider.ReplaceDocuments(t.Context(), collection, registrationKey, []string{sourceID}, []types.Document{{
			ID: "second", Index: collection, SourceID: sourceID, Body: "second",
		}})
	}()
	select {
	case got := <-importEntered:
		t.Fatalf("concurrent import %q entered before the first mutation completed", got)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if got := <-importEntered; got != "second" {
		t.Fatalf("second import = %q", got)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}

	documentsMu.Lock()
	defer documentsMu.Unlock()
	if len(documents) != 1 || documents[storageDocumentIDFor(registrationKey, "second")] == nil {
		t.Fatalf("final documents = %#v", documents)
	}
}

func TestProviderMutationLockCoordinatesExternalWriters(t *testing.T) {
	locker := &recordingTypesenseMutationLocker{}
	provider := &Provider{cfg: Config{MutationLocker: locker}, mutationLocks: map[string]*sync.Mutex{}}
	called := false
	if err := provider.withMutationLock(t.Context(), "media", func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called || locker.calls != 1 {
		t.Fatalf("mutation called=%v external lock calls=%d", called, locker.calls)
	}
}

type recordingTypesenseMutationLocker struct {
	calls int
}

func (l *recordingTypesenseMutationLocker) WithLock(_ context.Context, mutate func() error) error {
	l.calls++
	return mutate()
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
