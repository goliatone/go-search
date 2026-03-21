package media

import (
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

type ArchiveProjection struct {
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

type LandingPreset struct {
	Slug        string
	Title       string
	Breadcrumb  string
	FacetFilter map[string][]string
}

func BuildArchiveProjection(record MediaRecord, trackLocale string) ArchiveProjection {
	defaults := inferredArchiveMetadata(record)
	topicPath := normalizePath(record.TopicPath)
	if len(topicPath) == 0 && strings.TrimSpace(record.Topic) != "" {
		topicPath = pathFromHierarchy(defaults.TopicHierarchy)
	}
	if len(topicPath) == 0 && strings.TrimSpace(record.Topic) != "" {
		topicPath = []string{"Teaching Topics", strings.TrimSpace(record.Topic)}
	}
	categoryPath := normalizePath(record.CategoryPath)
	if len(categoryPath) == 0 {
		categoryPath = pathFromHierarchy(defaults.CategoryHierarchy)
	}
	projection := ArchiveProjection{
		TopicHierarchy:    HierarchicalFacetValues(topicPath),
		CategoryHierarchy: HierarchicalFacetValues(categoryPath),
		People:            firstNonEmptySlice(compactStrings(record.People), defaults.People),
		Subjects:          firstNonEmptySlice(compactStrings(record.Subjects), defaults.Subjects),
		Texts:             firstNonEmptySlice(compactStrings(record.Texts), defaults.Texts),
		Deities:           firstNonEmptySlice(compactStrings(record.Deities), defaults.Deities),
		Location:          firstNonEmpty(strings.TrimSpace(record.Location), defaults.Location),
		Sangha:            firstNonEmpty(strings.TrimSpace(record.Sangha), defaults.Sangha),
		Format:            firstNonEmpty(strings.TrimSpace(record.Format), defaults.Format),
		Series:            firstNonEmpty(strings.TrimSpace(record.Series), defaults.Series),
		DurationSeconds:   max(record.DurationSeconds, defaults.DurationSeconds),
		PublishedYear:     max(publishedYear(record.PublishedAt), defaults.PublishedYear),
		Badge:             firstNonEmpty(strings.TrimSpace(record.Badge), strings.TrimSpace(record.Format), defaults.Badge, trackLocale),
	}
	if len(topicPath) > 0 {
		projection.TopicLeaf = topicPath[len(topicPath)-1]
	}
	if len(categoryPath) > 0 {
		projection.CategoryLeaf = categoryPath[len(categoryPath)-1]
	}
	if projection.DurationSeconds > 0 {
		projection.DurationBucket = DurationBucket(projection.DurationSeconds)
	}
	if projection.PublishedYear > 0 {
		projection.Decade = DecadeBucket(projection.PublishedYear)
	}
	return projection
}

func inferredArchiveMetadata(record MediaRecord) ArchiveProjection {
	switch strings.ToLower(strings.TrimSpace(record.Topic)) {
	case "architecture":
		return ArchiveProjection{
			TopicHierarchy:    HierarchicalFacetValues([]string{"Teaching Topics", "Architecture"}),
			CategoryHierarchy: HierarchicalFacetValues([]string{"Teaching Categories", "Commentary"}),
			People:            []string{"Codex Team"},
			Subjects:          []string{"Search Architecture"},
			Texts:             []string{"Search Blueprint"},
			Location:          "Boulder",
			Sangha:            "Archive Engineering",
			Format:            "Teaching",
			Series:            "Search V1",
			DurationSeconds:   1800,
			PublishedYear:     2024,
			Badge:             "Blueprint",
		}
	case "localization":
		return ArchiveProjection{
			TopicHierarchy:    HierarchicalFacetValues([]string{"Teaching Topics", "Localization"}),
			CategoryHierarchy: HierarchicalFacetValues([]string{"Teaching Categories", "Commentary"}),
			People:            []string{"Localization Team"},
			Subjects:          []string{"Locale Planning"},
			Texts:             []string{"Locale Search Matrix"},
			Location:          "Madrid",
			Sangha:            "Translation Sangha",
			Format:            "Teaching",
			Series:            "Locale Planning",
			DurationSeconds:   2400,
			PublishedYear:     2023,
			Badge:             "Locale",
		}
	case "ranking":
		return ArchiveProjection{
			TopicHierarchy:    HierarchicalFacetValues([]string{"Teaching Topics", "Ranking"}),
			CategoryHierarchy: HierarchicalFacetValues([]string{"Teaching Categories", "Empowerment"}),
			People:            []string{"Editorial Team"},
			Subjects:          []string{"Editorial Ranking"},
			Texts:             []string{"Ranking Playbook"},
			Location:          "New York",
			Sangha:            "Editorial Sangha",
			Format:            "Workshop",
			Series:            "Search Operations",
			DurationSeconds:   3300,
			PublishedYear:     2022,
			Badge:             "Editorial",
		}
	case "indexing":
		return ArchiveProjection{
			TopicHierarchy:    HierarchicalFacetValues([]string{"Teaching Topics", "Indexing"}),
			CategoryHierarchy: HierarchicalFacetValues([]string{"Teaching Categories", "Commentary"}),
			People:            []string{"Indexing Team"},
			Subjects:          []string{"Document Projection"},
			Texts:             []string{"Indexer Registry"},
			Location:          "Portland",
			Sangha:            "Systems Sangha",
			Format:            "Seminar",
			Series:            "Search V1",
			DurationSeconds:   2700,
			PublishedYear:     2021,
			Badge:             "Indexing",
		}
	default:
		return ArchiveProjection{
			CategoryHierarchy: HierarchicalFacetValues([]string{"Teaching Categories", "Teaching"}),
			People:            []string{"Archive Team"},
			Subjects:          []string{"Archive Search"},
			Texts:             []string{"Transcript Archive"},
			Location:          "Online",
			Sangha:            "Archive Sangha",
			Format:            "Teaching",
			Series:            "Archive Library",
			DurationSeconds:   1500,
			PublishedYear:     2020,
			Badge:             "Archive",
		}
	}
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
	switch {
	case seconds <= 0:
		return ""
	case seconds <= 300:
		return "0-5 min"
	case seconds <= 900:
		return "5-15 min"
	case seconds <= 1800:
		return "15-30 min"
	case seconds <= 3600:
		return "30-60 min"
	default:
		return "60+ min"
	}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptySlice(values []string, fallback []string) []string {
	if len(values) > 0 {
		return values
	}
	return append([]string(nil), fallback...)
}

func pathFromHierarchy(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return types.SplitFacetPath(values[len(values)-1], types.DefaultFacetPathSeparator)
}
