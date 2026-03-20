package apphttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/goliatone/go-router"
	"github.com/goliatone/go-search/examples/search-shell/internal/core"
	"github.com/goliatone/go-search/examples/search-shell/internal/searchdemo"
)

var homeTemplate = template.Must(template.New("home").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>{{.Name}}</title>
<style>
:root {
  --bg:#f6f7f4;
  --card:#fffef8;
  --ink:#17211d;
  --muted:#5c675f;
  --border:#d9e0d4;
  --accent:#0f766e;
  --accent-2:#14532d;
}
*{box-sizing:border-box}
body{margin:0;font:15px/1.5 ui-sans-serif,system-ui,sans-serif;background:linear-gradient(180deg,#eef3eb 0,#f8f7f1 100%);color:var(--ink)}
main{max-width:1080px;margin:24px auto;padding:0 16px}
.card{background:var(--card);border:1px solid var(--border);border-radius:14px;padding:18px 20px;margin-bottom:14px}
h1,h2{margin:0 0 10px 0}
h1{font-size:28px}
h2{font-size:18px}
p{margin:6px 0;color:var(--muted)}
code{font-size:13px;background:#eff3ea;padding:2px 5px;border-radius:4px}
a{color:var(--accent);text-decoration:none}
a:hover{text-decoration:underline}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:14px}
@media (max-width:840px){.grid{grid-template-columns:1fr}}
.badge{display:inline-block;padding:2px 8px;border-radius:999px;font-size:12px;background:#dcfce7;color:var(--accent-2)}
pre{margin:8px 0 0;background:#102218;color:#d9fbe8;padding:10px 12px;border-radius:10px;overflow:auto}
ul{margin:8px 0 0 18px;padding:0}
li{margin:4px 0}
</style>
</head>
<body>
<main>
  <section class="card">
    <h1>{{.Name}}</h1>
    <p>Environment: <code>{{.Env}}</code> | Listening on <code>{{.Address}}</code></p>
    <p>Admin base path: <code>{{.AdminBasePath}}</code> | Search provider: <code>{{.Provider}}</code></p>
    <p>Config path: <code>{{.ConfigPath}}</code></p>
    <p>Started at: <code>{{.StartedAt}}</code></p>
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
      <h2>Search Runtime</h2>
      <p>Index: <code>{{.Search.IndexName}}</code></p>
      <p>Seed documents: <span class="badge">{{.Search.Documents}} docs</span></p>
      <p>Demo locale: <code>{{.Search.DefaultLocale}}</code></p>
      <pre>{{.SearchJSON}}</pre>
    </section>
  </section>

  <section class="grid">
    <section class="card">
      <h2>Demo Auth</h2>
      <ul>
        {{range .Credentials}}
        <li><code>{{.Username}}</code> / <code>{{.Password}}</code></li>
        {{end}}
      </ul>
      <pre>{{.DemoToken}}</pre>
    </section>

    <section class="card">
      <h2>Feature Flags</h2>
      <ul>
        {{range .Features}}
        <li><code>{{.Name}}</code> {{if .Enabled}}<span class="badge">on</span>{{else}}off{{end}}</li>
        {{end}}
      </ul>
    </section>
  </section>
</main>
</body>
</html>
`))

var searchTemplate = template.Must(template.New("search").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>Search Demo</title>
<style>
:root {
  --bg:#f5f5ef;
  --card:#fff;
  --ink:#1a1a18;
  --muted:#62625a;
  --border:#d9d8cf;
  --accent:#155e75;
}
*{box-sizing:border-box}
body{margin:0;font:15px/1.5 ui-sans-serif,system-ui,sans-serif;background:var(--bg);color:var(--ink)}
main{max-width:1120px;margin:24px auto;padding:0 16px}
.card{background:var(--card);border:1px solid var(--border);border-radius:14px;padding:18px 20px;margin-bottom:14px}
.grid{display:grid;grid-template-columns:320px 1fr;gap:14px}
@media (max-width:920px){.grid{grid-template-columns:1fr}}
label{display:block;font-size:13px;color:var(--muted);margin-bottom:6px}
input,select{width:100%;padding:10px 12px;border:1px solid var(--border);border-radius:10px;background:#fff}
button{margin-top:12px;padding:10px 14px;border:0;border-radius:10px;background:var(--accent);color:#fff;cursor:pointer}
code{font-size:13px;background:#f0f0ea;padding:2px 5px;border-radius:4px}
pre{margin:8px 0 0;background:#0d1721;color:#d5e8ff;padding:10px 12px;border-radius:10px;overflow:auto}
.result{padding:12px 0;border-top:1px solid var(--border)}
.result:first-child{border-top:0;padding-top:0}
.muted{color:var(--muted)}
h1,h2,h3{margin:0 0 10px 0}
h1{font-size:26px}
h2{font-size:18px}
h3{font-size:16px}
</style>
</head>
<body>
<main>
  <section class="card">
    <h1>Search Demo</h1>
    <p class="muted">Bootstrap harness for the go-search package using the memory provider first.</p>
  </section>
  <section class="grid">
    <section class="card">
      <form method="GET" action="/demo/search">
        <label for="q">Query</label>
        <input id="q" name="q" value="{{.Query}}" placeholder="transcript" />
        <label for="locale">Locale</label>
        <input id="locale" name="locale" value="{{.Locale}}" placeholder="en" />
        <label for="topic">Topic</label>
        <input id="topic" name="topic" value="{{.Topic}}" placeholder="architecture" />
        <label for="group">Grouped</label>
        <select id="group" name="group">
          <option value="true" {{if .Group}}selected{{end}}>true</option>
          <option value="false" {{if not .Group}}selected{{end}}>false</option>
        </select>
        <button type="submit">Search</button>
      </form>
      <p><a href="/">Back home</a></p>
    </section>
    <section class="card">
      <h2>Request</h2>
      <pre>{{.RequestJSON}}</pre>
      <h2>Result</h2>
      <p class="muted">Groups: <code>{{len .Groups}}</code> | Hits: <code>{{len .Hits}}</code> | Total: <code>{{.Total}}</code></p>
      {{if .Groups}}
        {{range .Groups}}
        <div class="result">
          <h3>{{if .Parent}}{{.Parent.Title}}{{else}}{{.Key}}{{end}}</h3>
          <p class="muted">Group key: <code>{{.Key}}</code> | hits: <code>{{.Count}}</code></p>
          {{range .Hits}}
          <p><strong>{{.Title}}</strong> <span class="muted">({{.Locale}})</span></p>
          {{if .Snippet}}<p>{{.Snippet.Text}}</p>{{end}}
          {{if .Anchor}}<p class="muted">Anchor: <code>{{.Anchor.StartMS}}</code>-<code>{{.Anchor.EndMS}}</code> <a href="{{.Anchor.URL}}">{{.Anchor.URL}}</a></p>{{end}}
          {{end}}
        </div>
        {{end}}
      {{else}}
        {{range .Hits}}
        <div class="result">
          <h3>{{.Title}}</h3>
          <p class="muted">{{.Type}} | {{.Locale}} | score {{printf "%.2f" .Score}}</p>
          {{if .Snippet}}<p>{{.Snippet.Text}}</p>{{end}}
        </div>
        {{else}}
        <p class="muted">No results.</p>
        {{end}}
      {{end}}
      <h2>Raw JSON</h2>
      <pre>{{.ResultJSON}}</pre>
    </section>
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
	Name          string                `json:"name"`
	Env           string                `json:"env"`
	Address       string                `json:"address"`
	AdminBasePath string                `json:"admin_base_path"`
	ConfigPath    string                `json:"config_path"`
	StartedAt     string                `json:"started_at"`
	DemoToken     string                `json:"demo_token"`
	Provider      string                `json:"provider"`
	Features      []core.FeatureStatus  `json:"features"`
	Links         []link                `json:"links"`
	Credentials   []core.DemoCredential `json:"credentials"`
	Search        searchdemo.Status     `json:"search"`
	SearchJSON    string                `json:"search_json"`
}

type searchView struct {
	Query       string
	Locale      string
	Topic       string
	Group       bool
	Groups      []any
	Hits        []any
	Total       int
	RequestJSON string
	ResultJSON  string
}

func Register(appCore *core.Core) error {
	if appCore == nil || appCore.Router == nil {
		return fmt.Errorf("core router is not initialized")
	}

	appCore.Router.Get("/", homeHandler(appCore)).SetName("shell.home")
	appCore.Router.Get("/healthz", healthHandler(appCore)).SetName("shell.healthz")
	appCore.Router.Get("/readyz", readyHandler(appCore)).SetName("shell.readyz")
	appCore.Router.Get("/demo/search", searchPageHandler(appCore)).SetName("shell.demo.search")
	appCore.Router.Get("/api/demo/health", searchHealthAPIHandler(appCore)).SetName("shell.demo.health")
	appCore.Router.Get("/api/demo/search", searchAPIHandler(appCore)).SetName("shell.demo.search.api")
	appCore.Router.Get("/api/demo/suggest", suggestAPIHandler(appCore)).SetName("shell.demo.suggest.api")
	return nil
}

func homeHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		if appCore == nil || appCore.Config == nil || appCore.Search == nil {
			return c.JSON(500, map[string]any{"error": "core config is unavailable"})
		}

		status, _ := appCore.Search.Status(c.Context())
		statusJSON := prettyJSON(status)
		cfg := appCore.Config
		view := homeView{
			Name:          cfg.Name,
			Env:           cfg.Env,
			Address:       normalizeAddress(cfg.Server.Address),
			AdminBasePath: cfg.Admin.BasePath,
			ConfigPath:    cfg.ConfigPath,
			StartedAt:     appCore.StartedAt.Format(time.RFC3339),
			DemoToken:     strings.TrimSpace(appCore.DemoToken),
			Provider:      appCore.Search.ProviderName(),
			Features:      appCore.Features(),
			Credentials:   appCore.DemoCredentials,
			Search:        status,
			SearchJSON:    statusJSON,
			Links: []link{
				{Label: "Home", Href: "/"},
				{Label: "Demo search page", Href: "/demo/search?q=transcript"},
				{Label: "Demo health API", Href: "/api/demo/health"},
				{Label: "Demo search API", Href: "/api/demo/search?q=transcript"},
				{Label: "Demo suggest API", Href: "/api/demo/suggest?q=search"},
				{Label: "Admin root", Href: cfg.Admin.BasePath},
				{Label: "Admin login", Href: path.Join(cfg.Admin.BasePath, "login")},
				{Label: "Health", Href: "/healthz"},
				{Label: "Ready", Href: "/readyz"},
			},
		}
		if view.DemoToken == "" {
			view.DemoToken = "(token mint failed)"
		}

		var out bytes.Buffer
		if err := homeTemplate.Execute(&out, view); err != nil {
			return c.JSON(500, map[string]any{"error": "failed to render home page", "details": err.Error()})
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return c.SendString(out.String())
	}
}

func searchPageHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		request := appCore.Search.BindSearchRequest(parseSearchRequest(c))
		result, err := appCore.Search.Search(c.Context(), request)
		if err != nil {
			return c.JSON(500, map[string]any{"error": err.Error()})
		}
		view := searchView{
			Query:       request.Query,
			Locale:      request.Locale,
			Topic:       request.Topic,
			Group:       request.Group,
			Groups:      groupsToAny(result.Groups),
			Hits:        hitsToAny(result.Hits),
			Total:       result.Total,
			RequestJSON: prettyJSON(request),
			ResultJSON:  prettyJSON(result),
		}
		var out bytes.Buffer
		if err := searchTemplate.Execute(&out, view); err != nil {
			return c.JSON(500, map[string]any{"error": "failed to render search page", "details": err.Error()})
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

func searchAPIHandler(appCore *core.Core) router.HandlerFunc {
	return func(c router.Context) error {
		request := appCore.Search.BindSearchRequest(parseSearchRequest(c))
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
			Locale:         strings.TrimSpace(c.Query("locale")),
			AcceptLanguage: strings.TrimSpace(c.Header("Accept-Language")),
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
	return searchdemo.SearchRequest{
		Query:          strings.TrimSpace(c.Query("q")),
		Locale:         strings.TrimSpace(c.Query("locale")),
		AcceptLanguage: strings.TrimSpace(c.Header("Accept-Language")),
		Topic:          strings.TrimSpace(c.Query("topic")),
		Group:          !strings.EqualFold(strings.TrimSpace(c.Query("group")), "false"),
		Page:           atoiDefault(c.Query("page"), 1),
		PerPage:        atoiDefault(c.Query("per_page"), 10),
	}
}

func prettyJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func hitsToAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := []any{}
	_ = json.Unmarshal(data, &out)
	return out
}

func groupsToAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := []any{}
	_ = json.Unmarshal(data, &out)
	return out
}

func atoiDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
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
