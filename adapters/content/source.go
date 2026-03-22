package content

import (
	"context"
	"fmt"
)

type Record struct {
	ID            string
	Type          string
	SourceType    string
	SourceID      string
	SourceVersion string
	Title         string
	Summary       string
	Body          string
	URL           string
	Locale        string
	Fields        map[string]any
	Facets        map[string][]string
	Numeric       map[string]float64
	Booleans      map[string]bool
	Scope         map[string]string
	Metadata      map[string]any
}

type Source struct {
	records []Record
	byID    map[string]Record
}

func NewSource(records []Record) *Source {
	out := &Source{
		records: make([]Record, 0, len(records)),
		byID:    make(map[string]Record, len(records)),
	}
	for _, record := range records {
		out.records = append(out.records, cloneRecord(record))
		out.byID[record.ID] = cloneRecord(record)
	}
	return out
}

func (s *Source) Get(_ context.Context, id string) (Record, error) {
	record, ok := s.byID[id]
	if !ok {
		return Record{}, fmt.Errorf("content record %q not found", id)
	}
	return cloneRecord(record), nil
}

func (s *Source) List(_ context.Context, limit int, cursor string) ([]Record, string, error) {
	start := 0
	if cursor != "" {
		for i, record := range s.records {
			if record.ID == cursor {
				start = i + 1
				break
			}
		}
	}
	if start >= len(s.records) {
		return nil, "", nil
	}
	end := len(s.records)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	out := make([]Record, 0, end-start)
	for _, record := range s.records[start:end] {
		out = append(out, cloneRecord(record))
	}
	next := ""
	if end < len(s.records) {
		next = s.records[end-1].ID
	}
	return out, next, nil
}
