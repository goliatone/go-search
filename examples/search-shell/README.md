# Search Shell Example

Standalone `go-search` example derived from the `go-admin` admin-shell pattern.

This example is intentionally a nested module with its own `go.mod` so it can
import local checkouts of `go-search`, `go-admin`, and sibling packages through
explicit `replace` directives.

## Run

```bash
cd examples/search-shell
APP_AUTH__SIGNING_KEY="$(openssl rand -base64 32)" \
APP_AUTH__DEMO_PASSWORD="$(openssl rand -base64 18)" \
go run ./cmd/demo
```

Or:

```bash
./taskfile dev:serve
```

Run all Go commands for this example from `examples/search-shell`, not the repo root.

## Provider Modes

The example supports three runtime modes through `internal/config/app.json` or env vars:

- `memory`
  Default. No external dependency. Uses in-memory generations and keeps cache off unless explicitly enabled.
- `typesense`
  Set `APP_SEARCH_DEMO__PROVIDER=typesense` plus `APP_SEARCH_DEMO__TYPESENSE_SERVER_URL` and `APP_SEARCH_DEMO__TYPESENSE_API_KEY`.
- `postgres`
  Set `APP_SEARCH_DEMO__PROVIDER=postgres` plus `APP_SEARCH_DEMO__POSTGRES_DSN`.
  In this mode the demo uses PostgreSQL as the search provider and a Bun-backed generation store for cache invalidation.

Example:

```bash
cd examples/search-shell
APP_SEARCH_DEMO__PROVIDER=postgres \
APP_SEARCH_DEMO__POSTGRES_DSN='postgres://localhost/search_shell?sslmode=disable' \
go run ./cmd/demo
```

## Cache

Cache is default-off and opt-in:

```bash
cd examples/search-shell
APP_SEARCH_DEMO__CACHE_ENABLED=true go run ./cmd/demo
```

When cache is enabled the demo wraps:

- search queries
- suggest queries
- provider capabilities/health metadata

Editorial UI and API exposure are independently controllable:

```bash
cd examples/search-shell
APP_SEARCH_DEMO__EDITORIAL_ENABLED=false go run ./cmd/demo
```

You can bypass cache per request with `cache_disabled=true`:

- `http://localhost:8484/api/demo/search?q=search&cache_disabled=true`
- `http://localhost:8484/api/demo/suggest?q=search&cache_disabled=true`

The operations page and `/api/demo/health` status payload expose the active provider, generation backend, cache wrapper status, and smoke-flow URLs.

## URLs

Default address is loopback-only at `127.0.0.1:8484`:

- `http://localhost:8484/`
- `http://localhost:8484/healthz`
- `http://localhost:8484/readyz`
- `http://localhost:8484/demo/search`
- `http://localhost:8484/api/demo/health`
- `http://localhost:8484/api/demo/search?q=transcript`
- `http://localhost:8484/api/demo/suggest?q=search`
- `http://localhost:8484/admin`

## Demo Credentials

The username defaults to `admin`. Signing keys and passwords are generated
cryptographically for each process unless configured explicitly; they are never
rendered or logged. Set `APP_AUTH__SIGNING_KEY` and
`APP_AUTH__DEMO_PASSWORD` to stable values (at least 32 and 12 characters,
respectively) when browser login is needed.

Operations, statistics, editorial management, reindexing, and user mutations
require an authenticated admin session. Cookie-authenticated mutations also
require the CSRF token issued by an authenticated safe request; bearer-token
clients are not subject to browser CSRF checks.

## Notes

- Seed data is loaded through `go-search` commands at startup.
- The seed catalog intentionally overlaps archive facets across topic, category, people, subject, text, deity, locale, decade, duration, location, sangha, format, and series so filter combinations are easy to verify.
- This is the delivery harness described in `SEARCH_DEMO.md`.
- The demo runtime loads the root [`testdata/locale_search_culture.json`](/Users/goliatone/Development/GO/src/github.com/goliatone/go-search/testdata/locale_search_culture.json) fixture by default, so the example and package tests share the same locale-policy catalog.
- `Accept-Language` is bound through `locale.BindLocale` before planner entry, using active-only catalog policy and the configured default locale.
- Multilingual lexical search uses planner locale policy to expand parent and fallback locales, and normalized result metadata includes locale-match/origin annotations.

## Release Gate

Phase 10 release artifacts live in [`release/`](/Users/goliatone/Development/GO/src/github.com/goliatone/go-search/examples/search-shell/release/README.md).

Run them from `examples/search-shell`:

```bash
./taskfile release:validate
./taskfile release:validate:parity
./taskfile release:validate:full
```

`release:validate` always runs the memory-backed checklist and validation profile.

`release:validate:parity` and `release:validate:full` require:

- `APP_SEARCH_DEMO__TYPESENSE_SERVER_URL`
- `APP_SEARCH_DEMO__TYPESENSE_API_KEY`
- `APP_SEARCH_DEMO__POSTGRES_DSN`

The release runbook is [`release/SEARCH_V1_RUNBOOK.md`](/Users/goliatone/Development/GO/src/github.com/goliatone/go-search/examples/search-shell/release/SEARCH_V1_RUNBOOK.md).
