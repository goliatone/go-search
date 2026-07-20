package media

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

const (
	FacetFieldTopic             = "topic"
	FacetFieldTopicHierarchy    = "topic_hierarchy"
	FacetFieldCategory          = "category"
	FacetFieldCategoryHierarchy = "category_hierarchy"
	FacetFieldPeople            = "people"
	FacetFieldSubject           = "subject"
	FacetFieldText              = "text"
	FacetFieldDeity             = "deity"
	FacetFieldLocale            = "locale"
	FacetFieldDecade            = "decade"
	FacetFieldDurationBucket    = "duration_bucket"
	FacetFieldLocation          = "location"
	FacetFieldSangha            = "sangha"
	FacetFieldFormat            = "format"
	FacetFieldSeries            = "series"
	FieldPublishedYear          = "published_year"
	FieldDurationSeconds        = "duration_seconds"
	FieldResultBadge            = "result_badge"
)

type MediaProjection struct {
	TopicLeaf         string
	TopicHierarchy    []string
	CategoryLeaf      string
	CategoryHierarchy []string
	People            []string
	Subjects          []string
	Texts             []string
	Deities           []string
	Location          string
	Sangha            string
	Format            string
	Series            string
	DurationSeconds   int
	DurationBucket    string
	PublishedYear     int
	Decade            string
	Badge             string
}

// ArchiveProjection is retained as a source-compatible alias for one release line.
// Deprecated: use MediaProjection and BuildMediaProjection.
type ArchiveProjection = MediaProjection

type DurationBucketRange struct {
	Key        string `json:"key"`
	MinSeconds int    `json:"min_seconds"`
	MaxSeconds *int   `json:"max_seconds,omitempty"`
}

type DurationBucketPolicy struct {
	Buckets []DurationBucketRange `json:"buckets"`
}

func (p DurationBucketPolicy) Validate() error {
	if len(p.Buckets) == 0 {
		return fmt.Errorf("duration bucket policy requires at least one bucket")
	}
	expectedMin := 0
	seenKeys := map[string]struct{}{}
	for index, bucket := range p.Buckets {
		key := strings.TrimSpace(bucket.Key)
		if key == "" {
			return fmt.Errorf("duration bucket %d requires a key", index)
		}
		if _, exists := seenKeys[key]; exists {
			return fmt.Errorf("duration bucket key %q is duplicated", key)
		}
		seenKeys[key] = struct{}{}
		if bucket.MinSeconds != expectedMin {
			return fmt.Errorf("duration bucket %q must start at %d", bucket.Key, expectedMin)
		}
		if bucket.MaxSeconds == nil {
			if index != len(p.Buckets)-1 {
				return fmt.Errorf("only the final duration bucket may omit max_seconds")
			}
			continue
		}
		if *bucket.MaxSeconds <= bucket.MinSeconds {
			return fmt.Errorf("duration bucket %q must have increasing bounds", bucket.Key)
		}
		expectedMin = *bucket.MaxSeconds
	}
	if p.Buckets[len(p.Buckets)-1].MaxSeconds != nil {
		return fmt.Errorf("final duration bucket must be unbounded")
	}
	return nil
}

func DefaultDurationBucketPolicy() DurationBucketPolicy {
	return DurationBucketPolicy{Buckets: []DurationBucketRange{
		{Key: "0-5 min", MinSeconds: 0, MaxSeconds: new(301)},
		{Key: "5-15 min", MinSeconds: 301, MaxSeconds: new(901)},
		{Key: "15-30 min", MinSeconds: 901, MaxSeconds: new(1801)},
		{Key: "30-60 min", MinSeconds: 1801, MaxSeconds: new(3601)},
		{Key: "60+ min", MinSeconds: 3601},
	}}
}

type LandingPreset struct {
	Slug        string
	Title       string
	Breadcrumb  string
	FacetFilter map[string][]string
}

func BuildMediaProjection(record MediaRecord, trackLocale string, policy DurationBucketPolicy) (MediaProjection, error) {
	if len(policy.Buckets) == 0 {
		policy = DefaultDurationBucketPolicy()
	}
	if err := policy.Validate(); err != nil {
		return MediaProjection{}, err
	}
	topicPath := normalizePath(record.TopicPath)
	if len(topicPath) == 0 && strings.TrimSpace(record.Topic) != "" {
		topicPath = []string{strings.TrimSpace(record.Topic)}
	}
	categoryPath := normalizePath(record.CategoryPath)
	projection := MediaProjection{
		TopicHierarchy:    HierarchicalFacetValues(topicPath),
		CategoryHierarchy: HierarchicalFacetValues(categoryPath),
		People:            compactStrings(record.People),
		Subjects:          compactStrings(record.Subjects),
		Texts:             compactStrings(record.Texts),
		Deities:           compactStrings(record.Deities),
		Location:          strings.TrimSpace(record.Location),
		Sangha:            strings.TrimSpace(record.Sangha),
		Format:            strings.TrimSpace(record.Format),
		Series:            strings.TrimSpace(record.Series),
		DurationSeconds:   max(record.DurationSeconds, 0),
		PublishedYear:     publishedYear(record.PublishedAt),
		Badge:             strings.TrimSpace(record.Badge),
	}
	_ = trackLocale // locale is caller data but does not synthesize projection facts.
	if len(topicPath) > 0 {
		projection.TopicLeaf = topicPath[len(topicPath)-1]
	}
	if len(categoryPath) > 0 {
		projection.CategoryLeaf = categoryPath[len(categoryPath)-1]
	}
	if projection.DurationSeconds > 0 {
		projection.DurationBucket, _ = DurationBucketWithPolicy(projection.DurationSeconds, policy)
	}
	if projection.PublishedYear > 0 {
		projection.Decade = DecadeBucket(projection.PublishedYear)
	}
	return projection, nil
}

