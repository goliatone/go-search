package typesense

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	tstypesense "github.com/typesense/typesense-go/v3/typesense"
	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

func upsertDocuments(ctx context.Context, client *tstypesense.Client, runtime managedIndex, docs []types.Document) error {
	if len(docs) == 0 {
		return nil
	}
	payload := make([]any, 0, len(docs))
	for _, doc := range docs {
		payload = append(payload, compileDocument(runtime.def, doc))
	}
	results, err := client.Collection(runtime.collectionName).Documents().Import(ctx, payload, &tsapi.ImportDocumentsParams{
		Action:   new(tsapi.Upsert),
		ReturnId: new(true),
	})
	if err != nil {
		return errs.Wrap(err, map[string]any{"collection": runtime.collectionName, "index": runtime.def.Name})
	}
	for _, result := range results {
		if result == nil || result.Success {
			continue
		}
		return errs.Wrap(io.ErrUnexpectedEOF, map[string]any{
			"collection": runtime.collectionName,
			"index":      runtime.def.Name,
			"error":      result.Error,
			"document":   result.Document,
		})
	}
	return nil
}

func deleteDocuments(ctx context.Context, client *tstypesense.Client, runtime managedIndex, ids []string) error {
	storageIDs, err := listDocumentIDsByField(ctx, client, runtime, "document_id", ids, "")
	if err != nil {
		return err
	}
	return deleteDocumentsByStorageID(ctx, client, runtime, storageIDs)
}

func deleteDocumentsByStorageID(ctx context.Context, client *tstypesense.Client, runtime managedIndex, ids []string) error {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := client.Collection(runtime.collectionName).Document(id).Delete(ctx); err != nil && !isTypesenseStatus(err, 404) {
			return errs.Wrap(err, map[string]any{"collection": runtime.collectionName, "index": runtime.def.Name, "id": id})
		}
	}
	return nil
}

func deleteBySource(ctx context.Context, client *tstypesense.Client, runtime managedIndex, registrationKey string, sourceIDs []string) error {
	ids, err := listDocumentIDsBySource(ctx, client, runtime, registrationKey, sourceIDs)
	if err != nil {
		return err
	}
	return deleteDocumentsByStorageID(ctx, client, runtime, ids)
}

func replaceDocuments(ctx context.Context, client *tstypesense.Client, runtime managedIndex, registrationKey string, sourceIDs []string, docs []types.Document) error {
	registrationKey = strings.TrimSpace(registrationKey)
	for i := range docs {
		docs[i].RegistrationKey = firstNonEmpty(strings.TrimSpace(docs[i].RegistrationKey), registrationKey)
	}
	if err := upsertDocuments(ctx, client, runtime, docs); err != nil {
		return err
	}
	if len(sourceIDs) == 0 {
		return nil
	}
	existingIDs, err := listDocumentIDsBySource(ctx, client, runtime, registrationKey, sourceIDs)
	if err != nil {
		return err
	}
	keep := map[string]struct{}{}
	for _, doc := range docs {
		if id := strings.TrimSpace(storageDocumentID(doc)); id != "" {
			keep[id] = struct{}{}
		}
	}
	stale := make([]string, 0, len(existingIDs))
	for _, id := range existingIDs {
		if _, ok := keep[id]; ok {
			continue
		}
		stale = append(stale, id)
	}
	return deleteDocumentsByStorageID(ctx, client, runtime, stale)
}

