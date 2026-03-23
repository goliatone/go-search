package testkit

import (
	"github.com/goliatone/go-search/adapters/content"
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/pkg/types"
)

type IndexedDocuments struct {
	Index string
	Docs  []types.Document
}

func ArchiveFacetDocuments(index string) []types.Document {
	startA, endA := int64(1000), int64(2000)
	startB, endB := int64(3000), int64(4000)
	startC, endC := int64(5000), int64(6000)
	return []types.Document{
		{
			ID:         "segment-architecture-1",
			Index:      index,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-architecture",
			SourceType: "transcript",
			SourceID:   "track-architecture",
			Title:      "Architecture Walkthrough",
			Body:       "archive architecture prayer",
			URL:        "https://example.org/video-architecture",
			AnchorURL:  "https://example.org/video-architecture#t=1",
			Locale:     "en",
			StartMS:    &startA,
			EndMS:      &endA,
			Fields: map[string]any{
				"parent_title":           "Architecture Walkthrough",
				"parent_url":             "https://example.org/video-architecture",
				media.FieldResultBadge:   "Blueprint",
				media.FieldPublishedYear: 2025,
			},
			Facets: map[string][]string{
				media.FacetFieldTopic:             {"architecture"},
				media.FacetFieldTopicHierarchy:    {"Teaching Topics", "Teaching Topics > Architecture"},
				media.FacetFieldCategoryHierarchy: {"Teaching Categories", "Teaching Categories > Workshop"},
				media.FacetFieldDurationBucket:    {"30-60 min"},
				media.FacetFieldDecade:            {"2020s"},
				media.FacetFieldFormat:            {"Teaching"},
				media.FacetFieldLocale:            {"en"},
				media.FacetFieldLocation:          {"Mexico City"},
				media.FacetFieldSangha:            {"Cloud Sangha"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear:   2025,
				media.FieldDurationSeconds: 2400,
			},
		},
		{
			ID:         "segment-tara-1",
			Index:      index,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-tara",
			SourceType: "transcript",
			SourceID:   "track-tara",
			Title:      "Tara Teachings",
			Body:       "archive tara prayer",
			URL:        "https://example.org/video-tara",
			AnchorURL:  "https://example.org/video-tara#t=3",
			Locale:     "en",
			StartMS:    &startB,
			EndMS:      &endB,
			Fields: map[string]any{
				"parent_title":           "Tara Teachings",
				"parent_url":             "https://example.org/video-tara",
				media.FieldResultBadge:   "Featured",
				media.FieldPublishedYear: 2024,
			},
			Facets: map[string][]string{
				media.FacetFieldTopic:             {"tara"},
				media.FacetFieldTopicHierarchy:    {"Teaching Topics", "Teaching Topics > Tara"},
				media.FacetFieldCategoryHierarchy: {"Teaching Categories", "Teaching Categories > Workshop"},
				media.FacetFieldDurationBucket:    {"30-60 min"},
				media.FacetFieldDecade:            {"2020s"},
				media.FacetFieldFormat:            {"Teaching"},
				media.FacetFieldLocale:            {"en"},
				media.FacetFieldLocation:          {"Mexico City"},
				media.FacetFieldSangha:            {"Cloud Sangha"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear:   2024,
				media.FieldDurationSeconds: 2100,
			},
		},
		{
			ID:         "segment-ranking-1",
			Index:      index,
			Type:       types.DocumentTypeTranscriptSegment,
			ParentID:   "video-ranking",
			SourceType: "transcript",
			SourceID:   "track-ranking",
			Title:      "Ranking Workshop",
			Body:       "archive ranking prayer",
			URL:        "https://example.org/video-ranking",
			AnchorURL:  "https://example.org/video-ranking#t=5",
			Locale:     "en",
			StartMS:    &startC,
			EndMS:      &endC,
			Fields: map[string]any{
				"parent_title":           "Ranking Workshop",
				"parent_url":             "https://example.org/video-ranking",
				media.FieldPublishedYear: 2022,
			},
			Facets: map[string][]string{
				media.FacetFieldTopic:             {"ranking"},
				media.FacetFieldTopicHierarchy:    {"Teaching Topics", "Teaching Topics > Ranking"},
				media.FacetFieldCategoryHierarchy: {"Teaching Categories", "Teaching Categories > Lecture"},
				media.FacetFieldDurationBucket:    {"0-15 min"},
				media.FacetFieldDecade:            {"2020s"},
				media.FacetFieldFormat:            {"Workshop"},
				media.FacetFieldLocale:            {"en"},
				media.FacetFieldLocation:          {"Berkeley"},
				media.FacetFieldSangha:            {"Open Sangha"},
			},
			Numeric: map[string]float64{
				media.FieldPublishedYear:   2022,
				media.FieldDurationSeconds: 900,
			},
		},
	}
}

func HeterogeneousDocuments() []IndexedDocuments {
	return []IndexedDocuments{
		{
			Index: "videos",
			Docs: []types.Document{{
				ID:      "video-1",
				Index:   "videos",
				Type:    types.DocumentTypeVideo,
				Title:   "Search Architecture Walkthrough",
				Summary: "video summary",
				Body:    "search architecture video",
				URL:     "/videos/1",
				Locale:  "en",
				Facets:  map[string][]string{"entity_type": {types.DocumentTypeVideo}},
			}},
		},
		{
			Index: "documents",
			Docs: []types.Document{{
				ID:      "document-1",
				Index:   "documents",
				Type:    types.DocumentTypeDocument,
				Title:   "Search Rollout Workbook",
				Summary: "document summary",
				Body:    "search rollout document",
				URL:     "/documents/1",
				Locale:  "en",
				Facets:  map[string][]string{"entity_type": {types.DocumentTypeDocument}},
			}},
		},
		{
			Index: "blog_articles",
			Docs: []types.Document{{
				ID:      "blog-1",
				Index:   "blog_articles",
				Type:    types.DocumentTypeBlogArticle,
				Title:   "Search Notes",
				Summary: "blog summary",
				Body:    "search notes article",
				URL:     "/blog/1",
				Locale:  "en",
				Facets:  map[string][]string{"entity_type": {types.DocumentTypeBlogArticle}},
			}},
		},
	}
}

func SharedIndexContentRecords() []content.Record {
	return []content.Record{
		{ID: "video-1", Type: types.DocumentTypeVideo, Title: "Search Architecture Walkthrough", Body: "search architecture video", URL: "/videos/1", Locale: "en"},
		{ID: "document-1", Type: types.DocumentTypeDocument, Title: "Search Rollout Workbook", Body: "search rollout document", URL: "/documents/1", Locale: "en"},
		{ID: "blog-1", Type: types.DocumentTypeBlogArticle, Title: "Search Notes", Body: "search notes article", URL: "/blog/1", Locale: "en"},
	}
}
