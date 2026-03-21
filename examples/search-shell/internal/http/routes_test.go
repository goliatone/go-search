package apphttp

import (
	"net/url"
	"testing"

	"github.com/goliatone/go-search/adapters/media"
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
