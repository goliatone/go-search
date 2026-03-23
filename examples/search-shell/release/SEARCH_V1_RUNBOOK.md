# Search V1 Example Runbook

## Fast Local Gate

1. Run `./taskfile dev:test` from `examples/search-shell`.
2. Run `./taskfile release:validate` from `examples/search-shell`.
3. Confirm the release package covers memory-mode grouped archive search, mixed-content search, locale policy, editorial controls, user scope filtering, and cache invalidation.

## Full Release Gate

1. Start external dependencies required for the formal provider parity gate:
   - Typesense configured through `APP_SEARCH_DEMO__TYPESENSE_SERVER_URL` and `APP_SEARCH_DEMO__TYPESENSE_API_KEY`
   - PostgreSQL configured through `APP_SEARCH_DEMO__POSTGRES_DSN`
2. From the `go-search` repo root, run `./taskfile dev:test:release:full`.
3. Confirm the release checklist JSON has real evidence refs and all signoffs approved before tagging or bumping downstream repos.

## Diagnose Provider Parity Failures

1. Run `./taskfile release:validate:parity` from `examples/search-shell`.
2. If Typesense fails first, confirm the server is reachable and the API key matches the configured runtime.
3. If PostgreSQL fails first, confirm the DSN points at a writable database and rerun after clearing any stale release indexes.
4. Treat differences in grouped parent ordering, locale-origin annotations, or cache invalidation as release blockers.

## Diagnose Rollback Paths

1. Switch `APP_SEARCH_DEMO__PROVIDER` back to the prior provider or `memory`.
2. Set `APP_SEARCH_DEMO__CACHE_ENABLED=false` to remove the cache layer.
3. Set `APP_SEARCH_DEMO__EDITORIAL_ENABLED=false` to disable editorial UI and API exposure.
4. Disable broad host search surfaces with the existing `search`, `cms`, and `users` feature flags if the rollback needs to reduce the visible surface further.

## Reproducible Example Workflows

1. Grouped archive search: `GET /api/demo/search?surface=media_grouped&group=true&locale=en&landing_slug=architecture&q=search`
2. Flat heterogeneous search: `GET /api/demo/search?surface=content_shared&locale=en&q=search`
3. Locale binding: `GET /api/demo/search?surface=content_shared&accept_language=fr-CA,fr;q=0.9,en;q=0.8&q=locale`
4. Site search compatibility: `GET /api/v1/site/search?q=search&locale=en`
5. Cache invalidation: `POST /api/demo/reindex` then repeat the flat heterogeneous search request
