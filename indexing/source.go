package indexing

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/goliatone/go-search/pkg/types"
)

type Source[T any] interface {
	Get(ctx context.Context, id string) (T, error)
	List(ctx context.Context, limit int, cursor string) ([]T, string, error)
}

type Projector[T any] interface {
	Project(ctx context.Context, record T) ([]types.Document, error)
}

type Registration[T any] struct {
	indexName       string
	definition      types.IndexDefinition
	registrationKey string
	sourceType      string
	source          Source[T]
	projector       Projector[T]
	idGetter        func(T) string
	deleteSourceIDs func(context.Context, string) ([]string, error)
}

func NewRegistration[T any](indexName string, definition types.IndexDefinition, sourceType string, source Source[T], projector Projector[T], idGetter func(T) string) *Registration[T] {
	return NewRegistrationWithOptions(indexName, definition, sourceType, sourceType, source, projector, idGetter, RegistrationOptions{})
}

func NewRegistrationWithKey[T any](indexName string, definition types.IndexDefinition, registrationKey string, sourceType string, source Source[T], projector Projector[T], idGetter func(T) string) *Registration[T] {
	return NewRegistrationWithOptions(indexName, definition, registrationKey, sourceType, source, projector, idGetter, RegistrationOptions{})
}

type RegistrationOptions struct {
	DeleteSourceIDs func(context.Context, string) ([]string, error)
}

func NewRegistrationWithOptions[T any](indexName string, definition types.IndexDefinition, registrationKey string, sourceType string, source Source[T], projector Projector[T], idGetter func(T) string, opts RegistrationOptions) *Registration[T] {
	return &Registration[T]{
		indexName:       indexName,
		definition:      definition,
		registrationKey: registrationKey,
		sourceType:      sourceType,
		source:          source,
		projector:       projector,
		idGetter:        idGetter,
		deleteSourceIDs: opts.DeleteSourceIDs,
	}
}

func (r *Registration[T]) IndexName() string {
	return r.indexName
}

func (r *Registration[T]) Definition() types.IndexDefinition {
	return r.definition
}

func (r *Registration[T]) RegistrationKey() string {
	return r.registrationKey
}

func (r *Registration[T]) SourceType() string {
	return r.sourceType
}

func (r *Registration[T]) IndexRecord(ctx context.Context, recordID string) ([]types.Document, error) {
	record, err := r.source.Get(ctx, recordID)
	if err != nil {
		return nil, err
	}
	docs, err := r.projector.Project(ctx, record)
	if err != nil {
		return nil, err
	}
	id := recordID
	if r.idGetter != nil {
		id = r.idGetter(record)
	}
	for i := range docs {
		docs[i].Index = r.definition.Name
		if docs[i].SourceType == "" {
			docs[i].SourceType = r.sourceType
		}
		if docs[i].SourceID == "" {
			docs[i].SourceID = id
		}
	}
	return docs, nil
}

func (r *Registration[T]) DeleteSourceIDs(ctx context.Context, recordID string) ([]string, error) {
	if r.deleteSourceIDs != nil {
		return r.deleteSourceIDs(ctx, recordID)
	}
	if r.idGetter == nil {
		return []string{recordID}, nil
	}
	record, err := r.source.Get(ctx, recordID)
	if err != nil {
		// Delete hooks may run after the upstream record is already gone.
		// Fall back to the provided record ID so legacy registrations still work.
		return []string{recordID}, nil
	}
	return []string{firstActivityValue(r.idGetter(record), recordID)}, nil
}

func (r *Registration[T]) ResolveActivityEvent(ctx context.Context, verb, recordID string, docs []types.Document, metadata map[string]any) (types.ActivityEvent, error) {
	event := types.ActivityEvent{
		Channel:    "search",
		Verb:       strings.TrimSpace(verb),
		ObjectType: strings.TrimSpace(r.sourceType),
		ObjectID:   strings.TrimSpace(recordID),
		RecordID:   strings.TrimSpace(recordID),
		Metadata:   cloneMetadata(metadata),
	}
	doc, err := r.activityDocument(ctx, recordID, docs)
	if err != nil {
		return event, err
	}
	applyDocumentActivityContext(&event, doc)
	return event, nil
}

func (r *Registration[T]) ListRecordIDs(ctx context.Context, limit int, cursor string) ([]string, string, error) {
	records, next, err := r.source.List(ctx, limit, cursor)
	if err != nil {
		return nil, "", err
	}
	out := make([]string, 0, len(records))
	for _, record := range records {
		if r.idGetter != nil {
			out = append(out, r.idGetter(record))
			continue
		}
		out = append(out, recordID(record))
	}
	return out, next, nil
}

func recordID[T any](record T) string {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		value = value.Elem()
	}
	if value.Kind() == reflect.Struct {
		field := value.FieldByName("ID")
		if field.IsValid() && field.CanInterface() {
			return fmt.Sprint(field.Interface())
		}
	}
	return ""
}

func (r *Registration[T]) activityDocument(ctx context.Context, recordID string, docs []types.Document) (*types.Document, error) {
	if doc := selectActivityDocument(docs, recordID); doc != nil {
		return doc, nil
	}
	record, err := r.source.Get(ctx, recordID)
	if err != nil {
		return nil, err
	}
	projected, err := r.projector.Project(ctx, record)
	if err != nil {
		return nil, err
	}
	return selectActivityDocument(projected, recordID), nil
}

func selectActivityDocument(docs []types.Document, recordID string) *types.Document {
	recordID = strings.TrimSpace(recordID)
	for i := range docs {
		doc := docs[i]
		if recordID == "" || strings.EqualFold(strings.TrimSpace(doc.SourceID), recordID) || strings.EqualFold(strings.TrimSpace(doc.ID), recordID) {
			copy := doc.Clone()
			return &copy
		}
	}
	if len(docs) == 0 {
		return nil
	}
	copy := docs[0].Clone()
	return &copy
}

func applyDocumentActivityContext(event *types.ActivityEvent, doc *types.Document) {
	if event == nil || doc == nil {
		return
	}
	if sourceType := strings.TrimSpace(doc.SourceType); sourceType != "" {
		event.ObjectType = sourceType
	}
	if objectID := firstActivityValue(strings.TrimSpace(doc.SourceID), strings.TrimSpace(event.ObjectID), strings.TrimSpace(doc.ID)); objectID != "" {
		event.ObjectID = objectID
	}
	if recordID := firstActivityValue(strings.TrimSpace(event.RecordID), strings.TrimSpace(doc.SourceID), strings.TrimSpace(doc.ID), fieldString(doc.Fields, "user_id")); recordID != "" {
		event.RecordID = recordID
	}
	if tenantID := strings.TrimSpace(doc.Scope.TenantID); tenantID != "" {
		event.TenantID = tenantID
	}
	if orgID := strings.TrimSpace(doc.Scope.OrgID); orgID != "" {
		event.OrgID = orgID
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	if event.RecordID != "" {
		event.Metadata["record_id"] = event.RecordID
	}
	if event.TenantID != "" {
		event.Metadata["tenant_id"] = event.TenantID
	}
	if event.OrgID != "" {
		event.Metadata["org_id"] = event.OrgID
	}
	if userID := fieldString(doc.Fields, "user_id"); userID != "" {
		event.Metadata["user_id"] = userID
	}
}

func fieldString(fields map[string]any, key string) string {
	if len(fields) == 0 {
		return ""
	}
	value, ok := fields[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstActivityValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	maps.Copy(out, metadata)
	return out
}
