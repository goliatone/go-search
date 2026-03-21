# Search Shell Example

Standalone `go-search` example derived from the `go-admin` admin-shell pattern.

This example is intentionally a nested module with its own `go.mod` so it can
import local checkouts of `go-search`, `go-admin`, and sibling packages through
explicit `replace` directives.

## Run

```bash
cd examples/search-shell
go run ./cmd/demo
```

Or:

```bash
./taskfile dev:serve
```

## URLs

Default address is `:8484`:

- `http://localhost:8484/`
- `http://localhost:8484/healthz`
- `http://localhost:8484/readyz`
- `http://localhost:8484/demo/search`
- `http://localhost:8484/api/demo/health`
- `http://localhost:8484/api/demo/search?q=transcript`
- `http://localhost:8484/api/demo/suggest?q=search`
- `http://localhost:8484/admin`

## Demo Credentials

- `admin` / `admin.pwd`

## Notes

- The bootstrap runtime uses the `memory` provider.
- Seed data is loaded through `go-search` commands at startup.
- The seed catalog intentionally overlaps archive facets across topic, category, people, subject, text, deity, locale, decade, duration, location, sangha, format, and series so filter combinations are easy to verify.
- This is the delivery harness described in `SEARCH_DEMO.md`.
- The demo runtime loads the root [`testdata/locale_search_culture.json`](/Users/goliatone/Development/GO/src/github.com/goliatone/go-search/testdata/locale_search_culture.json) fixture by default, so the example and package tests share the same locale-policy catalog.
- `Accept-Language` is bound through `locale.BindLocale` before planner entry, using active-only catalog policy and the configured default locale.
- Multilingual lexical search uses planner locale policy to expand parent and fallback locales, and normalized result metadata includes locale-match/origin annotations.
