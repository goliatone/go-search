package media

import (
	"context"
	"strings"

	"github.com/goliatone/go-search/indexing/subtitle"
	"github.com/goliatone/go-search/locale"
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
	defaults := subtitle.DefaultMergeConfig()
	if cfg.MergeVersion == "" {
		cfg.MergeVersion = defaults.Version
	}
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = defaults.MaxCharacters
	}
	if cfg.MaxGapMS <= 0 {
		cfg.MaxGapMS = defaults.MaxGapMS
	}
	return &TranscriptProjector{cfg: cfg}
}

func (p *TranscriptProjector) Project(_ context.Context, record TranscriptRecord) ([]types.Document, error) {
	trackLocale := locale.Normalize(record.Track.Locale)
	cues, err := subtitle.Parse(record.Format, record.Content)
	if err != nil {
		return nil, err
	}
	merged := subtitle.MergeCues(cues, subtitle.MergeConfig{
		MaxCharacters: p.cfg.MaxChars,
		MaxGapMS:      p.cfg.MaxGapMS,
		Version:       p.cfg.MergeVersion,
	})
	return subtitle.BuildSegmentDocuments(merged, subtitle.DocumentOptions{
		Index:           p.cfg.Index,
		SourceType:      p.cfg.SourceType,
		SourceID:        record.ID,
		ParentID:        record.Media.ID,
		Locale:          trackLocale,
		Version:         p.cfg.MergeVersion,
		BaseURL:         record.Media.URL,
		ParentTitle:     record.Media.Title,
		ParentSummary:   record.Media.Summary,
		ParentURL:       record.Media.URL,
		ParentThumbnail: record.Media.Thumbnail,
		ParentFacets: map[string][]string{
			"topic":  compact(record.Media.Topic),
			"locale": compact(trackLocale),
		},
		ParentFields: map[string]any{
			"topic":            record.Media.Topic,
			"parent_title":     record.Media.Title,
			"parent_summary":   record.Media.Summary,
			"parent_url":       record.Media.URL,
			"parent_thumbnail": record.Media.Thumbnail,
		},
		Track: record.Track,
	}), nil
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