func compileDocument(def types.IndexDefinition, doc types.Document) map[string]any {
	payload := map[string]any{
		"id":               storageDocumentID(doc),
		"document_id":      doc.ID,
		"index":            doc.Index,
		"registration_key": strings.TrimSpace(doc.RegistrationKey),
		"type":             doc.Type,
		"parent_id":        doc.ParentID,
		"source_type":      doc.SourceType,
		"source_id":        doc.SourceID,
		"title":            doc.Title,
		"summary":          doc.Summary,
		"body":             doc.Body,
		"url":              doc.URL,
		"anchor_url":       doc.AnchorURL,
		"locale":           doc.Locale,
	}
	if doc.StartMS != nil {
		payload["start_ms"] = *doc.StartMS
	}
	if doc.EndMS != nil {
		payload["end_ms"] = *doc.EndMS
	}

	if value, ok := doc.Fields["parent_title"]; ok {
		payload["parent_title"] = value
	}
	if value, ok := doc.Fields["parent_summary"]; ok {
		payload["parent_summary"] = value
	}
	if value, ok := doc.Fields["parent_url"]; ok {
		payload["parent_url"] = value
	}
	if value, ok := doc.Fields["parent_thumbnail"]; ok {
		payload["parent_thumbnail"] = value
	}
	if value, ok := doc.Fields["track_kind"]; ok {
		payload["track_kind"] = value
	}
	if value, ok := doc.Fields["source_format"]; ok {
		payload["source_format"] = value
	}
	if values, ok := doc.Facets["topic"]; ok && len(values) > 0 {
		payload["topic"] = append([]string(nil), values...)
	}

	customFields := declaredFields(def)
	for field := range customFields {
		if _, ok := payload[field]; ok {
			continue
		}
		switch {
		case doc.Fields != nil && doc.Fields[field] != nil:
			payload[field] = doc.Fields[field]
		case doc.Numeric != nil:
			if value, ok := doc.Numeric[field]; ok {
				payload[field] = value
				continue
			}
			fallthrough
		case doc.Booleans != nil:
			if value, ok := doc.Booleans[field]; ok {
				payload[field] = value
				continue
			}
			fallthrough
		case doc.Metadata != nil:
			if value, ok := doc.Metadata[field]; ok {
				payload[field] = value
				continue
			}
		case doc.Facets != nil:
			if value, ok := doc.Facets[field]; ok {
				payload[field] = append([]string(nil), value...)
			}
		}
	}
	for _, field := range existenceTrackedFields(def) {
		payload[existsFieldName(field)] = hasMeaningfulValue(payload[field])
	}
	return payload
}

func extractDocumentIDs(result *tsapi.SearchResult) []string {
	if result == nil || result.Hits == nil {
		return nil
	}
	out := []string{}
	for _, hit := range *result.Hits {
		if hit.Document == nil {
			continue
		}
		if value, ok := (*hit.Document)["id"]; ok {
			out = append(out, stringify(value))
		}
	}
	return out
}

func listDocumentIDsBySource(ctx context.Context, client *tstypesense.Client, runtime managedIndex, registrationKey string, sourceIDs []string) ([]string, error) {
	return listDocumentIDsByField(ctx, client, runtime, "source_id", sourceIDs, "registration_key:="+encodeStringValue(strings.TrimSpace(registrationKey)))
}

func listDocumentIDsByField(ctx context.Context, client *tstypesense.Client, runtime managedIndex, field string, rawValues []string, extraFilters ...string) ([]string, error) {
	values := make([]string, 0, len(rawValues))
	for _, id := range rawValues {
		id = strings.TrimSpace(id)
		if id != "" {
			values = append(values, encodeStringValue(id))
		}
	}
	if len(values) == 0 {
		return nil, nil
	}

	filterParts := []string{field + ":=[" + strings.Join(values, ",") + "]"}
	for _, filter := range extraFilters {
		if strings.TrimSpace(filter) != "" {
			filterParts = append(filterParts, filter)
		}
	}
	filter := strings.Join(filterParts, " && ")
	out := []string{}
	for pageNumber := 1; ; pageNumber++ {
		query := "*"
		queryBy := "id"
		perPage := 250
		includeFields := "id"
		result, err := client.Collection(runtime.collectionName).Documents().Search(ctx, &tsapi.SearchCollectionParams{
			Q:             &query,
			QueryBy:       &queryBy,
			FilterBy:      &filter,
			Page:          new(pageNumber),
			PerPage:       &perPage,
			IncludeFields: &includeFields,
		})
		if err != nil {
			return nil, errs.Wrap(err, map[string]any{"collection": runtime.collectionName, "index": runtime.def.Name, "filter": filter})
		}

		ids := extractDocumentIDs(result)
		if len(ids) == 0 {
			return out, nil
		}
		out = append(out, ids...)
		if len(ids) < perPage {
			return out, nil
		}
	}
}

func storageDocumentID(doc types.Document) string {
	return storageDocumentIDFor(strings.TrimSpace(doc.RegistrationKey), strings.TrimSpace(doc.ID))
}

func storageDocumentIDFor(registrationKey, documentID string) string {
	if documentID == "" {
		return ""
	}
	if registrationKey == "" {
		registrationKey = "_default"
	}
	return registrationKey + "::" + documentID
}

func hasMeaningfulValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []string:
		return len(v) > 0
	case []any:
		return len(v) > 0
	default:
		return true
	}
}

//go:fix inline
func indexActionPtr(value tsapi.IndexAction) *tsapi.IndexAction { return new(value) }

func stringify(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		body, _ := json.Marshal(v)
		return strings.Trim(string(body), "\"")
	}
}
