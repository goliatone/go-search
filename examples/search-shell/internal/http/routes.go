package apphttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	quicksite "github.com/goliatone/go-admin/quickstart/site"
	"github.com/goliatone/go-router"
	"github.com/goliatone/go-search/adapters/media"
	"github.com/goliatone/go-search/examples/search-shell/internal/core"
	"github.com/goliatone/go-search/examples/search-shell/internal/searchdemo"
	"github.com/goliatone/go-search/pkg/types"
	userstypes "github.com/goliatone/go-users/pkg/types"
)

var templateFuncs = template.FuncMap{
	"json": prettyJSON,
	"add": func(a, b int) int {
		return a + b
	},
	"eq": func(a, b string) bool {
		return a == b
	},
	"mul": func(a, b int) int {
		return a * b
	},
	"contains": func(values []string, value string) bool {
		for _, item := range values {
			if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(value)) {
				return true
			}
		}
		return false
	},
	"join": func(values []string, sep string) string {
		return strings.Join(values, sep)
	},
	"intValue": func(value *int) string {
		if value == nil {
			return ""
		}
		return strconv.Itoa(*value)
	},
	"formatTimestamp":      formatTimestamp,
	"formatTimestampRange": formatTimestampRange,
	"snippetParts":         snippetParts,
	"facetTitle": func(field string) string {
		switch strings.TrimSpace(field) {
		case media.FacetFieldTopicHierarchy:
			return "Topic"
		case media.FacetFieldCategoryHierarchy:
			return "Category"
		case media.FacetFieldPeople:
			return "People"
		case media.FacetFieldSubject:
			return "Subject"
		case media.FacetFieldText:
			return "Text"
		case media.FacetFieldDeity:
			return "Deity"
		case media.FacetFieldLocale:
			return "Locale"
		case media.FacetFieldDecade:
			return "Decade"
		case media.FacetFieldDurationBucket:
			return "Duration"
		case media.FacetFieldLocation:
			return "Location"
		case media.FacetFieldSangha:
			return "Sangha"
		case media.FacetFieldFormat:
			return "Format"
		case media.FacetFieldSeries:
			return "Series"
		default:
			return strings.ReplaceAll(strings.TrimSpace(field), "_", " ")
		}
	},
}

type snippetPart struct {
	Text string
	Mark bool
}

func snippetParts(snippet *types.SearchSnippet) []snippetPart {
	if snippet == nil {
		return nil
	}
	if strings.TrimSpace(snippet.Highlighted) != "" {
		return parseHighlightedSnippet(snippet.Highlighted)
	}
	return []snippetPart{{Text: snippet.Text}}
}

func parseHighlightedSnippet(raw string) []snippetPart {
	var parts []snippetPart
	marked := false
	for raw != "" {
		open := strings.Index(raw, "<mark>")
		close := strings.Index(raw, "</mark>")
		if open == -1 && close == -1 {
			parts = appendSnippetPart(parts, raw, marked)
			break
		}

		tag := ""
		var next int
		if open != -1 && (close == -1 || open < close) {
			tag = "<mark>"
			next = open
		} else {
			tag = "</mark>"
			next = close
		}

		parts = appendSnippetPart(parts, raw[:next], marked)
		marked = tag == "<mark>"
		raw = raw[next+len(tag):]
	}
	return parts
}

func appendSnippetPart(parts []snippetPart, text string, marked bool) []snippetPart {
	if text == "" {
		return parts
	}
	if len(parts) > 0 && parts[len(parts)-1].Mark == marked {
		parts[len(parts)-1].Text += text
		return parts
	}
	return append(parts, snippetPart{Text: text, Mark: marked})
}

