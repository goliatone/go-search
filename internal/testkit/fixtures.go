package testkit

import (
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/pkg/types"
)

func SampleTranscriptRecord() media.TranscriptRecord {
	return media.TranscriptRecord{
		ID: "track-1",
		Media: media.MediaRecord{
			ID:        "video-1",
			Title:     "Ocean Wind",
			Summary:   "Archive video about coastal chants",
			URL:       "https://example.org/videos/ocean-wind",
			Thumbnail: "https://example.org/thumbs/ocean-wind.jpg",
			Topic:     "archive",
			Locale:    "en",
		},
		Track: types.TranscriptTrack{
			MediaID:      "video-1",
			Locale:       "en",
			SourceFormat: "srt",
			TrackKind:    "translation",
			SourceLocale: "bo",
		},
		Format: "srt",
		Content: `1
00:00:01,000 --> 00:00:02,500
ocean wind

2
00:00:03,000 --> 00:00:04,500
chanting prayer
`,
	}
}
