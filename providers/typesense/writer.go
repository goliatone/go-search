package typesense

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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
	snapshots := make([]documentSnapshot, 0, len(docs))
	for _, doc := range docs {
		compiled := compileDocument(runtime.def, doc)
		payload = append(payload, compiled)
		snapshot, err := captureDocumentSnapshot(ctx, client, runtime, compiled)
		if err != nil {
			return err
		}
		snapshots = append(snapshots, snapshot)
	}
	results, err := client.Collection(runtime.collectionName).Documents().Import(ctx, payload, &tsapi.ImportDocumentsParams{
		Action:   new(tsapi.Upsert),
		ReturnId: new(true),
	})
	if err != nil {
		return errs.Wrap(err, map[string]any{"collection": runtime.collectionName, "index": runtime.def.Name})
	}
	var joined error
	for i, result := range results {
		if result == nil || result.Success {
			continue
		}
		if rollbackErr := rollbackDocumentSnapshots(ctx, client, runtime, snapshots, results); rollbackErr != nil {
			joined = errors.Join(io.ErrUnexpectedEOF, rollbackErr)
		} else {
			joined = io.ErrUnexpectedEOF
		}
		return errs.Wrap(joined, map[string]any{
			"collection": runtime.collectionName,
			"index":      runtime.def.Name,
			"position":   i,
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

func deleteByRegistration(ctx context.Context, client *tstypesense.Client, runtime managedIndex, registrationKey string) error {
	ids, err := listDocumentIDsByField(ctx, client, runtime, "registration_key", []string{strings.TrimSpace(registrationKey)})
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
		"id":                     storageDocumentID(doc),
		"document_id":            doc.ID,
		"index":                  firstNonEmpty(doc.Index, def.Name),
		"registration_key":       strings.TrimSpace(doc.RegistrationKey),
		"type":                   doc.Type,
		"parent_id":              doc.ParentID,
		"source_type":            doc.SourceType,
		"source_id":              doc.SourceID,
		"title":                  doc.Title,
		"summary":                doc.Summary,
		"body":                   doc.Body,
		"url":                    doc.URL,
		"anchor_url":             doc.AnchorURL,
		"locale":                 doc.Locale,
		"scope_tenant_id":        strings.TrimSpace(doc.Scope.TenantID),
		"scope_org_id":           strings.TrimSpace(doc.Scope.OrgID),
		"scope_labels":           scopeLabelTokens(doc.Scope.Labels),
		"visibility_public":      doc.Visibility.Public,
		"visibility_roles":       append([]string(nil), doc.Visibility.Roles...),
		"visibility_permissions": append([]string(nil), doc.Visibility.Permissions...),
		"visibility_status":      strings.TrimSpace(doc.Visibility.Status),
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

type documentSnapshot struct {
	DocumentID string
	Exists     bool
	Previous   map[string]any
	Attempted  string
}

func captureDocumentSnapshot(ctx context.Context, client *tstypesense.Client, runtime managedIndex, payload map[string]any) (documentSnapshot, error) {
	documentID := strings.TrimSpace(stringify(payload["id"]))
	if documentID == "" {
		return documentSnapshot{}, errs.InvalidInput("typesense document id is required", map[string]any{
			"collection": runtime.collectionName,
			"index":      runtime.def.Name,
		})
	}
	previous, err := client.Collection(runtime.collectionName).Document(documentID).Retrieve(ctx)
	if err != nil {
		if isTypesenseStatus(err, 404) {
			return documentSnapshot{
				DocumentID: documentID,
				Attempted:  documentPayloadHash(payload),
			}, nil
		}
		return documentSnapshot{}, errs.Wrap(err, map[string]any{
			"collection": runtime.collectionName,
			"index":      runtime.def.Name,
			"id":         documentID,
		})
	}
	return documentSnapshot{
		DocumentID: documentID,
		Exists:     true,
		Previous:   previous,
		Attempted:  documentPayloadHash(payload),
	}, nil
}

func rollbackDocumentSnapshots(ctx context.Context, client *tstypesense.Client, runtime managedIndex, snapshots []documentSnapshot, results []*tsapi.ImportDocumentResponse) error {
	var joined error
	for i := len(results) - 1; i >= 0; i-- {
		if i >= len(snapshots) {
			continue
		}
		result := results[i]
		if result == nil || !result.Success {
			continue
		}
		snapshot := snapshots[i]
		current, err := currentDocumentSnapshot(ctx, client, runtime, snapshot.DocumentID)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		if current == nil || documentPayloadHash(current) != snapshot.Attempted {
			// Another writer changed the document after our write landed.
			// Skip rollback so we do not overwrite newer state.
			continue
		}
		if snapshot.Exists {
			if _, err := client.Collection(runtime.collectionName).Documents().Upsert(ctx, snapshot.Previous, &tsapi.DocumentIndexParameters{}); err != nil {
				joined = errors.Join(joined, errs.Wrap(err, map[string]any{
					"collection": runtime.collectionName,
					"index":      runtime.def.Name,
					"id":         snapshot.DocumentID,
					"operation":  "restore",
				}))
			}
			continue
		}
		if _, err := client.Collection(runtime.collectionName).Document(snapshot.DocumentID).Delete(ctx); err != nil && !isTypesenseStatus(err, 404) {
			joined = errors.Join(joined, errs.Wrap(err, map[string]any{
				"collection": runtime.collectionName,
				"index":      runtime.def.Name,
				"id":         snapshot.DocumentID,
				"operation":  "cleanup",
			}))
		}
	}
	return joined
}

func currentDocumentSnapshot(ctx context.Context, client *tstypesense.Client, runtime managedIndex, documentID string) (map[string]any, error) {
	current, err := client.Collection(runtime.collectionName).Document(documentID).Retrieve(ctx)
	if err != nil {
		if isTypesenseStatus(err, 404) {
			return nil, nil
		}
		return nil, errs.Wrap(err, map[string]any{
			"collection": runtime.collectionName,
			"index":      runtime.def.Name,
			"id":         documentID,
			"operation":  "reload",
		})
	}
	return current, nil
}

func documentPayloadHash(payload map[string]any) string {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
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
		// Typesense reserves `id` as the internal document identifier, so
		// listing by filtered fields must still query on a schema field.
		queryBy := "document_id"
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

func stringify(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		body, _ := json.Marshal(v)
		return strings.Trim(string(body), "\"")
	}
}
