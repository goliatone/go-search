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
	ID      string
	Media   MediaRecord
	Track   types.TranscriptTrack
	Content string
	Format  string
	// Units selects the caller-normalized input path when non-nil. Content and
	// Format remain the backwards-compatible SRT/VTT input path.
	Units          []subtitle.NormalizedUnit
	Metadata       map[string]any
	ResultID       string
	ResultType     string
	MatchLocation  string
	MatchField     string
	ParentType     string
	Visibility     types.Visibility
	ParentMetadata map[string]any
}

type TranscriptProjectorConfig struct {
	Index           string
	SourceType      string
	MergeVersion    string
	MaxChars        int
	MaxGapMS        int64
	DurationBuckets DurationBucketPolicy
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
	mediaFields, err := BuildMediaProjection(record.Media, trackLocale, p.cfg.DurationBuckets)
	if err != nil {
		return nil, err
	}
	mergeConfig := subtitle.MergeConfig{
		MaxCharacters: p.cfg.MaxChars,
		MaxGapMS:      p.cfg.MaxGapMS,
		Version:       p.cfg.MergeVersion,
	}
	parentFields := map[string]any{
		"topic":              mediaFields.TopicLeaf,
		"topic_hierarchy":    append([]string(nil), mediaFields.TopicHierarchy...),
		"category":           mediaFields.CategoryLeaf,
		"category_hierarchy": append([]string(nil), mediaFields.CategoryHierarchy...),
		"people":             append([]string(nil), mediaFields.People...),
		"subject":            append([]string(nil), mediaFields.Subjects...),
		"text":               append([]string(nil), mediaFields.Texts...),
		"deity":              append([]string(nil), mediaFields.Deities...),
		"location":           mediaFields.Location,
		"sangha":             mediaFields.Sangha,
		"format":             mediaFields.Format,
		"series":             mediaFields.Series,
		"decade":             mediaFields.Decade,
		"duration_bucket":    mediaFields.DurationBucket,
		"result_badge":       mediaFields.Badge,
		"parent_title":       record.Media.Title,
		"parent_summary":     record.Media.Summary,
		"parent_url":         record.Media.URL,
		"parent_thumbnail":   record.Media.Thumbnail,
	}
	parentNumeric := map[string]float64{}
	if mediaFields.PublishedYear > 0 {
		parentFields["published_year"] = mediaFields.PublishedYear
		parentNumeric["published_year"] = float64(mediaFields.PublishedYear)
	}
	if mediaFields.DurationSeconds > 0 {
		parentNumeric["duration_seconds"] = float64(mediaFields.DurationSeconds)
	}
	documentOptions := subtitle.DocumentOptions{
		Index:           p.cfg.Index,
		SourceType:      p.cfg.SourceType,
		SourceID:        record.ID,
		ParentID:        record.Media.ID,
		ParentType:      record.ParentType,
		ResultID:        record.ResultID,
		ResultType:      record.ResultType,
		MatchLocation:   record.MatchLocation,
		MatchField:      record.MatchField,
		Locale:          trackLocale,
		Version:         p.cfg.MergeVersion,
		BaseURL:         record.Media.URL,
		ParentTitle:     record.Media.Title,
		ParentSummary:   record.Media.Summary,
		ParentURL:       record.Media.URL,
		ParentThumbnail: record.Media.Thumbnail,
		ParentFacets: map[string][]string{
			"topic":              compact(mediaFields.TopicLeaf),
			"topic_hierarchy":    append([]string(nil), mediaFields.TopicHierarchy...),
			"category":           compact(mediaFields.CategoryLeaf),
			"category_hierarchy": append([]string(nil), mediaFields.CategoryHierarchy...),
			"people":             append([]string(nil), mediaFields.People...),
			"subject":            append([]string(nil), mediaFields.Subjects...),
			"text":               append([]string(nil), mediaFields.Texts...),
			"deity":              append([]string(nil), mediaFields.Deities...),
			"locale":             compact(trackLocale),
			"decade":             compact(mediaFields.Decade),
			"duration_bucket":    compact(mediaFields.DurationBucket),
			"location":           compact(mediaFields.Location),
			"sangha":             compact(mediaFields.Sangha),
			"format":             compact(mediaFields.Format),
			"series":             compact(mediaFields.Series),
		},
		ParentFields:   parentFields,
		ParentNumeric:  parentNumeric,
		ParentMetadata: record.ParentMetadata,
		Metadata:       record.Metadata,
		Visibility:     record.Visibility,
		Track:          record.Track,
	}
	if record.Units != nil {
		return subtitle.BuildUnitDocuments(record.Units, mergeConfig, documentOptions)
	}
	cues, err := subtitle.Parse(record.Format, record.Content)
	if err != nil {
		return nil, err
	}
	return subtitle.BuildSegmentDocuments(subtitle.MergeCues(cues, mergeConfig), documentOptions), nil
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
