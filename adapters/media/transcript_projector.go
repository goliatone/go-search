package media

import (
	"context"
	"strings"

	"github.com/goliatone/go-search/indexing/subtitle"
	"github.com/goliatone/go-search/pkg/types"
)

type MediaRecord struct {
	ID        string
	Title     string
	Summary   string
	URL       string
	Thumbnail string
	Topic     string
	Locale    string
}

type TranscriptRecord struct {
	ID       string
	Media    MediaRecord
	Track    types.TranscriptTrack
	Content  string
	Format   string
	Metadata map[string]any
}

type TranscriptProjectorConfig struct {
	Index        string
	SourceType   string
	MergeVersion string
	MaxChars     int
	MaxGapMS     int64
}

type TranscriptProjector struct {
	cfg TranscriptProjectorConfig
}

func NewTranscriptProjector(cfg TranscriptProjectorConfig) *TranscriptProjector {
	if cfg.MergeVersion == "" {
		cfg.MergeVersion = "v1"
	}
	return &TranscriptProjector{cfg: cfg}
}

func (p *TranscriptProjector) Project(_ context.Context, record TranscriptRecord) ([]types.Document, error) {
	cues, err := subtitle.Parse(record.Format, record.Content)
	if err != nil {
		return nil, err
	}
	merged := subtitle.MergeCues(cues, subtitle.MergeConfig{
		MaxCharacters: p.cfg.MaxChars,
		MaxGapMS:      p.cfg.MaxGapMS,
		Version:       p.cfg.MergeVersion,
	})
	parent := types.Document{
		ID:         record.Media.ID,
		Index:      p.cfg.Index,
		Type:       types.DocumentTypeVideo,
		SourceType: p.cfg.SourceType,
		SourceID:   record.Media.ID,
		Title:      record.Media.Title,
		Summary:    record.Media.Summary,
		URL:        record.Media.URL,
		Locale:     record.Media.Locale,
		Facets: map[string][]string{
			"topic":  compact(record.Media.Topic),
			"locale": compact(record.Media.Locale),
		},
		Metadata: map[string]any{
			"thumbnail": record.Media.Thumbnail,
		},
	}
	docs := []types.Document{parent}
	docs = append(docs, subtitle.BuildSegmentDocuments(merged, subtitle.DocumentOptions{
		Index:       p.cfg.Index,
		SourceType:  p.cfg.SourceType,
		SourceID:    record.ID,
		ParentID:    record.Media.ID,
		Locale:      record.Track.Locale,
		Version:     p.cfg.MergeVersion,
		BaseURL:     record.Media.URL,
		ParentTitle: record.Media.Title,
		ParentURL:   record.Media.URL,
		ParentFields: map[string]any{
			"topic":     record.Media.Topic,
			"thumbnail": record.Media.Thumbnail,
		},
		Track: record.Track,
	})...)
	return docs, nil
}

func compact(values ...string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
