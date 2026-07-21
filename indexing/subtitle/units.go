package subtitle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goliatone/go-search/locale"
)

// NormalizedUnit is caller-normalized transcript text. ID must be unique within
// one projection and Order values must be non-negative and strictly increasing.
// StartMS and EndMS must either both be nil or both be set.
type NormalizedUnit struct {
	ID      string // Stable identity within the transcript source and locale.
	Order   int    // Stable source order; gaps are allowed.
	Text    string
	StartMS *int64
	EndMS   *int64
}

type normalizedSegment struct {
	identityParts []string
	text          string
	startMS       *int64
	endMS         *int64
}

func mergeUnits(units []NormalizedUnit, cfg MergeConfig) ([]normalizedSegment, error) {
	cfg = normalizeMergeConfig(cfg)
	fragments, err := normalizeUnitFragments(units, cfg.MaxCharacters)
	if err != nil {
		return nil, err
	}
	if len(fragments) == 0 {
		return nil, nil
	}

	current := fragments[0]
	out := make([]normalizedSegment, 0, len(fragments))
	for _, fragment := range fragments[1:] {
		mergedText := strings.TrimSpace(current.text + " " + fragment.text)
		if canMergeUnitSegments(current, fragment, mergedText, cfg) {
			current.text = mergedText
			current.identityParts = append(current.identityParts, fragment.identityParts...)
			if current.endMS != nil && fragment.endMS != nil && *fragment.endMS > *current.endMS {
				end := *fragment.endMS
				current.endMS = &end
			}
			continue
		}
		out = append(out, current)
		current = fragment
	}
	out = append(out, current)
	return out, nil
}

func normalizeUnitFragments(units []NormalizedUnit, maxCharacters int) ([]normalizedSegment, error) {
	if len(units) == 0 {
		return nil, nil
	}
	seenIDs := make(map[string]struct{}, len(units))
	fragments := make([]normalizedSegment, 0, len(units))
	previousOrder := -1
	var previousTimedStart *int64
	for index, unit := range units {
		if unit.Order < 0 {
			return nil, fmt.Errorf("subtitle unit %d has negative order %d", index, unit.Order)
		}
		if index > 0 && unit.Order <= previousOrder {
			return nil, fmt.Errorf("subtitle unit %d order %d must be greater than %d", index, unit.Order, previousOrder)
		}
		previousOrder = unit.Order

		text := strings.TrimSpace(unit.Text)
		if text == "" {
			continue
		}
		id := strings.TrimSpace(unit.ID)
		if id == "" {
			return nil, fmt.Errorf("subtitle unit %d has empty identity", index)
		}
		if _, exists := seenIDs[id]; exists {
			return nil, fmt.Errorf("subtitle unit %d repeats identity %q", index, id)
		}
		seenIDs[id] = struct{}{}

		startMS, endMS, err := normalizeUnitTiming(index, unit.StartMS, unit.EndMS)
		if err != nil {
			return nil, err
		}
		if startMS != nil {
			if previousTimedStart != nil && *startMS < *previousTimedStart {
				return nil, fmt.Errorf("subtitle unit %d starts at %dms before the previous timed unit at %dms", index, *startMS, *previousTimedStart)
			}
			start := *startMS
			previousTimedStart = &start
		}

		parts := splitUnitText(text, maxCharacters)
		for partIndex, part := range parts {
			identity := hashIdentityParts(
				id,
				strconv.Itoa(unit.Order),
				strconv.Itoa(partIndex),
				timingIdentity(startMS, endMS),
				part,
			)
			fragments = append(fragments, normalizedSegment{
				identityParts: []string{identity},
				text:          part,
				startMS:       cloneInt64(startMS),
				endMS:         cloneInt64(endMS),
			})
		}
	}
	return fragments, nil
}

func normalizeUnitTiming(index int, startMS, endMS *int64) (*int64, *int64, error) {
	if (startMS == nil) != (endMS == nil) {
		return nil, nil, fmt.Errorf("subtitle unit %d must provide both start and end timing", index)
	}
	if startMS == nil {
		return nil, nil, nil
	}
	if *startMS < 0 {
		return nil, nil, fmt.Errorf("subtitle unit %d has negative start time %dms", index, *startMS)
	}
	if *endMS <= *startMS {
		return nil, nil, fmt.Errorf("subtitle unit %d end time %dms must be greater than start time %dms", index, *endMS, *startMS)
	}
	return cloneInt64(startMS), cloneInt64(endMS), nil
}

func splitUnitText(text string, maxCharacters int) []string {
	remaining := []rune(strings.TrimSpace(text))
	if len(remaining) == 0 {
		return nil
	}
	if maxCharacters <= 0 {
		maxCharacters = DefaultMergeConfig().MaxCharacters
	}
	out := make([]string, 0, (len(remaining)+maxCharacters-1)/maxCharacters)
	for len(remaining) > maxCharacters {
		cut := maxCharacters
		for index := maxCharacters - 1; index > 0; index-- {
			if unicode.IsSpace(remaining[index]) {
				cut = index
				break
			}
		}
		part := strings.TrimSpace(string(remaining[:cut]))
		if part != "" {
			out = append(out, part)
		}
		remaining = []rune(strings.TrimSpace(string(remaining[cut:])))
	}
	if part := strings.TrimSpace(string(remaining)); part != "" {
		out = append(out, part)
	}
	return out
}

func canMergeUnitSegments(current, next normalizedSegment, mergedText string, cfg MergeConfig) bool {
	if utf8.RuneCountInString(mergedText) > cfg.MaxCharacters {
		return false
	}
	currentTimed := current.startMS != nil && current.endMS != nil
	nextTimed := next.startMS != nil && next.endMS != nil
	if currentTimed != nextTimed {
		return false
	}
	if !currentTimed {
		return true
	}
	return *next.startMS-*current.endMS <= cfg.MaxGapMS
}

func normalizedSegmentDocumentID(sourceType, sourceID, localeCode, version string, segment normalizedSegment) string {
	parts := []string{
		strings.TrimSpace(sourceType),
		strings.TrimSpace(sourceID),
		locale.Normalize(localeCode),
		strings.TrimSpace(version),
		hashIdentityParts(segment.identityParts...),
		segment.text,
		timingIdentity(segment.startMS, segment.endMS),
	}
	return hashIdentityParts(parts...)
}

func timingIdentity(startMS, endMS *int64) string {
	if startMS == nil || endMS == nil {
		return "untimed"
	}
	return strconv.FormatInt(*startMS, 10) + ":" + strconv.FormatInt(*endMS, 10)
}

func hashIdentityParts(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(strconv.Itoa(len(part))))
		_, _ = hash.Write([]byte{':'})
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
