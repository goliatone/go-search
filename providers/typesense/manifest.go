package typesense

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

// VerifyDocumentManifest compares the canonical storage payload for every
// projected document with a fresh Typesense export. It catches successful API
// writes that stored stale, truncated, or otherwise incorrect public identity,
// visibility, locale, URL, or facet fields before a generation is activated.
func (p *Provider) VerifyDocumentManifest(ctx context.Context, index string, docs []types.Document) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	expected := make(map[string]string, len(docs))
	for _, doc := range docs {
		payload := normalizeDocumentPayloadForManifest(compileDocument(runtime.def, doc))
		id := strings.TrimSpace(stringify(payload["id"]))
		if id == "" {
			return errs.InvalidInput("typesense manifest document id is required", map[string]any{"index": index})
		}
		if _, duplicate := expected[id]; duplicate {
			return errs.InvalidInput("typesense manifest contains a duplicate storage document id", map[string]any{"index": index, "id": id})
		}
		expected[id] = documentPayloadHash(payload)
	}

	reader, err := p.client.Collection(runtime.collectionName).Documents().Export(ctx, &tsapi.ExportDocumentsParams{})
	if err != nil {
		return errs.Wrap(err, map[string]any{"index": index, "collection": runtime.collectionName, "operation": "export_manifest"})
	}
	defer reader.Close()
	actual := map[string]string{}
	decoder := json.NewDecoder(reader)
	for {
		var payload map[string]any
		if err := decoder.Decode(&payload); err != nil {
			if err == io.EOF {
				break
			}
			return errs.Wrap(err, map[string]any{"index": index, "collection": runtime.collectionName, "operation": "decode_manifest"})
		}
		id := strings.TrimSpace(stringify(payload["id"]))
		if id == "" {
			return fmt.Errorf("typesense exported manifest for %q contains an empty storage id", index)
		}
		if _, duplicate := actual[id]; duplicate {
			return fmt.Errorf("typesense exported manifest for %q contains duplicate storage id %q", index, id)
		}
		actual[id] = documentPayloadHash(normalizeDocumentPayloadForManifest(payload))
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("typesense document manifest for %q has %d documents; expected %d", index, len(actual), len(expected))
	}
	for id, expectedHash := range expected {
		actualHash, ok := actual[id]
		if !ok {
			return fmt.Errorf("typesense document manifest for %q is missing storage document %q", index, id)
		}
		if actualHash != expectedHash {
			return fmt.Errorf("typesense document manifest for %q has content mismatch for storage document %q", index, id)
		}
	}
	return nil
}

func normalizeDocumentPayloadForManifest(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		switch typed := value.(type) {
		case nil:
			continue
		case []string:
			if len(typed) == 0 {
				continue
			}
		case []any:
			if len(typed) == 0 {
				continue
			}
		}
		out[key] = value
	}
	return out
}
