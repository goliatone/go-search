package subtitle

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/goliatone/go-search/pkg/types"
)

type MergeConfig struct {
	MaxCharacters int
	MaxGapMS      int64
	Version       string
}

func MergeCues(cues []Cue, cfg MergeConfig) []Cue {
	if cfg.MaxCharacters <= 0 {
		cfg.MaxCharacters = 280
	}
	if cfg.MaxGapMS <= 0 {
		cfg.MaxGapMS = 1500
	}
	if len(cues) == 0 {
		return nil
	}
	current := cues[0]
	out := []Cue{}
	for _, cue := range cues[1:] {
		mergedText := strings.TrimSpace(current.Text + " " + cue.Text)
		if cue.Start-current.End <= cfg.MaxGapMS && len(mergedText) <= cfg.MaxCharacters {
			current.End = cue.End
			current.Text = mergedText
			continue
		}
		out = append(out, current)
		current = cue
	}
	out = append(out, current)
	return out
}

func SegmentDocumentID(sourceType, sourceID, locale, version string, cue Cue) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		sourceType,
		sourceID,
		locale,
		version,
		strconv.FormatInt(cue.Start, 10),
		strconv.FormatInt(cue.End, 10),
	}, ":")))
	return hex.EncodeToString(sum[:])
}

func AnchorForCue(parentID string, cue Cue, baseURL string) types.MediaAnchor {
	return types.MediaAnchor{
		ParentID: parentID,
		StartMS:  cue.Start,
		EndMS:    cue.End,
		URL:      baseURL,
		Label:    cue.Text,
	}
}