var homeTemplate = template.Must(template.New("home").Funcs(templateFuncs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>{{.Name}}</title>
<style>
:root {
  --bg:#eef1e8;
  --card:#fffdf8;
  --ink:#1a241f;
  --muted:#667267;
  --border:#d7ded0;
  --accent:#0f766e;
  --accent-2:#7c2d12;
}
*{box-sizing:border-box}
body{margin:0;font:15px/1.55 ui-sans-serif,system-ui,sans-serif;background:linear-gradient(180deg,#edf2e7 0,#f8f4ea 100%);color:var(--ink)}
main{max-width:1180px;margin:24px auto;padding:0 16px}
.card{background:var(--card);border:1px solid var(--border);border-radius:16px;padding:18px 20px;margin-bottom:14px}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:14px}
@media (max-width:960px){.grid{grid-template-columns:1fr}}
h1,h2,h3{margin:0 0 10px 0}
h1{font-size:30px}
h2{font-size:18px}
p{margin:6px 0;color:var(--muted)}
code{font-size:13px;background:#edf3ea;padding:2px 6px;border-radius:5px}
a{color:var(--accent);text-decoration:none}
a:hover{text-decoration:underline}
pre{margin:8px 0 0;background:#132019;color:#dff8eb;padding:12px;border-radius:12px;overflow:auto}
ul{margin:8px 0 0 18px;padding:0}
li{margin:5px 0}
.badge{display:inline-block;padding:2px 8px;border-radius:999px;font-size:12px;background:#dcfce7;color:#14532d}
</style>
</head>
<body>
<main>
  <section class="card">
    <h1>{{.Name}}</h1>
    <p>Environment: <code>{{.Env}}</code> | Listening on <code>{{.Address}}</code></p>
    <p>Admin base path: <code>{{.AdminBasePath}}</code> | Search provider: <code>{{.Provider}}</code></p>
    <p>Config path: <code>{{.ConfigPath}}</code> | Started at: <code>{{.StartedAt}}</code></p>
  </section>

  <section class="grid">
    <section class="card">
      <h2>Quick Links</h2>
      <ul>
        {{range .Links}}
        <li><a href="{{.Href}}">{{.Label}}</a> <code>{{.Href}}</code></li>
        {{end}}
      </ul>
    </section>
    <section class="card">
      <h2>Runtime Summary</h2>
      <p>Index: <code>{{.Search.IndexName}}</code> | Documents: <span class="badge">{{.Search.Documents}}</span></p>
      <p>Generation: <code>{{.Search.Generation}}</code> | Editorial rules: <code>{{.Search.EditorialRules}}</code></p>
      <pre>{{.SearchJSON}}</pre>
    </section>
  </section>

  <section class="grid">
    {{if .EditorialEnabled}}
    <section class="card">
      <h2>Editorial Snapshot</h2>
      <pre>{{.RulesJSON}}</pre>
    </section>
    {{else}}
    <section class="card">
      <h2>Editorial Snapshot</h2>
      <p>Editorial UI and API exposure are disabled by configuration.</p>
      <pre>{{.RulesJSON}}</pre>
    </section>
    {{end}}
  </section>
</main>
</body>
</html>
`))

var searchTemplate = template.Must(template.New("search").Funcs(templateFuncs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>{{.Title}}</title>
<style>
:root {
  --bg:#f8f9fa;
  --card:#fff;
  --ink:#1a1d21;
  --muted:#6c757d;
  --border:#dee2e6;
  --accent:#0d6efd;
  --accent-hover:#0b5ed7;
  --success:#198754;
  --warning:#ffc107;
}
*{box-sizing:border-box}
body{margin:0;font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:var(--bg);color:var(--ink)}

/* Header */
.header{background:var(--card);border-bottom:1px solid var(--border);padding:12px 0;position:sticky;top:0;z-index:100}
.header-inner{max-width:1280px;margin:0 auto;padding:0 20px;display:flex;align-items:center;gap:20px}
.logo{font-size:20px;font-weight:700;color:var(--ink);text-decoration:none}
.search-box{flex:1;max-width:600px;position:relative}
.search-input{width:100%;padding:10px 16px;padding-right:40px;border:2px solid var(--border);border-radius:24px;font-size:15px;transition:border-color .15s}
.search-input:focus{outline:none;border-color:var(--accent)}
.search-btn{position:absolute;right:4px;top:50%;transform:translateY(-50%);background:var(--accent);color:#fff;border:0;padding:8px 16px;border-radius:20px;cursor:pointer;font-size:14px}
.search-btn:hover{background:var(--accent-hover)}
.nav-links{display:flex;gap:16px}
.nav-links a{color:var(--muted);text-decoration:none;font-size:14px}
.nav-links a:hover{color:var(--ink)}

/* Autocomplete */
.suggestions{position:absolute;top:100%;left:0;right:0;background:var(--card);border:1px solid var(--border);border-radius:12px;margin-top:4px;box-shadow:0 4px 12px rgba(0,0,0,.1);display:none;max-height:300px;overflow:auto}
.suggestions.active{display:block}
.suggestion-item{padding:10px 16px;cursor:pointer;border-bottom:1px solid var(--border)}
.suggestion-item:last-child{border-bottom:0}
.suggestion-item:hover{background:#f8f9fa}
.suggestion-title{font-weight:500}
.suggestion-meta{font-size:12px;color:var(--muted)}

/* Layout */
.container{max-width:1280px;margin:0 auto;padding:20px}
.layout{display:grid;grid-template-columns:280px 1fr;gap:24px}
@media (max-width:900px){.layout{grid-template-columns:1fr}}

/* Sidebar */
.sidebar{display:flex;flex-direction:column;gap:16px}
.filter-card{background:var(--card);border:1px solid var(--border);border-radius:12px;padding:16px}
.filter-title{font-size:14px;font-weight:600;margin-bottom:12px;display:flex;justify-content:space-between;align-items:center}
.filter-title button{background:none;border:0;color:var(--accent);font-size:12px;cursor:pointer}
.filter-group{margin-bottom:16px}
.filter-group:last-child{margin-bottom:0}
.filter-label{font-size:12px;color:var(--muted);margin-bottom:6px;display:block}
.filter-select,.filter-input{width:100%;padding:8px 12px;border:1px solid var(--border);border-radius:8px;font-size:14px;background:#fff}
.facet-list{max-height:none}
.facet-item{display:flex;align-items:center;gap:8px;padding:6px 0;font-size:14px}
.facet-item input[type="checkbox"]{width:16px;height:16px;accent-color:var(--accent)}
.facet-count{margin-left:auto;background:#e9ecef;padding:2px 8px;border-radius:10px;font-size:12px;color:var(--muted)}
.clear-filters{display:block;width:100%;padding:10px;background:#f8f9fa;border:1px solid var(--border);border-radius:8px;color:var(--muted);cursor:pointer;font-size:14px;margin-top:8px}
.clear-filters:hover{background:#e9ecef;color:var(--ink)}

/* Results */
.results-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px;flex-wrap:wrap;gap:12px}
.results-count{font-size:14px;color:var(--muted)}
.results-count strong{color:var(--ink)}
.sort-controls{display:flex;align-items:center;gap:8px}
.sort-controls label{font-size:13px;color:var(--muted)}
.sort-select{padding:6px 12px;border:1px solid var(--border);border-radius:6px;font-size:13px;background:#fff}

/* Result cards */
.result-card{background:var(--card);border:1px solid var(--border);border-radius:12px;padding:16px;margin-bottom:12px}
.result-card:hover{border-color:#adb5bd}
.result-header{display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:8px}
.result-title{font-size:16px;font-weight:600;color:var(--ink);text-decoration:none;margin:0}
.result-title a{color:inherit;text-decoration:none}
.result-title a:hover{color:var(--accent)}
.result-badges{display:flex;gap:6px;flex-wrap:wrap}
.badge{padding:3px 10px;border-radius:12px;font-size:11px;font-weight:500;text-transform:uppercase}
.badge-type{background:#e7f5ff;color:#1971c2}
.badge-locale{background:#fff3bf;color:#e67700}
.badge-score{background:#d3f9d8;color:#2f9e44}
.badge-archive{background:#ffe8cc;color:#d9480f}
.result-snippet{font-size:14px;color:var(--muted);margin:8px 0;line-height:1.5}
.result-snippet mark{background:#fff3bf;padding:1px 2px;border-radius:2px}
.result-meta{font-size:12px;color:var(--muted);display:flex;gap:16px;flex-wrap:wrap}
.result-meta a{color:var(--accent)}

/* Groups */
.group-card{background:var(--card);border:1px solid var(--border);border-radius:12px;margin-bottom:16px;overflow:hidden}
.group-header{background:#f8f9fa;padding:14px 16px;border-bottom:1px solid var(--border)}
.group-title{font-size:17px;font-weight:600;margin:0 0 4px 0}
.group-meta{font-size:13px;color:var(--muted)}
.group-hits{padding:8px}
.group-hit{padding:12px;border-bottom:1px solid #f1f3f4}
.group-hit:last-child{border-bottom:0}

/* Pagination */
.pagination{display:flex;justify-content:center;align-items:center;gap:8px;margin-top:24px;padding:16px 0}
.page-btn{padding:8px 16px;border:1px solid var(--border);border-radius:8px;background:var(--card);color:var(--ink);text-decoration:none;font-size:14px;cursor:pointer}
.page-btn:hover:not(:disabled){background:#f8f9fa;border-color:#adb5bd}
.page-btn:disabled{opacity:.5;cursor:not-allowed}
.page-btn.active{background:var(--accent);color:#fff;border-color:var(--accent)}
.page-info{font-size:14px;color:var(--muted);padding:0 12px}

/* Debug panel */
.debug-panel{background:var(--card);border:1px solid var(--border);border-radius:12px;margin-top:24px}
.debug-header{padding:12px 16px;border-bottom:1px solid var(--border);cursor:pointer;display:flex;justify-content:space-between;align-items:center}
.debug-header h3{margin:0;font-size:14px}
.debug-content{padding:16px;display:none}
.debug-content.active{display:block}
.debug-content pre{margin:0;background:#1e1e1e;color:#d4d4d4;padding:16px;border-radius:8px;overflow:auto;font-size:13px;max-height:400px}
.debug-tabs{display:flex;gap:8px;margin-bottom:12px}
.debug-tab{padding:6px 12px;border:1px solid var(--border);border-radius:6px;background:#f8f9fa;cursor:pointer;font-size:13px}
.debug-tab.active{background:var(--accent);color:#fff;border-color:var(--accent)}

/* Empty state */
.empty-state{text-align:center;padding:48px 24px;color:var(--muted)}
.empty-state h3{color:var(--ink);margin-bottom:8px}
</style>
</head>
<body>
<!-- Header with search -->
<header class="header">
  <div class="header-inner">
    <a href="/" class="logo">Search Demo</a>
    <form class="search-box" method="GET" action="{{.ActionPath}}" id="searchForm">
      <input type="text" class="search-input" id="searchInput" name="q" value="{{.Query}}" placeholder="Search archive content..." autocomplete="off" />
      <button type="submit" class="search-btn">Search</button>
      <div class="suggestions" id="suggestions"></div>
      <!-- Preserve other params -->
      <input type="hidden" name="surface" value="{{.Surface}}" />
      <input type="hidden" name="locale" value="{{.Locale}}" />
      {{if .CacheDisabled}}<input type="hidden" name="cache_disabled" value="true" />{{end}}
      <input type="hidden" name="accept_language" value="{{.AcceptLanguage}}" />
      <input type="hidden" name="group" value="{{if .Group}}true{{else}}false{{end}}" />
      {{if .LandingSlug}}<input type="hidden" name="landing_slug" value="{{.LandingSlug}}" />{{end}}
      {{if .Topics}}<input type="hidden" name="topics" value="{{join .Topics ","}}" />{{end}}
      {{range $field, $values := .FacetFilters}}{{if $values}}<input type="hidden" name="facet_{{$field}}" value="{{join $values ","}}" />{{end}}{{end}}
      {{if .PublishedYearGTE}}<input type="hidden" name="published_year_gte" value="{{intValue .PublishedYearGTE}}" />{{end}}
      {{if .PublishedYearLTE}}<input type="hidden" name="published_year_lte" value="{{intValue .PublishedYearLTE}}" />{{end}}
      {{if .DurationSecondsGTE}}<input type="hidden" name="duration_seconds_gte" value="{{intValue .DurationSecondsGTE}}" />{{end}}
      {{if .DurationSecondsLTE}}<input type="hidden" name="duration_seconds_lte" value="{{intValue .DurationSecondsLTE}}" />{{end}}
      {{if .SortField}}<input type="hidden" name="sort" value="{{.SortField}}" />{{end}}
      {{if .SortDir}}<input type="hidden" name="sort_dir" value="{{.SortDir}}" />{{end}}
    </form>
    <nav class="nav-links">
      <a href="/">Home</a>
      <a href="/demo/ops">Operations</a>
      <a href="{{.CurrentAPIURL}}">API</a>
    </nav>
  </div>
</header>

<div class="container">
  <div class="layout">
    <!-- Sidebar filters -->
    <aside class="sidebar">
      <div class="filter-card">
        {{if .LandingTitle}}
        <div class="filter-group">
          <span class="filter-label">{{.Breadcrumb}}</span>
          <strong>{{.LandingTitle}}</strong>
        </div>
        {{end}}
        <div class="filter-title">
          Filters
          <button type="button" onclick="clearFilters()">Clear all</button>
        </div>

        <!-- Facets from results -->
        {{range .Result.Facets}}
        {{if .Values}}
        <div class="filter-group">
          {{$field := .Field}}
          <span class="filter-label">{{facetTitle $field}}</span>
          <div class="facet-list">
            {{range .Values}}
            <label class="facet-item">
              <input type="checkbox" name="facet_{{$field}}" value="{{.Value}}" {{if eq $field "topic"}}{{if contains $.Topics .Value}}checked{{end}}{{else if eq $field "locale"}}{{if eq $.Locale .Value}}checked{{end}}{{else}}{{if contains (index $.FacetFilters $field) .Value}}checked{{end}}{{end}} onchange="applyFacet('{{$field}}', '{{.Value}}', this.checked)" />
              <span style="padding-left: {{mul .Level 14}}px">{{if .Label}}{{.Label}}{{else}}{{.Value}}{{end}}</span>
              <span class="facet-count">{{.Count}}</span>
            </label>
            {{end}}
          </div>
        </div>
        {{end}}
        {{end}}

        <div class="filter-group">
          <label class="filter-label" for="filterSurface">Surface</label>
          <select class="filter-select" id="filterSurface">
            <option value="content_shared" {{if eq .Surface "content_shared"}}selected{{end}}>Content shared index</option>
            <option value="content_split" {{if eq .Surface "content_split"}}selected{{end}}>Content split indexes</option>
            <option value="media_grouped" {{if eq .Surface "media_grouped"}}selected{{end}}>Media transcripts</option>
            <option value="users" {{if eq .Surface "users"}}selected{{end}}>Users inventory</option>
          </select>
        </div>

        <div class="filter-group">
          <label class="filter-label" for="filterLocale">Locale</label>
          <input type="text" class="filter-input" id="filterLocale" value="{{.Locale}}" placeholder="en" />
        </div>

        <div class="filter-group">
          <label class="facet-item" style="padding:0">
            <input type="checkbox" id="filterCacheDisabled" {{if .CacheDisabled}}checked{{end}} />
            <span>Bypass query cache for this request</span>
          </label>
        </div>

        <div class="filter-group">
          <label class="filter-label" for="filterAcceptLanguage">Accept-Language override</label>
          <input type="text" class="filter-input" id="filterAcceptLanguage" value="{{.AcceptLanguage}}" placeholder="fr-CA,fr;q=0.9,en;q=0.8" />
        </div>

        <div class="filter-group">
          <span class="filter-label">Locale binding</span>
          <span>Source: <strong>{{if .LocaleSource}}{{.LocaleSource}}{{else}}empty{{end}}</strong></span><br />
          <span>Supported: <strong>{{if .LocaleSupported}}yes{{else}}no{{end}}</strong></span>
        </div>

        <div class="filter-group">
          <label class="filter-label" for="filterTopic">Topic</label>
          <input type="text" class="filter-input" id="filterTopic" value="{{.TopicFilter}}" placeholder="architecture, ui" />
        </div>

        <div class="filter-group">
          <label class="filter-label" for="publishedYearGTE">Published year range</label>
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px">
            <input type="number" class="filter-input" id="publishedYearGTE" value="{{intValue .PublishedYearGTE}}" placeholder="From" />
            <input type="number" class="filter-input" id="publishedYearLTE" value="{{intValue .PublishedYearLTE}}" placeholder="To" />
          </div>
        </div>

        <div class="filter-group">
          <label class="filter-label" for="durationSecondsGTE">Duration seconds range</label>
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px">
            <input type="number" class="filter-input" id="durationSecondsGTE" value="{{intValue .DurationSecondsGTE}}" placeholder="Min seconds" />
            <input type="number" class="filter-input" id="durationSecondsLTE" value="{{intValue .DurationSecondsLTE}}" placeholder="Max seconds" />
          </div>
        </div>

        <div class="filter-group">
          <label class="filter-label" for="filterGroup">Group results</label>
          <select class="filter-select" id="filterGroup">
            <option value="true" {{if .Group}}selected{{end}}>Grouped by parent</option>
            <option value="false" {{if not .Group}}selected{{end}}>Individual hits</option>
          </select>
        </div>

        <button class="clear-filters" onclick="applyFilters()">Apply Filters</button>
      </div>
    </aside>

    <!-- Main results -->
    <main>
      <div class="results-header">
        <div class="results-count">
          {{if .Query}}
          {{if .Group}}
          <strong>{{.Result.Total}}</strong> grouped results for "<strong>{{.Query}}</strong>"
          {{else}}
          <strong>{{.Result.Total}}</strong> results for "<strong>{{.Query}}</strong>"
          {{end}}
          {{else}}
          {{if .Group}}
          <strong>{{.Result.Total}}</strong> grouped results
          {{else}}
          <strong>{{.Result.Total}}</strong> total results
          {{end}}
          {{end}}
          {{if .TopicFilter}} in topics <strong>{{.TopicFilter}}</strong>{{end}}
          {{if .Group}} with facet counts by parent result{{end}}
        </div>
        <div class="sort-controls">
          <label for="sortSelect">Sort by:</label>
          <select class="sort-select" id="sortSelect" onchange="applySort(this.value)">
            <option value="" {{if not .SortField}}selected{{end}}>Relevance</option>
            <option value="published_year:desc" {{if and (eq .SortField "published_year") (eq .SortDir "desc")}}selected{{end}}>Newest</option>
            <option value="published_year:asc" {{if and (eq .SortField "published_year") (eq .SortDir "asc")}}selected{{end}}>Oldest</option>
            <option value="title:asc" {{if and (eq .SortField "title") (eq .SortDir "asc")}}selected{{end}}>Title A-Z</option>
            <option value="title:desc" {{if and (eq .SortField "title") (eq .SortDir "desc")}}selected{{end}}>Title Z-A</option>
          </select>
        </div>
      </div>

      {{if .Result.Groups}}
      <!-- Grouped results -->
      {{range .Result.Groups}}
      <div class="group-card">
        <div class="group-header">
          <h3 class="group-title">{{if .Parent}}{{.Parent.Title}}{{else}}{{.Key}}{{end}}</h3>
          <div class="group-meta">
            <span>{{.Count}} matching segments in this parent result</span>
            {{if .Parent}} &bull; <a href="{{.Parent.URL}}">View media</a>{{end}}
          </div>
        </div>
        <div class="group-hits">
          {{range .Hits}}
          <div class="group-hit">
            <div class="result-header">
              <h4 class="result-title">{{.Title}}</h4>
              <div class="result-badges">
                {{with index .Fields "result_badge"}}<span class="badge badge-archive">{{.}}</span>{{end}}
                <span class="badge badge-type">{{.Type}}</span>
                <span class="badge badge-locale">{{.Locale}}</span>
                <span class="badge badge-score">{{printf "%.1f" .Score}}</span>
              </div>
            </div>
            {{if .Snippet}}<p class="result-snippet">{{range snippetParts .Snippet}}{{if .Mark}}<mark>{{.Text}}</mark>{{else}}{{.Text}}{{end}}{{end}}</p>{{end}}
            <div class="result-meta">
              {{if .Anchor}}<span>Timestamp: {{formatTimestampRange .Anchor}}</span>{{end}}
              {{if .Anchor}}<a href="{{.Anchor.URL}}">Jump to segment</a>{{end}}
            </div>
          </div>
          {{end}}
        </div>
      </div>
      {{end}}
      {{else if .Result.Hits}}
      <!-- Individual hits -->
      {{range .Result.Hits}}
      <div class="result-card">
        <div class="result-header">
          <h3 class="result-title"><a href="{{.URL}}">{{.Title}}</a></h3>
          <div class="result-badges">
            {{with index .Fields "result_badge"}}<span class="badge badge-archive">{{.}}</span>{{end}}
            <span class="badge badge-type">{{.Type}}</span>
            <span class="badge badge-locale">{{.Locale}}</span>
            <span class="badge badge-score">{{printf "%.1f" .Score}}</span>
          </div>
        </div>
        {{if .Snippet}}<p class="result-snippet">{{range snippetParts .Snippet}}{{if .Mark}}<mark>{{.Text}}</mark>{{else}}{{.Text}}{{end}}{{end}}</p>{{end}}
        <div class="result-meta">
          {{if .Parent}}<span>Parent: {{.Parent.Title}}</span>{{end}}
          {{if .Anchor}}<span>{{formatTimestampRange .Anchor}}</span>{{end}}
          {{with index .Fields "tenant_id"}}<span>Tenant: {{.}}</span>{{end}}
          {{with index .Fields "org_id"}}<span>Org: {{.}}</span>{{end}}
          {{with index .Fields "status"}}<span>Status: {{.}}</span>{{end}}
          {{with index .Fields "role"}}<span>Role: {{.}}</span>{{end}}
          {{if .Anchor}}<a href="{{.Anchor.URL}}">View</a>{{end}}
        </div>
      </div>
      {{end}}
      {{else}}
      <div class="empty-state">
        <h3>No results found</h3>
        <p>Try adjusting your search terms or filters</p>
      </div>
      {{end}}

      <!-- Pagination -->
      {{if gt .Result.Total .PerPage}}
      <div class="pagination">
        {{if .HasPrev}}
        <a href="{{.PrevURL}}" class="page-btn">Previous</a>
        {{else}}
        <button class="page-btn" disabled>Previous</button>
        {{end}}

        <span class="page-info">Page <strong>{{.Page}}</strong> of <strong>{{.TotalPages}}</strong></span>

        {{if .HasNext}}
        <a href="{{.NextURL}}" class="page-btn">Next</a>
        {{else}}
        <button class="page-btn" disabled>Next</button>
        {{end}}
      </div>
      {{end}}

      <!-- Debug panel -->
      <div class="debug-panel">
        <div class="debug-header" onclick="toggleDebug()">
          <h3>Debug Information</h3>
          <span id="debugToggle">Show</span>
        </div>
        <div class="debug-content" id="debugContent">
          <div class="debug-tabs">
            <button class="debug-tab active" onclick="showDebugTab('request')">Request</button>
            <button class="debug-tab" onclick="showDebugTab('response')">Response</button>
          </div>
          <div id="debugRequest">
            <pre>{{.RequestJSON}}</pre>
          </div>
          <div id="debugResponse" style="display:none">
            <pre>{{.ResultJSON}}</pre>
          </div>
        </div>
      </div>
    </main>
  </div>
</div>

<script>
// Autocomplete
const searchInput = document.getElementById('searchInput');
const suggestions = document.getElementById('suggestions');
let debounceTimer;

searchInput.addEventListener('input', function() {
  clearTimeout(debounceTimer);
  const query = this.value.trim();
  if (query.length < 2) {
    suggestions.classList.remove('active');
    return;
  }
  debounceTimer = setTimeout(() => fetchSuggestions(query), 200);
});

searchInput.addEventListener('blur', () => {
  setTimeout(() => suggestions.classList.remove('active'), 200);
});

async function fetchSuggestions(query) {
  try {
    const surface = document.getElementById('filterSurface')?.value || '{{.Surface}}';
    const locale = document.getElementById('filterLocale')?.value || '';
    const cacheDisabled = document.getElementById('filterCacheDisabled')?.checked ? 'true' : '';
    const acceptLanguage = document.getElementById('filterAcceptLanguage')?.value || '';
    const resp = await fetch('{{.SuggestPath}}?q=' + encodeURIComponent(query) + '&surface=' + encodeURIComponent(surface) + '&locale=' + encodeURIComponent(locale) + '&cache_disabled=' + encodeURIComponent(cacheDisabled) + '&accept_language=' + encodeURIComponent(acceptLanguage) + '&limit=5');
    const data = await resp.json();
    if (data.result && data.result.items && data.result.items.length > 0) {
      suggestions.innerHTML = data.result.items.map(item =>
        '<div class="suggestion-item" onclick="selectSuggestion(\'' + escapeHtml(item.title) + '\')">' +
        '<div class="suggestion-title">' + escapeHtml(item.title) + '</div>' +
        '<div class="suggestion-meta">' + escapeHtml(item.type || '') + ' &bull; ' + escapeHtml(item.locale || '') + '</div>' +
        '</div>'
      ).join('');
      suggestions.classList.add('active');
    } else {
      suggestions.classList.remove('active');
    }
  } catch (e) {
    console.error('Suggest error:', e);
  }
}

function selectSuggestion(title) {
  searchInput.value = title;
  suggestions.classList.remove('active');
  document.getElementById('searchForm').submit();
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

// Filters
function applyFilters() {
  const params = new URLSearchParams(window.location.search);
  params.set('q', searchInput.value || '');
  const surface = document.getElementById('filterSurface').value;
  params.set('surface', surface);

  const locale = document.getElementById('filterLocale').value.trim();
  if (locale) params.set('locale', locale);
  else params.delete('locale');

  if (document.getElementById('filterCacheDisabled').checked) params.set('cache_disabled', 'true');
  else params.delete('cache_disabled');

  const acceptLanguage = document.getElementById('filterAcceptLanguage').value.trim();
  if (acceptLanguage) params.set('accept_language', acceptLanguage);
  else params.delete('accept_language');

  const topics = document.getElementById('filterTopic').value
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
  params.delete('topic');
  if (topics.length > 0) params.set('topics', topics.join(','));
  else params.delete('topics');

  const group = document.getElementById('filterGroup').value;
  params.set('group', group);
  setOptionalParam(params, 'published_year_gte', document.getElementById('publishedYearGTE').value);
  setOptionalParam(params, 'published_year_lte', document.getElementById('publishedYearLTE').value);
  setOptionalParam(params, 'duration_seconds_gte', document.getElementById('durationSecondsGTE').value);
  setOptionalParam(params, 'duration_seconds_lte', document.getElementById('durationSecondsLTE').value);
  params.set('page', '1');

  window.location.href = '{{.ActionPath}}?' + params.toString();
}

function clearFilters() {
  const params = new URLSearchParams(window.location.search);
  params.set('q', searchInput.value || '');
  params.delete('locale');
  params.delete('cache_disabled');
  params.delete('accept_language');
  params.delete('topic');
  params.delete('topics');
  params.delete('published_year_gte');
  params.delete('published_year_lte');
  params.delete('duration_seconds_gte');
  params.delete('duration_seconds_lte');
  params.delete('tenant_id');
  params.delete('org_id');
  params.delete('actor_user_id');
  params.delete('actor_role');
  Array.from(params.keys())
    .filter(key => key.startsWith('facet_'))
    .forEach(key => params.delete(key));
  params.set('surface', 'content_shared');
  params.set('group', 'false');
  params.set('page', '1');
  window.location.href = '{{.ActionPath}}?' + params.toString();
}

function applyFacet(field, value, checked) {
  const params = new URLSearchParams(window.location.search);
  if (field === 'topic') {
    const topics = new Set(
      (params.get('topics') || params.get('topic') || '')
        .split(',')
        .map(item => item.trim())
        .filter(Boolean)
    );
    params.delete('topic');
    if (checked) topics.add(value);
    else topics.delete(value);
    if (topics.size > 0) params.set('topics', Array.from(topics).join(','));
    else params.delete('topics');
  } else {
    const key = 'facet_' + field;
    const values = new Set(
      (params.get(key) || '')
        .split(',')
        .map(item => item.trim())
        .filter(Boolean)
    );
    if (checked) values.add(value);
    else values.delete(value);
    if (values.size > 0) params.set(key, Array.from(values).join(','));
    else params.delete(key);
    if (field === 'locale') {
      if (checked) params.set('locale', value);
      else if ((params.get('locale') || '').toLowerCase() === value.toLowerCase()) params.delete('locale');
    }
  }
  params.set('page', '1');
  window.location.href = '{{.ActionPath}}?' + params.toString();
}

function applySort(value) {
  const params = new URLSearchParams(window.location.search);
  if (value) {
    const [field, dir] = value.split(':');
    params.set('sort', field);
    params.set('sort_dir', dir);
  } else {
    params.delete('sort');
    params.delete('sort_dir');
  }
  params.set('page', '1');
  window.location.href = '{{.ActionPath}}?' + params.toString();
}

function setOptionalParam(params, key, value) {
  const normalized = (value || '').trim();
  if (normalized) params.set(key, normalized);
  else params.delete(key);
}

// Debug panel
function toggleDebug() {
  const content = document.getElementById('debugContent');
  const toggle = document.getElementById('debugToggle');
  content.classList.toggle('active');
  toggle.textContent = content.classList.contains('active') ? 'Hide' : 'Show';
}

function showDebugTab(tab) {
  document.querySelectorAll('.debug-tab').forEach(el => el.classList.remove('active'));
  document.querySelector('.debug-tab[onclick*="' + tab + '"]').classList.add('active');
  document.getElementById('debugRequest').style.display = tab === 'request' ? 'block' : 'none';
  document.getElementById('debugResponse').style.display = tab === 'response' ? 'block' : 'none';
}
</script>
</body>
</html>
`))

var opsTemplate = template.Must(template.New("ops").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>Search Operations</title>
<style>
:root {
  --bg:#f2f4f8;
  --card:#ffffff;
  --ink:#18202b;
  --muted:#5f6b7a;
  --border:#d6dde8;
}
*{box-sizing:border-box}
body{margin:0;font:15px/1.55 ui-sans-serif,system-ui,sans-serif;background:linear-gradient(180deg,#eef3f8 0,#f8fafc 100%);color:var(--ink)}
main{max-width:1180px;margin:24px auto;padding:0 16px}
.card{background:var(--card);border:1px solid var(--border);border-radius:16px;padding:18px 20px;margin-bottom:14px}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:14px}
@media (max-width:960px){.grid{grid-template-columns:1fr}}
h1,h2{margin:0 0 10px 0}
code{font-size:13px;background:#eef2f7;padding:2px 6px;border-radius:5px}
pre{margin:8px 0 0;background:#111827;color:#dbeafe;padding:12px;border-radius:12px;overflow:auto}
ul{margin:8px 0 0 18px;padding:0}
li{margin:4px 0}
</style>
</head>
<body>
<main>
  <section class="card">
    <h1>Search Operations</h1>
    <ul>
      <li><code>POST /api/demo/ensure</code> re-ensures the index schema and registry.</li>
      <li><code>POST /api/demo/reindex</code> runs the indexer over the seeded transcript source.</li>
      {{if .EditorialEnabled}}
      <li><code>GET /api/demo/editorial</code> lists editorial rules.</li>
      <li><code>POST /api/demo/editorial/upsert</code> upserts a rule.</li>
      <li><code>POST /api/demo/editorial/enable</code>, <code>/disable</code>, <code>/delete</code> mutate a rule by id.</li>
      {{else}}
      <li><code>APP_SEARCH_DEMO__EDITORIAL_ENABLED=false</code> disables editorial UI and API exposure for rollback or reduced-surface releases.</li>
      {{end}}
    </ul>
  </section>

  <section class="grid">
    <section class="card">
      <h2>Status</h2>
      <pre>{{.StatusJSON}}</pre>
    </section>
    {{if .EditorialEnabled}}
    <section class="card">
      <h2>Editorial Rules</h2>
      <pre>{{.RulesJSON}}</pre>
    </section>
    {{else}}
    <section class="card">
      <h2>Editorial Rules</h2>
      <p>Editorial rule endpoints are disabled in this runtime.</p>
      <pre>{{.RulesJSON}}</pre>
    </section>
    {{end}}
  </section>

  <section class="card">
    <h2>Smoke Flows</h2>
    <ul>
      {{range .SmokeFlows}}
      <li><code>{{.Method}} {{.Path}}</code> {{.Description}}</li>
      {{end}}
    </ul>
  </section>

  <section class="grid">
    <section class="card">
      <h2>Reindex Example</h2>
      <pre>curl -X POST {{.BaseURL}}/api/demo/reindex -H 'Content-Type: application/json' -d '{"batch_size":25}'</pre>
    </section>
    {{if .EditorialEnabled}}
    <section class="card">
      <h2>Rule Upsert Example</h2>
      <pre>curl -X POST {{.BaseURL}}/api/demo/editorial/upsert -H 'Content-Type: application/json' -d '{
  "rule": {
    "id": "demo-pin-shell",
    "target_type": "transcript_segment",
    "parent_target_id": "media-1",
    "action": "pin",
    "position": 1,
    "enabled": true,
    "scope": {
      "indexes": ["media_transcripts"],
      "query": "blueprint",
      "locale": "en"
    }
  }
}'</pre>
    </section>
    {{else}}
    <section class="card">
      <h2>Editorial Rollback Lever</h2>
      <pre>APP_SEARCH_DEMO__EDITORIAL_ENABLED=false go run ./cmd/demo</pre>
    </section>
    {{end}}
  </section>
</main>
</body>
</html>
`))

type link struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

type homeView struct {
	Name             string            `json:"name"`
	Env              string            `json:"env"`
	Address          string            `json:"address"`
	AdminBasePath    string            `json:"admin_base_path"`
	ConfigPath       string            `json:"config_path"`
	StartedAt        string            `json:"started_at"`
	Provider         string            `json:"provider"`
	Links            []link            `json:"links"`
	Search           searchdemo.Status `json:"search"`
	SearchJSON       string            `json:"search_json"`
	RulesJSON        string            `json:"rules_json"`
	EditorialEnabled bool              `json:"editorial_enabled"`
}

type searchView struct {
	Title              string
	ActionPath         string
	APIPath            string
	SuggestPath        string
	CurrentAPIURL      string
	Query              string
	Surface            string
	Locale             string
	CacheDisabled      bool
	AcceptLanguage     string
	LocaleSource       string
	LocaleSupported    bool
	TopicFilter        string
	Topics             []string
	FacetFilters       map[string][]string
	LandingTitle       string
	LandingSlug        string
	Breadcrumb         string
	PublishedYearGTE   *int
	PublishedYearLTE   *int
	DurationSecondsGTE *int
	DurationSecondsLTE *int
	Group              bool
	SortField          string
	SortDir            string
	Page               int
	PerPage            int
	TotalPages         int
	HasPrev            bool
	HasNext            bool
	PrevURL            string
	NextURL            string
	Result             types.SearchResultPage
	RequestJSON        string
	ResultJSON         string
}

type opsView struct {
	BaseURL          string
	StatusJSON       string
	RulesJSON        string
	SmokeFlows       []searchdemo.SmokeFlow
	EditorialEnabled bool
}

type editorialUpsertPayload struct {
	Rule types.EditorialRankRule `json:"rule"`
}

type editorialIDPayload struct {
	ID string `json:"id"`
}

type reindexPayload struct {
	BatchSize int `json:"batch_size"`
}

type demoUserPayload struct {
	User userstypes.AuthUser `json:"user"`
}

type demoUserLifecyclePayload struct {
	UserID string `json:"user_id"`
	Target string `json:"target"`
}

type demoUserProfilePayload struct {
	UserID string                  `json:"user_id"`
	Patch  userstypes.ProfilePatch `json:"patch"`
}

type demoUserRolePayload struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func Register(appCore *core.Core) error {
	if appCore == nil || appCore.Router == nil {
		return fmt.Errorf("core router is not initialized")
	}
	protected, err := appCore.DemoRouteProtection()
	if err != nil {
		return err
	}

	appCore.Router.Get("/", homeHandler(appCore)).SetName("shell.home")
	appCore.Router.Get("/healthz", healthHandler(appCore)).SetName("shell.healthz")
	appCore.Router.Get("/readyz", readyHandler(appCore)).SetName("shell.readyz")
	appCore.Router.Get("/demo/search", searchPageHandler(appCore, "Search Demo", "/demo/search", "/api/demo/search", "/api/demo/suggest")).SetName("shell.demo.search")
	appCore.Router.Get("/demo/topics/:topic_slug", topicLandingHandler(appCore)).SetName("shell.demo.topic")
	appCore.Router.Get("/demo/ops", opsPageHandler(appCore), protected).SetName("shell.demo.ops")
	appCore.Router.Get("/api/demo/health", searchHealthAPIHandler(appCore)).SetName("shell.demo.health")
	appCore.Router.Get("/api/demo/stats", searchStatsAPIHandler(appCore), protected).SetName("shell.demo.stats")
	appCore.Router.Get("/api/demo/search", searchAPIHandler(appCore)).SetName("shell.demo.search.api")
	appCore.Router.Get("/api/demo/suggest", suggestAPIHandler(appCore)).SetName("shell.demo.suggest.api")
	if editorialEnabled(appCore) {
		appCore.Router.Get("/api/demo/editorial", editorialListAPIHandler(appCore), protected).SetName("shell.demo.editorial.list")
		appCore.Router.Post("/api/demo/editorial/upsert", editorialUpsertAPIHandler(appCore), protected).SetName("shell.demo.editorial.upsert")
		appCore.Router.Post("/api/demo/editorial/enable", editorialToggleAPIHandler(appCore, true), protected).SetName("shell.demo.editorial.enable")
		appCore.Router.Post("/api/demo/editorial/disable", editorialToggleAPIHandler(appCore, false), protected).SetName("shell.demo.editorial.disable")
		appCore.Router.Post("/api/demo/editorial/delete", editorialDeleteAPIHandler(appCore), protected).SetName("shell.demo.editorial.delete")
	}
	appCore.Router.Post("/api/demo/ensure", ensureAPIHandler(appCore), protected).SetName("shell.demo.ensure")
	appCore.Router.Post("/api/demo/reindex", reindexAPIHandler(appCore), protected).SetName("shell.demo.reindex")
	appCore.Router.Post("/api/demo/users/create", userCreateAPIHandler(appCore), protected).SetName("shell.demo.users.create")
	appCore.Router.Post("/api/demo/users/update", userUpdateAPIHandler(appCore), protected).SetName("shell.demo.users.update")
	appCore.Router.Post("/api/demo/users/lifecycle", userLifecycleAPIHandler(appCore), protected).SetName("shell.demo.users.lifecycle")
	appCore.Router.Post("/api/demo/users/profile", userProfileAPIHandler(appCore), protected).SetName("shell.demo.users.profile")
	appCore.Router.Post("/api/demo/users/role", userRoleAPIHandler(appCore), protected).SetName("shell.demo.users.role")

	return quicksite.RegisterSiteRoutes(
		appCore.Router,
		appCore.Admin,
		appCore.AdminConfig,
		quicksite.SiteConfig{
			BasePath:      "/site",
			DefaultLocale: appCore.Config.Admin.DefaultLocale,
			Search: quicksite.SiteSearchConfig{
				Route:    "/search",
				Endpoint: "/api/v1/site/search",
				Indexes:  []string{appCore.Search.IndexName()},
			},
		},
		quicksite.WithSearchProvider(appCore.SiteSearchProvider),
		quicksite.WithSearchOperations(appCore.SearchOperations),
		quicksite.WithSearchHandlers(searchPageHandler(appCore, "Site Search", "/site/search", "/api/v1/site/search", "/api/v1/site/search/suggest"), nil),
		quicksite.WithContentHandler(func(c router.Context) error {
			return c.JSON(404, map[string]any{
				"error": "site content is not configured in the search shell",
			})
		}),
	)
}

func homeHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		if appCore == nil || appCore.Config == nil || appCore.Search == nil {
			return c.JSON(500, map[string]any{"error": "core config is unavailable"})
		}

		status, err := appCore.Search.Status(c.Context())
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		rulesJSON := "editorial exposure disabled"
		if editorialEnabled(appCore) {
			rules, err := appCore.Search.ListEditorialRules(c.Context(), nil)
			if err != nil {
				return c.JSON(500, map[string]any{"error": err.Error()})
			}
			rulesJSON = prettyJSON(rules)
		}

		cfg := appCore.Config
		links := []link{
			{Label: "Demo media grouped", Href: "/demo/search?surface=media_grouped&group=true&q=transcript"},
			{Label: "Demo shared content", Href: "/demo/search?surface=content_shared&q=search"},
			{Label: "Demo split content", Href: "/demo/search?surface=content_split&q=search"},
			{Label: "Demo users", Href: "/demo/search?surface=users&q=admin"},
			{Label: "Architecture landing preset", Href: "/demo/topics/architecture"},
			{Label: "Operations page", Href: "/demo/ops"},
			{Label: "Demo search API", Href: "/api/demo/search?surface=content_shared&q=search"},
			{Label: "Admin search API", Href: path.Join(cfg.Admin.BasePath, "api/search") + "?query=search&limit=5"},
			{Label: "Site search page", Href: "/site/search?q=search&locale=en"},
			{Label: "Site search API", Href: "/api/v1/site/search?q=search&locale=en"},
			{Label: "Site suggest API", Href: "/api/v1/site/search/suggest?q=sea&locale=en"},
			{Label: "Admin root", Href: cfg.Admin.BasePath},
			{Label: "Admin login", Href: path.Join(cfg.Admin.BasePath, "login")},
		}
		if editorialEnabled(appCore) {
			links = append(links, link{Label: "Demo editorial API", Href: "/api/demo/editorial"})
		}
		view := homeView{
			Name:             cfg.Name,
			Env:              cfg.Env,
			Address:          normalizeAddress(cfg.Server.Address),
			AdminBasePath:    cfg.Admin.BasePath,
			ConfigPath:       cfg.ConfigPath,
			StartedAt:        appCore.StartedAt.Format(time.RFC3339),
			Provider:         appCore.Search.ProviderName(),
			Links:            links,
			Search:           status,
			SearchJSON:       prettyJSON(status),
			RulesJSON:        rulesJSON,
			EditorialEnabled: editorialEnabled(appCore),
		}
		var out bytes.Buffer
		if err := homeTemplate.Execute(&out, view); err != nil {
			return c.JSON(500, map[string]any{"error": "failed to render home page", "details": err.Error()})
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return c.SendString(out.String())
	}
}

func searchPageHandler(appCore *core.Core, title, actionPath, apiPath, suggestPath string) router.HandlerFunc {
	return func(c router.Context) error {
		request := parseSearchRequest(c)
		return renderSearchPage(c, appCore, title, actionPath, apiPath, suggestPath, request)
	}
}

func topicLandingHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		request := parseSearchRequest(c)
		request.LandingSlug = strings.TrimSpace(c.Param("topic_slug"))
		request.Surface = searchdemo.SurfaceMediaGrouped
		request.Group = true
		if preset, ok := media.TopicLandingPreset(request.LandingSlug); ok {
			if request.FacetFilters == nil {
				request.FacetFilters = map[string][]string{}
			}
			for field, values := range preset.FacetFilter {
				request.FacetFilters[field] = append([]string(nil), values...)
			}
		}
		title := "Topic Landing"
		if preset, ok := media.TopicLandingPreset(request.LandingSlug); ok {
			title = preset.Title
		}
		return renderSearchPage(c, appCore, title, path.Join("/demo/topics", request.LandingSlug), "/api/demo/search", "/api/demo/suggest", request)
	}
}

func renderSearchPage(c router.Context, appCore *core.Core, title, actionPath, apiPath, suggestPath string, request searchdemo.SearchRequest) error {
	request = appCore.Search.NormalizeSearchRequest(request)
	result, err := appCore.Search.Search(c.Context(), request)
	if err != nil {
		return c.JSON(500, map[string]any{"error": err.Error()})
	}

	// Calculate pagination
	page := result.Page
	if page < 1 {
		page = 1
	}
	perPage := result.PerPage
	if perPage < 1 {
		perPage = 10
	}
	totalPages := (result.Total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	topics := normalizeTopics(request.Topic, request.Topics)
	landingTitle := ""
	breadcrumb := ""
	if preset, ok := media.TopicLandingPreset(request.LandingSlug); ok {
		landingTitle = preset.Title
		breadcrumb = preset.Breadcrumb
	}

	view := searchView{
		Title:              title,
		ActionPath:         actionPath,
		APIPath:            apiPath,
		SuggestPath:        suggestPath,
		Query:              request.Query,
		Surface:            request.Surface,
		Locale:             request.Locale,
		CacheDisabled:      request.CacheDisabled,
		AcceptLanguage:     request.AcceptLanguage,
		LocaleSource:       request.LocaleSource,
		LocaleSupported:    request.LocaleSupported,
		TopicFilter:        strings.Join(topics, ", "),
		Topics:             topics,
		FacetFilters:       cloneFacetFilterMap(request.FacetFilters),
		LandingTitle:       landingTitle,
		LandingSlug:        request.LandingSlug,
		Breadcrumb:         breadcrumb,
		PublishedYearGTE:   request.PublishedYearGTE,
		PublishedYearLTE:   request.PublishedYearLTE,
		DurationSecondsGTE: request.DurationSecondsGTE,
		DurationSecondsLTE: request.DurationSecondsLTE,
		Group:              request.Group,
		SortField:          request.SortField,
		SortDir:            request.SortDir,
		Page:               page,
		PerPage:            perPage,
		TotalPages:         totalPages,
		HasPrev:            page > 1,
		HasNext:            page < totalPages,
		Result:             result,
		RequestJSON:        prettyJSON(request),
		ResultJSON:         prettyJSON(result),
	}
	view.CurrentAPIURL = buildSearchURL(view.APIPath, view, view.Page)
	if view.HasPrev {
		view.PrevURL = buildSearchURL(view.ActionPath, view, view.Page-1)
	}
	if view.HasNext {
		view.NextURL = buildSearchURL(view.ActionPath, view, view.Page+1)
	}
	var out bytes.Buffer
	if err := searchTemplate.Execute(&out, view); err != nil {
		return c.JSON(500, map[string]any{"error": "failed to render search page", "details": err.Error()})
	}
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	return c.SendString(out.String())
}

func opsPageHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		status, err := appCore.Search.Status(c.Context())
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		rulesJSON := "editorial exposure disabled"
		if editorialEnabled(appCore) {
			rules, err := appCore.Search.ListEditorialRules(c.Context(), nil)
			if err != nil {
				return c.JSON(500, map[string]any{"error": err.Error()})
			}
			rulesJSON = prettyJSON(rules)
		}
		view := opsView{
			BaseURL:          normalizeAddress(appCore.Config.Server.Address),
			StatusJSON:       prettyJSON(status),
			RulesJSON:        rulesJSON,
			SmokeFlows:       status.SmokeFlows,
			EditorialEnabled: editorialEnabled(appCore),
		}
		var out bytes.Buffer
		if err := opsTemplate.Execute(&out, view); err != nil {
			return c.JSON(500, map[string]any{"error": "failed to render operations page", "details": err.Error()})
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return c.SendString(out.String())
	}
}

func searchHealthAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		status, err := appCore.Search.Status(c.Context())
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, status)
	}
}

func searchStatsAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		stats, err := appCore.Search.Stats(c.Context())
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, stats)
	}
}

func searchAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		request := appCore.Search.NormalizeSearchRequest(parseSearchRequest(c))
		result, err := appCore.Search.Search(c.Context(), request)
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{
			"request": request,
			"result":  result,
		})
	}
}

func suggestAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		request := appCore.Search.BindSuggestRequest(searchdemo.SuggestRequest{
			Query:          strings.TrimSpace(c.Query("q")),
			Surface:        strings.TrimSpace(c.Query("surface")),
			Locale:         strings.TrimSpace(c.Query("locale")),
			CacheDisabled:  strings.EqualFold(strings.TrimSpace(c.Query("cache_disabled")), "true"),
			AcceptLanguage: resolveAcceptLanguage(c.Query("accept_language"), c.Header("Accept-Language")),
			Limit:          atoiDefault(c.Query("limit"), 5),
		})
		result, err := appCore.Search.Suggest(c.Context(), request)
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{
			"request": request,
			"result":  result,
		})
	}
}

func editorialListAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		enabled, ok := optionalBool(c.Query("enabled"))
		var filter *bool
		if ok {
			filter = &enabled
		}
		rules, err := appCore.Search.ListEditorialRules(c.Context(), filter)
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"rules": rules})
	}
}

func editorialUpsertAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := editorialUpsertPayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		if err := appCore.Search.UpsertEditorialRule(c.Context(), payload.Rule); err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true, "id": payload.Rule.ID})
	}
}

func editorialToggleAPIHandler(appCore *core.Core, enabled bool) router.HandlerFunc {
	return func(c router.Context) error {
		payload := editorialIDPayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		var err error
		if enabled {
			err = appCore.Search.EnableEditorialRule(c.Context(), payload.ID)
		} else {
			err = appCore.Search.DisableEditorialRule(c.Context(), payload.ID)
		}
		if err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true, "id": payload.ID, "enabled": enabled})
	}
}

func editorialDeleteAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := editorialIDPayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		if err := appCore.Search.DeleteEditorialRule(c.Context(), payload.ID); err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true, "id": payload.ID})
	}
}

func editorialEnabled(appCore *core.Core) bool {
	if appCore == nil || appCore.Config == nil {
		return false
	}
	return appCore.Config.SearchDemo.EditorialEnabled
}

func ensureAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		if err := appCore.Search.Ensure(c.Context()); err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		status, _ := appCore.Search.Status(c.Context())
		return c.JSON(200, map[string]any{"ok": true, "status": status})
	}
}

func reindexAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := reindexPayload{}
		_ = c.Bind(&payload)
		if err := appCore.Search.Reindex(c.Context(), payload.BatchSize); err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		status, _ := appCore.Search.Status(c.Context())
		return c.JSON(200, map[string]any{"ok": true, "status": status})
	}
}

func userCreateAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := demoUserPayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		user, err := appCore.Search.CreateUser(c.Context(), payload.User)
		if err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true, "user": user})
	}
}

func userUpdateAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := demoUserPayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		user, err := appCore.Search.UpdateUser(c.Context(), payload.User)
		if err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true, "user": user})
	}
}

func userLifecycleAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := demoUserLifecyclePayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		if err := appCore.Search.TransitionUser(c.Context(), payload.UserID, userstypes.LifecycleState(strings.TrimSpace(payload.Target))); err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true})
	}
}

func userProfileAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := demoUserProfilePayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		if err := appCore.Search.UpdateUserProfile(c.Context(), payload.UserID, payload.Patch); err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true})
	}
}

func userRoleAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		payload := demoUserRolePayload{}
		if err := c.Bind(&payload); err != nil {
			return c.JSON(400, map[string]any{"error": "invalid payload", "details": err.Error()})
		}
		if err := appCore.Search.UpdateUserRole(c.Context(), payload.UserID, payload.Role); err != nil {
			return c.JSON(400, map[string]any{"error": err.Error()})
		}
		return c.JSON(200, map[string]any{"ok": true})
	}
}

func healthHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		return c.JSON(200, map[string]any{
			"ok":         true,
			"service":    "search-shell",
			"started_at": appCore.StartedAt.Format(time.RFC3339),
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func readyHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		ready := appCore != nil && appCore.Admin != nil && appCore.Auther != nil && appCore.Server != nil && appCore.Search != nil
		status := 200
		if !ready {
			status = 503
		}
		return c.JSON(status, map[string]any{
			"ready":              ready,
			"admin_initialized":  appCore != nil && appCore.Admin != nil,
			"auth_initialized":   appCore != nil && appCore.Auther != nil,
			"search_initialized": appCore != nil && appCore.Search != nil,
			"timestamp":          time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func parseSearchRequest(c router.Context) searchdemo.SearchRequest {
	topics := normalizeTopics(strings.TrimSpace(c.Query("topic")), strings.Split(c.Query("topics"), ","))
	sortField, sortDir := normalizeSort(strings.TrimSpace(c.Query("sort")), strings.TrimSpace(c.Query("sort_dir")))
	legacyTopic := ""
	if len(topics) == 1 {
		legacyTopic = topics[0]
	}
	facetFilters := map[string][]string{}
	for key, raw := range c.Queries() {
		if !strings.HasPrefix(key, "facet_") {
			continue
		}
		field := strings.TrimPrefix(key, "facet_")
		values := normalizeTopics("", strings.Split(raw, ","))
		if len(values) > 0 {
			facetFilters[field] = values
		}
	}

	return searchdemo.SearchRequest{
		Query:              strings.TrimSpace(c.Query("q")),
		Surface:            strings.TrimSpace(c.Query("surface")),
		Locale:             strings.TrimSpace(c.Query("locale")),
		CacheDisabled:      strings.EqualFold(strings.TrimSpace(c.Query("cache_disabled")), "true"),
		AcceptLanguage:     resolveAcceptLanguage(c.Query("accept_language"), c.Header("Accept-Language")),
		Topic:              legacyTopic,
		Topics:             topics,
		FacetFilters:       facetFilters,
		LandingSlug:        strings.TrimSpace(c.Query("landing_slug")),
		PublishedYearGTE:   optionalInt(c.Query("published_year_gte")),
		PublishedYearLTE:   optionalInt(c.Query("published_year_lte")),
		DurationSecondsGTE: optionalInt(c.Query("duration_seconds_gte")),
		DurationSecondsLTE: optionalInt(c.Query("duration_seconds_lte")),
		Group:              strings.EqualFold(strings.TrimSpace(c.Query("group")), "true"),
		Page:               atoiDefault(c.Query("page"), 1),
		PerPage:            atoiDefault(c.Query("per_page"), 10),
		SortField:          sortField,
		SortDir:            sortDir,
	}
}

func buildSearchURL(basePath string, state searchView, page int) string {
	params := url.Values{}
	if query := strings.TrimSpace(state.Query); query != "" {
		params.Set("q", query)
	}
	if surface := strings.TrimSpace(state.Surface); surface != "" {
		params.Set("surface", surface)
	}
	if locale := strings.TrimSpace(state.Locale); locale != "" {
		params.Set("locale", locale)
	}
	if state.CacheDisabled {
		params.Set("cache_disabled", "true")
	}
	if acceptLanguage := strings.TrimSpace(state.AcceptLanguage); acceptLanguage != "" {
		params.Set("accept_language", acceptLanguage)
	}
	if len(state.Topics) > 0 {
		params.Set("topics", strings.Join(state.Topics, ","))
	}
	if slug := strings.TrimSpace(state.LandingSlug); slug != "" {
		params.Set("landing_slug", slug)
	}
	for field, values := range state.FacetFilters {
		if len(values) > 0 {
			params.Set("facet_"+field, strings.Join(values, ","))
		}
	}
	if state.PublishedYearGTE != nil {
		params.Set("published_year_gte", strconv.Itoa(*state.PublishedYearGTE))
	}
	if state.PublishedYearLTE != nil {
		params.Set("published_year_lte", strconv.Itoa(*state.PublishedYearLTE))
	}
	if state.DurationSecondsGTE != nil {
		params.Set("duration_seconds_gte", strconv.Itoa(*state.DurationSecondsGTE))
	}
	if state.DurationSecondsLTE != nil {
		params.Set("duration_seconds_lte", strconv.Itoa(*state.DurationSecondsLTE))
	}
	params.Set("group", strconv.FormatBool(state.Group))
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if state.PerPage > 0 {
		params.Set("per_page", strconv.Itoa(state.PerPage))
	}
	if sortField := strings.TrimSpace(state.SortField); sortField != "" {
		params.Set("sort", sortField)
		params.Set("sort_dir", firstNonEmpty(strings.TrimSpace(state.SortDir), "asc"))
	}
	encoded := params.Encode()
	if encoded == "" {
		return basePath
	}
	return basePath + "?" + encoded
}

func normalizeTopics(single string, values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	add := func(raw string) {
		for _, item := range strings.Split(raw, ",") {
			item = strings.TrimSpace(item)
			key := strings.ToLower(item)
			if item == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, item)
		}
	}
	add(single)
	for _, value := range values {
		add(value)
	}
	return out
}

func cloneFacetFilterMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for field, values := range in {
		out[field] = append([]string(nil), values...)
	}
	return out
}

func resolveAcceptLanguage(queryValue, headerValue string) string {
	if value := strings.TrimSpace(queryValue); value != "" {
		return value
	}
	return strings.TrimSpace(headerValue)
}

func normalizeSort(field, dir string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "title", media.FieldPublishedYear, media.FieldDurationSeconds:
	default:
		return "", ""
	}
	field = strings.ToLower(strings.TrimSpace(field))
	if strings.EqualFold(strings.TrimSpace(dir), "desc") {
		return field, "desc"
	}
	return field, "asc"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func prettyJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func formatTimestampRange(anchor *types.MediaAnchor) string {
	if anchor == nil {
		return ""
	}
	return fmt.Sprintf("%s - %s", formatTimestamp(anchor.StartMS), formatTimestamp(anchor.EndMS))
}

func formatTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}

	hours := ms / (60 * 60 * 1000)
	minutes := (ms % (60 * 60 * 1000)) / (60 * 1000)
	seconds := (ms % (60 * 1000)) / 1000
	tenths := (ms % 1000) / 100

	if hours > 0 {
		if tenths > 0 {
			return fmt.Sprintf("%d:%02d:%02d.%d", hours, minutes, seconds, tenths)
		}
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	if tenths > 0 {
		return fmt.Sprintf("%d:%02d.%d", minutes, seconds, tenths)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func optionalBool(raw string) (bool, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false, false
	}
	out, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return out, true
}

func atoiDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func optionalInt(raw string) *int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return nil
	}
	return &value
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}
	if strings.HasPrefix(address, ":") {
		return "http://localhost" + address
	}
	return "http://" + address
}
