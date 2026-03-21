package apphttp

import (
	"net/url"
	"strings"
	"testing"

	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/pkg/types"
)

func ptrInt(value int) *int { return &value }

func TestNormalizeTopicsMergesLegacyAndCSVValues(t *testing.T) {
	got := normalizeTopics("architecture", []string{"ui, architecture", "semantic", ""})
	want := []string{"architecture", "ui", "semantic"}
	if len(got) != len(want) {
		t.Fatalf("topic count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("topic[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildSearchURLPreservesStateAndEscapesValues(t *testing.T) {
	view := searchView{
		Query:              "a&b",
		Locale:             "en",
		AcceptLanguage:     "fr-CA,fr;q=0.9,en;q=0.8",
		Topics:             []string{"architecture", "ui"},
		LandingSlug:        "architecture",
		PublishedYearGTE:   ptrInt(2020),
		PublishedYearLTE:   ptrInt(2024),
		DurationSecondsGTE: ptrInt(900),
		Group:              false,
		SortField:          media.FieldPublishedYear,
		SortDir:            "desc",
		PerPage:            10,
	}

	raw := buildSearchURL("/demo/search", view, 2)
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Path != "/demo/search" {
		t.Fatalf("path = %q", parsed.Path)
	}
	params := parsed.Query()
	if params.Get("q") != "a&b" {
		t.Fatalf("q = %q", params.Get("q"))
	}
	if params.Get("accept_language") != "fr-CA,fr;q=0.9,en;q=0.8" {
		t.Fatalf("accept_language = %q", params.Get("accept_language"))
	}
	if params.Get("topics") != "architecture,ui" {
		t.Fatalf("topics = %q", params.Get("topics"))
	}
	if params.Get("landing_slug") != "architecture" {
		t.Fatalf("landing_slug = %q", params.Get("landing_slug"))
	}
	if params.Get("published_year_gte") != "2020" || params.Get("published_year_lte") != "2024" {
		t.Fatalf("published year range = %q/%q", params.Get("published_year_gte"), params.Get("published_year_lte"))
	}
	if params.Get("duration_seconds_gte") != "900" {
		t.Fatalf("duration_seconds_gte = %q", params.Get("duration_seconds_gte"))
	}
	if params.Get("group") != "false" {
		t.Fatalf("group = %q", params.Get("group"))
	}
	if params.Get("sort") != media.FieldPublishedYear || params.Get("sort_dir") != "desc" {
		t.Fatalf("sort params = %q/%q", params.Get("sort"), params.Get("sort_dir"))
	}
	if params.Get("page") != "2" {
		t.Fatalf("page = %q", params.Get("page"))
	}
}

func TestNormalizeSortSupportsNumericArchiveFields(t *testing.T) {
	field, dir := normalizeSort(media.FieldPublishedYear, "desc")
	if field != media.FieldPublishedYear || dir != "desc" {
		t.Fatalf("published year sort = %q/%q", field, dir)
	}
	field, dir = normalizeSort(media.FieldDurationSeconds, "asc")
	if field != media.FieldDurationSeconds || dir != "asc" {
		t.Fatalf("duration sort = %q/%q", field, dir)
	}
}

func TestSnippetHTMLOnlyAllowsMarkTags(t *testing.T) {
	got := string(snippetHTML(&types.SearchSnippet{
		Highlighted: `before <mark>match</mark><script>alert("xss")</script><img src=x onerror=alert(1)>`,
	}))
	if !strings.Contains(got, "<mark>match</mark>") {
		t.Fatalf("expected mark tags to survive sanitization, got %q", got)
	}
	if strings.Contains(got, "<script>") || strings.Contains(got, "<img") {
		t.Fatalf("expected unsafe tags to be escaped, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;") {
		t.Fatalf("expected script payload to be escaped, got %q", got)
	}
}

func TestResolveAcceptLanguagePrefersExplicitOverride(t *testing.T) {
	if got := resolveAcceptLanguage("es-419,es;q=0.9", "fr-CA,fr;q=0.9,en;q=0.8"); got != "es-419,es;q=0.9" {
		t.Fatalf("override = %q", got)
	}
	if got := resolveAcceptLanguage("", "fr-CA,fr;q=0.9,en;q=0.8"); got != "fr-CA,fr;q=0.9,en;q=0.8" {
		t.Fatalf("header fallback = %q", got)
	}
}

func TestFormatTimestampUsesHumanFriendlyClockFormat(t *testing.T) {
	cases := []struct {
		name string
		ms   int64
		want string
	}{
		{name: "seconds", ms: 5000, want: "0:05"},
		{name: "fractional seconds", ms: 5600, want: "0:05.6"},
		{name: "minutes", ms: 125000, want: "2:05"},
		{name: "hours", ms: 3723000, want: "1:02:03"},
		{name: "negative clamped", ms: -1, want: "0:00"},
	}

	for _, tc := range cases {
		if got := formatTimestamp(tc.ms); got != tc.want {
			t.Fatalf("%s: formatTimestamp(%d) = %q, want %q", tc.name, tc.ms, got, tc.want)
		}
	}
}

func TestFormatTimestampRangeUsesFormattedBounds(t *testing.T) {
	got := formatTimestampRange(&types.MediaAnchor{StartMS: 5600, EndMS: 128000})
	if got != "0:05.6 - 2:08" {
		t.Fatalf("formatTimestampRange() = %q", got)
	}
}
