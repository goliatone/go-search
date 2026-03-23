# Search V1 Release Artifacts

This package is the canonical Phase 10 release gate for Search V1.

Artifacts:

- `search_v1_release_checklist.schema.json`: machine-readable checklist schema
- `search_v1_release_checklist.json`: pending release template; it should fail validation until signoff is complete
- `testdata/search_v1_release_checklist_approved_sample.json`: approved sample showing the finished checklist shape
- `SEARCH_V1_RUNBOOK.md`: operator runbook for the example harness and cross-repo release gate

Tests verify both layers:

- the checklist validator rejects the pending template
- the approved sample remains valid
- the runtime validation profile exercises grouped archive search, mixed-content search, locale policy, editorial controls, scope filters, and cache invalidation
- the route validation profile checks demo and site routes, plus the editorial exposure toggle

Workflow map:

- Phase 3 locale policy: `RunSearchV1RuntimeValidationProfile` checks `Accept-Language`, exact matches, and fallback annotations
- Phase 5 editorial/media: grouped archive and editorial pin/hide checks
- Phase 6 `go-admin` compatibility: site search page and API checks
- Phase 7 `go-cms` lifecycle adoption: seeded CMS-backed content is part of the mixed-content runtime profile
- Phase 8 `go-users` adoption: user-scope search and suggest checks
- Phase 9 provider/cache parity: cache + reindex checks and external provider validation
