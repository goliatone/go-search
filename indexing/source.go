package indexing

import (
	"context"
	"fmt"
	"reflect"

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
}

func NewRegistration[T any](indexName string, definition types.IndexDefinition, sourceType string, source Source[T], projector Projector[T], idGetter func(T) string) *Registration[T] {
	return NewRegistrationWithKey(indexName, definition, sourceType, sourceType, source, projector, idGetter)
}

func NewRegistrationWithKey[T any](indexName string, definition types.IndexDefinition, registrationKey string, sourceType string, source Source[T], projector Projector[T], idGetter func(T) string) *Registration[T] {
	return &Registration[T]{
		indexName:       indexName,
		definition:      definition,
		registrationKey: registrationKey,
		sourceType:      sourceType,
		source:          source,
		projector:       projector,
		idGetter:        idGetter,
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

func (r *Registration[T]) DeleteSourceIDs(_ context.Context, recordID string) ([]string, error) {
	return []string{recordID}, nil
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
