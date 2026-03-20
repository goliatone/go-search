package subtitle

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
)

type MergeConfig struct {
	MaxCharacters int
	MaxGapMS      int64
	Version       string
}

func DefaultMergeConfig() MergeConfig {
	return MergeConfig{
		MaxCharacters: 280,
		MaxGapMS:      1500,
		Version:       "v1",
	}
}

func MergeCues(cues []Cue, cfg MergeConfig) []Cue {
	cfg = normalizeMergeConfig(cfg)
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

func normalizeMergeConfig(cfg MergeConfig) MergeConfig {
	out := DefaultMergeConfig()
	if cfg.MaxCharacters > 0 {
		out.MaxCharacters = cfg.MaxCharacters
	}
	if cfg.MaxGapMS > 0 {
		out.MaxGapMS = cfg.MaxGapMS
	}
	if cfg.Version != "" {
		out.Version = cfg.Version
	}
	return out
}

func SegmentDocumentID(sourceType, sourceID, localeCode, version string, cue Cue) string {
	localeCode = locale.Normalize(localeCode)
	sum := sha1.Sum([]byte(strings.Join([]string{
		sourceType,
		sourceID,
		localeCode,
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
		URL:      fmt.Sprintf("%s#t=%d", strings.TrimSpace(baseURL), cue.Start/1000),
		Label:    cue.Text,
	}
}
