package apphttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	commandregistry "github.com/goliatone/go-command/registry"
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/examples/search-shell/internal/config"
	"github.com/goliatone/go-search/examples/search-shell/internal/core"
	"github.com/goliatone/go-search/examples/search-shell/internal/searchdemo"
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
		Surface:            "content_split",
		Locale:             "en",
		CacheDisabled:      true,
		AcceptLanguage:     "fr-CA,fr;q=0.9,en;q=0.8",
		Topics:             []string{"architecture", "ui"},
		LandingSlug:        "architecture",
		PublishedYearGTE:   ptrInt(2020),
		PublishedYearLTE:   ptrInt(2024),
		DurationSecondsGTE: ptrInt(900),
		TenantID:           "tenant-1",
		OrgID:              "org-1",
		ActorUserID:        "user-1",
		ActorRole:          "support",
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
	if params.Get("surface") != "content_split" {
		t.Fatalf("surface = %q", params.Get("surface"))
	}
	if params.Get("cache_disabled") != "true" {
		t.Fatalf("cache_disabled = %q", params.Get("cache_disabled"))
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
	if params.Get("tenant_id") != "tenant-1" || params.Get("org_id") != "org-1" {
		t.Fatalf("scope params = %q/%q", params.Get("tenant_id"), params.Get("org_id"))
	}
	if params.Get("actor_user_id") != "user-1" || params.Get("actor_role") != "support" {
		t.Fatalf("actor params = %q/%q", params.Get("actor_user_id"), params.Get("actor_role"))
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

func TestSearchTemplateRendersExplicitSurfaceModesAndMixedContentTypes(t *testing.T) {
	view := searchView{
		Title:         "Search",
		ActionPath:    "/demo/search",
		APIPath:       "/api/demo/search",
		SuggestPath:   "/api/demo/suggest",
		Surface:       "content_shared",
		CacheDisabled: true,
		Page:          1,
		PerPage:       10,
		TotalPages:    1,
		Result: types.SearchResultPage{
			Page:    1,
			PerPage: 10,
			Total:   3,
			Hits: []types.SearchHit{
				{ID: "video-1", Type: types.DocumentTypeVideo, Title: "Search Architecture Walkthrough", URL: "/videos/1", Locale: "en"},
				{ID: "document-1", Type: types.DocumentTypeDocument, Title: "Search Rollout Workbook", URL: "/documents/1", Locale: "en"},
				{ID: "blog-1", Type: types.DocumentTypeBlogArticle, Title: "Search Notes", URL: "/blog/1", Locale: "en"},
			},
		},
	}
	var out bytes.Buffer
	if err := searchTemplate.Execute(&out, view); err != nil {
		t.Fatalf("render template: %v", err)
	}
	body := out.String()
	for _, needle := range []string{
		`value="content_shared" selected`,
		`value="content_split"`,
		`value="media_grouped"`,
		`value="users"`,
		`Media transcripts`,
		`name="cache_disabled" value="true"`,
		`badge badge-type">video<`,
		`badge badge-type">document<`,
		`badge badge-type">blog_article<`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in rendered template", needle)
		}
	}
	if strings.Contains(body, `surface === 'media_grouped' ? 'true'`) {
		t.Fatalf("expected grouping toggle to be independent from the selected media surface")
	}
	if !strings.Contains(body, `const group = document.getElementById('filterGroup').value;`) {
		t.Fatalf("expected template to submit the explicit group filter value")
	}
}

func TestOpsTemplateRendersSmokeFlows(t *testing.T) {
	view := opsView{
		BaseURL:    "http://localhost:8484",
		StatusJSON: "{}",
		RulesJSON:  "[]",
		SmokeFlows: []searchdemo.SmokeFlow{{
			ID:          "cache_invalidation",
			Label:       "Cache invalidation after rebuild",
			Method:      "POST+GET",
			Path:        "/api/demo/reindex then repeat /api/demo/search?surface=content_shared&locale=en&q=search",
			Description: "Confirms generation-based cache invalidation after indexed writes or rebuilds.",
		}},
	}
	var out bytes.Buffer
	if err := opsTemplate.Execute(&out, view); err != nil {
		t.Fatalf("render ops template: %v", err)
	}
	body := out.String()
	for _, needle := range []string{
		"Smoke Flows",
		"generation-based cache invalidation",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected %q in rendered ops template", needle)
		}
	}
}

func TestDemoAPIsRoundTripCacheDisabledFlag(t *testing.T) {
	commandregistry.WithTestRegistry(func() {
		cfg := config.Defaults()
		appCore, err := core.New(context.Background(), &cfg)
		if err != nil {
			t.Fatalf("new core: %v", err)
		}
		t.Cleanup(func() {
			_ = appCore.Shutdown(context.Background())
		})
		if err := Register(appCore); err != nil {
			t.Fatalf("register routes: %v", err)
		}

		for _, tc := range []struct {
			name string
			url  string
		}{
			{name: "search", url: "/api/demo/search?q=search&cache_disabled=true"},
			{name: "suggest", url: "/api/demo/suggest?q=search&cache_disabled=true"},
		} {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			resp, err := appCore.Fiber.Test(req)
			if err != nil {
				t.Fatalf("%s request: %v", tc.name, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("%s status = %d", tc.name, resp.StatusCode)
			}
			var body struct {
				Request map[string]any `json:"request"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("%s decode: %v", tc.name, err)
			}
			if body.Request["cache_disabled"] != true {
				t.Fatalf("%s cache_disabled = %#v", tc.name, body.Request["cache_disabled"])
			}
		}
	})
}

func TestEditorialRoutesCanBeDisabledByConfig(t *testing.T) {
	commandregistry.WithTestRegistry(func() {
		cfg := config.Defaults()
		cfg.SearchDemo.EditorialEnabled = false

		appCore, err := core.New(context.Background(), &cfg)
		if err != nil {
			t.Fatalf("new core: %v", err)
		}
		t.Cleanup(func() {
			_ = appCore.Shutdown(context.Background())
		})
		if err := Register(appCore); err != nil {
			t.Fatalf("register routes: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/demo/editorial", nil)
		resp, err := appCore.Fiber.Test(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})
}
