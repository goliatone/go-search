package media

import "context"

type TranscriptSource struct {
	records map[string]TranscriptRecord
	order   []string
}

func NewTranscriptSource(records []TranscriptRecord) *TranscriptSource {
	out := &TranscriptSource{
		records: map[string]TranscriptRecord{},
		order:   make([]string, 0, len(records)),
	}
	for _, record := range records {
		out.records[record.ID] = record
		out.order = append(out.order, record.ID)
	}
	return out
}

func (s *TranscriptSource) Get(_ context.Context, id string) (TranscriptRecord, error) {
	return s.records[id], nil
}

func (s *TranscriptSource) List(_ context.Context, limit int, cursor string) ([]TranscriptRecord, string, error) {
	start := 0
	if cursor != "" {
		for i, id := range s.order {
			if id == cursor {
				start = i + 1
				break
			}
		}
	}
	if limit <= 0 {
		limit = len(s.order)
	}
	end := min(start+limit, len(s.order))
	out := make([]TranscriptRecord, 0, end-start)
	for _, id := range s.order[start:end] {
		out = append(out, s.records[id])
	}
	next := ""
	if end < len(s.order) {
		next = s.order[end-1]
	}
	return out, next, nil
}
