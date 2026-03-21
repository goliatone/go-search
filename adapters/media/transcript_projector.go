package media

import (
	"context"
	"strings"
	"time"

	"github.com/goliatone/go-search/indexing/subtitle"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

type MediaRecord struct {
	ID              string
	Title           string
	Summary         string
	URL             string
	Thumbnail       string
	Topic           string
	TopicPath       []string
	CategoryPath    []string
	People          []string
	Subjects        []string
	Texts           []string
	Deities         []string
	Location        string
	Sangha          string
	Format          string
	Series          string
	DurationSeconds int
	PublishedAt     *time.Time
	Badge           string
	Locale          string
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
	archiveFields := BuildArchiveProjection(record.Media, trackLocale)
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
			"topic":              compact(archiveFields.TopicLeaf),
			"topic_hierarchy":    append([]string(nil), archiveFields.TopicHierarchy...),
			"category":           compact(archiveFields.CategoryLeaf),
			"category_hierarchy": append([]string(nil), archiveFields.CategoryHierarchy...),
			"people":             append([]string(nil), archiveFields.People...),
			"subject":            append([]string(nil), archiveFields.Subjects...),
			"text":               append([]string(nil), archiveFields.Texts...),
			"deity":              append([]string(nil), archiveFields.Deities...),
			"locale":             compact(trackLocale),
			"decade":             compact(archiveFields.Decade),
			"duration_bucket":    compact(archiveFields.DurationBucket),
			"location":           compact(archiveFields.Location),
			"sangha":             compact(archiveFields.Sangha),
			"format":             compact(archiveFields.Format),
			"series":             compact(archiveFields.Series),
		},
		ParentFields: map[string]any{
			"topic":              archiveFields.TopicLeaf,
			"topic_hierarchy":    append([]string(nil), archiveFields.TopicHierarchy...),
			"category":           archiveFields.CategoryLeaf,
			"category_hierarchy": append([]string(nil), archiveFields.CategoryHierarchy...),
			"people":             append([]string(nil), archiveFields.People...),
			"subject":            append([]string(nil), archiveFields.Subjects...),
			"text":               append([]string(nil), archiveFields.Texts...),
			"deity":              append([]string(nil), archiveFields.Deities...),
			"location":           archiveFields.Location,
			"sangha":             archiveFields.Sangha,
			"format":             archiveFields.Format,
			"series":             archiveFields.Series,
			"decade":             archiveFields.Decade,
			"duration_bucket":    archiveFields.DurationBucket,
			"published_year":     archiveFields.PublishedYear,
			"result_badge":       archiveFields.Badge,
			"parent_title":       record.Media.Title,
			"parent_summary":     record.Media.Summary,
			"parent_url":         record.Media.URL,
			"parent_thumbnail":   record.Media.Thumbnail,
		},
		ParentNumeric: map[string]float64{
			"published_year":   float64(archiveFields.PublishedYear),
			"duration_seconds": float64(archiveFields.DurationSeconds),
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
