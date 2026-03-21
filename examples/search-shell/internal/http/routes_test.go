package apphttp

import (
	"net/url"
	"testing"
)

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
		Query:     "a&b",
		Locale:    "en",
		Topics:    []string{"architecture", "ui"},
		Group:     false,
		SortField: "title",
		SortDir:   "desc",
		PerPage:   10,
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
	if params.Get("group") != "false" {
		t.Fatalf("group = %q", params.Get("group"))
	}
	if params.Get("sort") != "title" || params.Get("sort_dir") != "desc" {
		t.Fatalf("sort params = %q/%q", params.Get("sort"), params.Get("sort_dir"))
	}
	if params.Get("page") != "2" {
		t.Fatalf("page = %q", params.Get("page"))
	}
}