// BuildArchiveProjection is a deprecated, non-fabricating compatibility wrapper.
// Deprecated: use BuildMediaProjection.
func BuildArchiveProjection(record MediaRecord, trackLocale string) ArchiveProjection {
	projection, _ := BuildMediaProjection(record, trackLocale, DefaultDurationBucketPolicy())
	return projection
}

func HierarchicalFacetValues(path []string) []string {
	path = normalizePath(path)
	if len(path) == 0 {
		return nil
	}
	out := make([]string, 0, len(path))
	for i := 1; i <= len(path); i++ {
		out = append(out, types.JoinFacetPath(path[:i], types.DefaultFacetPathSeparator))
	}
	return out
}

func DurationBucket(seconds int) string {
	value, _ := DurationBucketWithPolicy(seconds, DefaultDurationBucketPolicy())
	return value
}

func DurationBucketWithPolicy(seconds int, policy DurationBucketPolicy) (string, error) {
	if err := policy.Validate(); err != nil {
		return "", err
	}
	if seconds <= 0 {
		return "", nil
	}
	for _, bucket := range policy.Buckets {
		if seconds >= bucket.MinSeconds && (bucket.MaxSeconds == nil || seconds < *bucket.MaxSeconds) {
			return bucket.Key, nil
		}
	}
	return "", nil
}

func DecadeBucket(year int) string {
	if year <= 0 {
		return ""
	}
	decade := (year / 10) * 10
	return strconv.Itoa(decade) + "s"
}

func DefaultArchiveFacetRequests() []types.FacetRequest {
	return []types.FacetRequest{
		{Field: FacetFieldTopicHierarchy, Limit: 24, Kind: types.FacetKindHierarchical, Disjunctive: true},
		{Field: FacetFieldCategoryHierarchy, Limit: 24, Kind: types.FacetKindHierarchical, Disjunctive: true},
		{Field: FacetFieldPeople, Limit: 12, Disjunctive: true},
		{Field: FacetFieldSubject, Limit: 12, Disjunctive: true},
		{Field: FacetFieldText, Limit: 12, Disjunctive: true},
		{Field: FacetFieldDeity, Limit: 12, Disjunctive: true},
		{Field: FacetFieldLocale, Limit: 12, Disjunctive: true},
		{Field: FacetFieldDecade, Limit: 12, Disjunctive: true},
		{Field: FacetFieldDurationBucket, Limit: 12, Disjunctive: true},
		{Field: FacetFieldLocation, Limit: 12, Disjunctive: true},
		{Field: FacetFieldSangha, Limit: 12, Disjunctive: true},
		{Field: FacetFieldFormat, Limit: 12, Disjunctive: true},
		{Field: FacetFieldSeries, Limit: 12, Disjunctive: true},
	}
}

func TopicLandingPreset(slug string) (LandingPreset, bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	presets := map[string]LandingPreset{
		"tara": {
			Slug:       "tara",
			Title:      "Tara",
			Breadcrumb: "Back to Teaching Topics",
			FacetFilter: map[string][]string{
				FacetFieldTopicHierarchy: {types.JoinFacetPath([]string{"Teaching Topics", "Tara"}, types.DefaultFacetPathSeparator)},
			},
		},
		"architecture": {
			Slug:       "architecture",
			Title:      "Architecture",
			Breadcrumb: "Back to Teaching Topics",
			FacetFilter: map[string][]string{
				FacetFieldTopicHierarchy: {types.JoinFacetPath([]string{"Teaching Topics", "Architecture"}, types.DefaultFacetPathSeparator)},
			},
		},
		"localization": {
			Slug:       "localization",
			Title:      "Localization",
			Breadcrumb: "Back to Teaching Topics",
			FacetFilter: map[string][]string{
				FacetFieldTopicHierarchy: {types.JoinFacetPath([]string{"Teaching Topics", "Localization"}, types.DefaultFacetPathSeparator)},
			},
		},
	}
	preset, ok := presets[slug]
	return preset, ok
}

func normalizePath(path []string) []string {
	out := make([]string, 0, len(path))
	for _, item := range path {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}

func publishedYear(value any) int {
	switch v := value.(type) {
	case *time.Time:
		if v == nil {
			return 0
		}
		return v.Year()
	case interface{ Year() int }:
		return v.Year()
	default:
		return 0
	}
}
