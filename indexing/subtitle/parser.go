package subtitle

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

type Cue struct {
	Index int
	Start int64
	End   int64
	Text  string
}

func Parse(format string, data string) ([]Cue, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "srt":
		return ParseSRT(data)
	case "vtt", "webvtt":
		return ParseVTT(data)
	default:
		return nil, fmt.Errorf("unsupported subtitle format %q", format)
	}
}

func ParseSRT(data string) ([]Cue, error) {
	blocks := splitBlocks(data)
	out := make([]Cue, 0, len(blocks))
	for _, block := range blocks {
		lines := nonEmptyLines(block)
		if len(lines) < 2 {
			continue
		}
		index, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
		start, end, err := parseRange(lines[1], ",")
		if err != nil {
			return nil, err
		}
		out = append(out, Cue{
			Index: index,
			Start: start,
			End:   end,
			Text:  strings.Join(lines[2:], " "),
		})
	}
	return out, nil
}

func ParseVTT(data string) ([]Cue, error) {
	blocks := splitBlocks(data)
	out := make([]Cue, 0, len(blocks))
	for _, block := range blocks {
		lines := nonEmptyLines(block)
		if len(lines) == 0 || strings.EqualFold(strings.TrimSpace(lines[0]), "WEBVTT") {
			continue
		}
		offset := 0
		if !strings.Contains(lines[0], "-->") {
			offset = 1
		}
		if len(lines) <= offset {
			continue
		}
		start, end, err := parseRange(lines[offset], ".")
		if err != nil {
			return nil, err
		}
		out = append(out, Cue{
			Index: len(out) + 1,
			Start: start,
			End:   end,
			Text:  strings.Join(lines[offset+1:], " "),
		})
	}
	return out, nil
}

func splitBlocks(data string) []string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	return strings.Split(data, "\n\n")
}

func nonEmptyLines(block string) []string {
	scanner := bufio.NewScanner(strings.NewReader(block))
	out := []string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func parseRange(line string, millisecondSeparator string) (int64, int64, error) {
	parts := strings.Split(line, "-->")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid subtitle range %q", line)
	}
	start, err := parseTimestamp(parts[0], millisecondSeparator)
	if err != nil {
		return 0, 0, err
	}
	end, err := parseTimestamp(parts[1], millisecondSeparator)
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseTimestamp(value, millisecondSeparator string) (int64, error) {
	value = strings.TrimSpace(strings.Fields(value)[0])
	parts := strings.Split(value, millisecondSeparator)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	clock := strings.Split(parts[0], ":")
	if len(clock) != 3 {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	hours, _ := strconv.ParseInt(clock[0], 10, 64)
	minutes, _ := strconv.ParseInt(clock[1], 10, 64)
	seconds, _ := strconv.ParseInt(clock[2], 10, 64)
	millis, _ := strconv.ParseInt(parts[1], 10, 64)
	return (((hours*60)+minutes)*60+seconds)*1000 + millis, nil
}
