package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/ranking"
	"github.com/uptrace/bun"
)

type BunDB interface {
	NewInsert() *bun.InsertQuery
	NewSelect() *bun.SelectQuery
	NewDelete() *bun.DeleteQuery
	NewRaw(query string, args ...any) *bun.RawQuery
	PingContext(ctx context.Context) error
	RunInTx(ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context, tx bun.Tx) error) error
}

type Provider struct {
	db  BunDB
	cfg Config

	mu          sync.RWMutex
	indexes     map[string]types.IndexDefinition
	schemaMu    sync.Mutex
	schemaReady bool
}

func New(cfg Config) (*Provider, error) {
	cfg = normalizeConfig(cfg)
	if cfg.DB == nil {
		return nil, errs.ConfigurationError("postgres db is required", nil)
	}
	return &Provider{
		db:      cfg.DB,
		cfg:     cfg,
		indexes: map[string]types.IndexDefinition{},
	}, nil
}

func (p *Provider) Name() string { return "postgres" }

func (p *Provider) Capabilities(context.Context) (types.CapabilitySet, error) {
	return types.CapabilitySet{
		Facets:               true,
		HierarchicalFacets:   true,
		DisjunctiveFacets:    true,
		Grouping:             true,
		Highlighting:         true,
		Snippets:             true,
		PrefixSearch:         true,
		TypoTolerance:        true,
		SupportedSearchModes: []types.SearchMode{types.SearchModeLexical},
		Limitations: []types.CapabilityLimitation{
			{
				Capability: "range_facets",
				Message:    "postgres provider supports range filtering but does not yet compute dedicated numeric/date range facet buckets in the canonical response",
			},
		},
		Metadata: map[string]any{
			"table_name":        p.cfg.TableName,
			"search_config":     p.cfg.SearchConfig,
			"trigram_threshold": p.cfg.TrigramThreshold,
			"suggest_threshold": p.cfg.SuggestTrigramThreshold,
			"shared_table_v1":   true,
			"provider_strategy": "shared_search_table",
			"supports_pg_trgm":  true,
			"supports_tsvector": true,
		},
	}, nil
}

func (p *Provider) EnsureIndex(ctx context.Context, def types.IndexDefinition) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.indexes[def.Name] = def
	return nil
}

func (p *Provider) UpsertDocuments(ctx context.Context, index string, docs []types.Document) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	models := make([]documentModel, 0, len(docs))
	for _, doc := range docs {
		doc.Index = index
		models = append(models, toModel(index, doc, p.cfg.SearchConfig))
	}
	if len(models) == 0 {
		return nil
	}
	_, err := p.db.NewInsert().
		Model(&models).
		On("CONFLICT (index_name, registration_key, document_id) DO UPDATE").
		Set("registration_key = EXCLUDED.registration_key").
		Set("document_type = EXCLUDED.document_type").
		Set("parent_id = EXCLUDED.parent_id").
		Set("source_type = EXCLUDED.source_type").
		Set("source_id = EXCLUDED.source_id").
		Set("source_version = EXCLUDED.source_version").
		Set("search_config = EXCLUDED.search_config").
		Set("title = EXCLUDED.title").
		Set("summary = EXCLUDED.summary").
		Set("body = EXCLUDED.body").
		Set("url = EXCLUDED.url").
		Set("anchor_url = EXCLUDED.anchor_url").
		Set("locale = EXCLUDED.locale").
		Set("score = EXCLUDED.score").
		Set("created_at_unix = EXCLUDED.created_at_unix").
		Set("updated_at_unix = EXCLUDED.updated_at_unix").
		Set("published_at_unix = EXCLUDED.published_at_unix").
		Set("start_ms = EXCLUDED.start_ms").
		Set("end_ms = EXCLUDED.end_ms").
		Set("fields = EXCLUDED.fields").
		Set("facets = EXCLUDED.facets").
		Set("numeric = EXCLUDED.numeric").
		Set("booleans = EXCLUDED.booleans").
		Set("scope_tenant_id = EXCLUDED.scope_tenant_id").
		Set("scope_org_id = EXCLUDED.scope_org_id").
		Set("scope_labels = EXCLUDED.scope_labels").
		Set("visibility_public = EXCLUDED.visibility_public").
		Set("visibility_roles = EXCLUDED.visibility_roles").
		Set("visibility_permissions = EXCLUDED.visibility_permissions").
		Set("visibility_status = EXCLUDED.visibility_status").
		Set("metadata = EXCLUDED.metadata").
		Exec(ctx)
	return err
}

func (p *Provider) ReplaceDocuments(ctx context.Context, index, registrationKey string, sourceIDs []string, docs []types.Document) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	registrationKey = strings.TrimSpace(registrationKey)
	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if len(sourceIDs) > 0 {
			if _, err := tx.NewDelete().
				Model((*documentModel)(nil)).
				Where("index_name = ?", index).
				Where("registration_key = ?", registrationKey).
				Where("source_id IN (?)", bun.List(sourceIDs)).
				Exec(ctx); err != nil {
				return err
			}
		}
		if len(docs) == 0 {
			return nil
		}
		models := make([]documentModel, 0, len(docs))
		for _, doc := range docs {
			doc.Index = index
			doc.RegistrationKey = firstNonEmptyString(strings.TrimSpace(doc.RegistrationKey), registrationKey)
			models = append(models, toModel(index, doc, p.cfg.SearchConfig))
		}
		_, err := tx.NewInsert().
			Model(&models).
			On("CONFLICT (index_name, registration_key, document_id) DO UPDATE").
			Set("registration_key = EXCLUDED.registration_key").
			Set("document_type = EXCLUDED.document_type").
			Set("parent_id = EXCLUDED.parent_id").
			Set("source_type = EXCLUDED.source_type").
			Set("source_id = EXCLUDED.source_id").
			Set("source_version = EXCLUDED.source_version").
			Set("search_config = EXCLUDED.search_config").
			Set("title = EXCLUDED.title").
			Set("summary = EXCLUDED.summary").
			Set("body = EXCLUDED.body").
			Set("url = EXCLUDED.url").
			Set("anchor_url = EXCLUDED.anchor_url").
			Set("locale = EXCLUDED.locale").
			Set("score = EXCLUDED.score").
			Set("created_at_unix = EXCLUDED.created_at_unix").
			Set("updated_at_unix = EXCLUDED.updated_at_unix").
			Set("published_at_unix = EXCLUDED.published_at_unix").
			Set("start_ms = EXCLUDED.start_ms").
			Set("end_ms = EXCLUDED.end_ms").
			Set("fields = EXCLUDED.fields").
			Set("facets = EXCLUDED.facets").
			Set("numeric = EXCLUDED.numeric").
			Set("booleans = EXCLUDED.booleans").
			Set("scope_tenant_id = EXCLUDED.scope_tenant_id").
			Set("scope_org_id = EXCLUDED.scope_org_id").
			Set("scope_labels = EXCLUDED.scope_labels").
			Set("visibility_public = EXCLUDED.visibility_public").
			Set("visibility_roles = EXCLUDED.visibility_roles").
			Set("visibility_permissions = EXCLUDED.visibility_permissions").
			Set("visibility_status = EXCLUDED.visibility_status").
			Set("metadata = EXCLUDED.metadata").
			Exec(ctx)
		return err
	})
}

func (p *Provider) DeleteDocuments(ctx context.Context, index string, ids []string) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := p.db.NewDelete().
		Model((*documentModel)(nil)).
		Where("index_name = ?", index).
		Where("document_id IN (?)", bun.List(ids)).
		Exec(ctx)
	return err
}

func (p *Provider) DeleteBySource(ctx context.Context, index, registrationKey string, sourceIDs []string) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	if len(sourceIDs) == 0 {
		return nil
	}
	registrationKey = strings.TrimSpace(registrationKey)
	_, err := p.db.NewDelete().
		Model((*documentModel)(nil)).
		Where("index_name = ?", index).
		Where("registration_key = ?", registrationKey).
		Where("source_id IN (?)", bun.List(sourceIDs)).
		Exec(ctx)
	return err
}

func (p *Provider) Search(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	if err := p.ensureSchema(ctx); err != nil {
		return types.SearchResultPage{}, err
	}
	if err := validateSearchRequest(req); err != nil {
		return types.SearchResultPage{}, err
	}
	started := p.cfg.Clock.Now()
	rows, err := p.searchRows(ctx, req)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	hits := make([]types.SearchHit, 0, len(rows))
	docsByIndex := map[string]map[string]types.Document{}
	for _, row := range rows {
		doc := row.toDocument()
		if _, ok := docsByIndex[doc.Index]; !ok {
			docsByIndex[doc.Index] = map[string]types.Document{}
		}
		docsByIndex[doc.Index][facetDocumentKey(doc)] = doc.Clone()
		if !matchesScope(req.Scope, doc) || !matchesLocale(req, doc) || !matchesFilter(req.Filters, doc) {
			continue
		}
		score, ok := scoredDocument(row, req.Query, doc)
		if !ok {
			continue
		}
		hits = append(hits, toHit(doc, score, req))
	}
	sortHits(hits, req)
	page := types.SearchResultPage{
		Hits:       paginateHits(hits, req.Page, req.PerPage),
		Facets:     buildFacets(req, docsByIndex),
		Page:       req.Page,
		PerPage:    req.PerPage,
		Total:      len(hits),
		DurationMS: p.cfg.Clock.Now().Sub(started).Milliseconds(),
	}
	if req.GroupBy != "" {
		grouped := ranking.GroupHits(hits)
		page.Groups = paginateGroups(grouped, req.Page, req.PerPage)
		page.Total = len(grouped)
		page.Hits = flattenGroupHits(page.Groups)
	}
	return page, nil
}

func (p *Provider) SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
	out := make([]types.SearchResultPage, 0, len(requests))
	for _, req := range requests {
		page, err := p.Search(ctx, req)
		if err != nil {
			return nil, err
		}
		out = append(out, page)
	}
	return out, nil
}

func (p *Provider) Suggest(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	if err := p.ensureSchema(ctx); err != nil {
		return types.SuggestResult{}, err
	}
	rows, err := p.suggestRows(ctx, req)
	if err != nil {
		return types.SuggestResult{}, err
	}
	items := []types.SuggestHit{}
	seen := map[string]struct{}{}
	for _, row := range rows {
		doc := row.toDocument()
		if !matchesScope(req.Scope, doc) {
			continue
		}
		if req.Locale != "" && doc.Locale != "" && !strings.EqualFold(req.Locale, doc.Locale) {
			continue
		}
		item := suggestHit(doc, req.PreferParent)
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		item.Score = maxFloat(row.TrigramScore, row.CombinedScore)
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Title < items[j].Title
		}
		return items[i].Score > items[j].Score
	})
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	return types.SuggestResult{Items: items}, nil
}

func (p *Provider) Health(ctx context.Context, req types.HealthRequest) (types.HealthStatus, error) {
	if err := p.ensureSchema(ctx); err != nil {
		return types.HealthStatus{}, err
	}
	if err := p.db.PingContext(ctx); err != nil {
		return types.HealthStatus{
			Provider:  p.Name(),
			Healthy:   false,
			CheckedAt: p.cfg.Clock.Now(),
			Message:   err.Error(),
		}, nil
	}

	type healthRow struct {
		IndexName string `bun:"index_name"`
		Documents int    `bun:"documents"`
	}
	rows := []healthRow{}
	q := p.db.NewSelect().
		Model((*documentModel)(nil)).
		ColumnExpr("index_name").
		ColumnExpr("COUNT(*) AS documents").
		GroupExpr("index_name")
	if len(req.Indexes) > 0 {
		q = q.Where("index_name IN (?)", bun.List(req.Indexes))
	}
	if err := q.Scan(ctx, &rows); err != nil {
		return types.HealthStatus{}, err
	}
	p.mu.RLock()
	ensured := make(map[string]types.IndexDefinition, len(p.indexes))
	for name, def := range p.indexes {
		ensured[name] = def
	}
	p.mu.RUnlock()

	rowCounts := make(map[string]int, len(rows))
	for _, row := range rows {
		rowCounts[row.IndexName] = row.Documents
	}
	names := orderedHealthIndexes(req.Indexes, ensured, rowCounts)
	indexes := make([]types.IndexHealth, 0, len(names))
	for _, name := range names {
		_, isEnsured := ensured[name]
		health := types.IndexHealth{
			Name:      name,
			Ready:     isEnsured,
			Documents: rowCounts[name],
			Metadata: map[string]any{
				"provider": "postgres",
				"ensured":  isEnsured,
			},
		}
		if !isEnsured {
			health.Message = "index not ensured"
		}
		indexes = append(indexes, health)
	}
	return types.HealthStatus{
		Provider:  p.Name(),
		Healthy:   true,
		CheckedAt: p.cfg.Clock.Now(),
		Indexes:   indexes,
		Metadata: map[string]any{
			"table_name": p.cfg.TableName,
		},
	}, nil
}

func (p *Provider) ensureSchema(ctx context.Context) error {
	p.schemaMu.Lock()
	defer p.schemaMu.Unlock()
	if p.schemaReady {
		return nil
	}
	db, ok := p.db.(*bun.DB)
	if !ok {
		return errs.ConfigurationError("postgres provider requires *bun.DB for migrations", nil)
	}
	if err := Migrations().Migrate(ctx, db); err != nil {
		return err
	}
	p.schemaReady = true
	return nil
}

func (p *Provider) searchRows(ctx context.Context, req types.SearchRequest) ([]documentModel, error) {
	rows := []documentModel{}
	q := p.db.NewSelect().
		Model(&rows).
		Where("index_name IN (?)", bun.List(req.Indexes))
	if trimmed := strings.TrimSpace(req.Query); trimmed != "" {
		querySearchConfig := requestSearchConfig(req, p.cfg.SearchConfig)
		q = q.ColumnExpr(
			"ts_rank_cd(search_vector, websearch_to_tsquery((CASE WHEN ? <> '' THEN ? ELSE search_config END)::regconfig, ?)) AS search_rank",
			querySearchConfig,
			querySearchConfig,
			trimmed,
		)
		q = q.ColumnExpr(
			"GREATEST(similarity(title, ?), similarity(summary, ?), similarity(body, ?)) AS trigram_score",
			trimmed, trimmed, trimmed,
		)
		q = q.ColumnExpr(
			"(ts_rank_cd(search_vector, websearch_to_tsquery((CASE WHEN ? <> '' THEN ? ELSE search_config END)::regconfig, ?)) * 10.0) + GREATEST(similarity(title, ?), similarity(summary, ?), similarity(body, ?)) AS combined_score",
			querySearchConfig, querySearchConfig, trimmed, trimmed, trimmed, trimmed,
		)
		q = q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("search_vector @@ websearch_to_tsquery((CASE WHEN ? <> '' THEN ? ELSE search_config END)::regconfig, ?)", querySearchConfig, querySearchConfig, trimmed).
				WhereOr("similarity(title, ?) >= ?", trimmed, p.cfg.TrigramThreshold).
				WhereOr("similarity(summary, ?) >= ?", trimmed, p.cfg.TrigramThreshold).
				WhereOr("similarity(body, ?) >= ?", trimmed, p.cfg.TrigramThreshold)
		})
		q = q.OrderExpr("combined_score DESC, document_id ASC")
	} else {
		q = q.ColumnExpr("0::double precision AS search_rank")
		q = q.ColumnExpr("0::double precision AS trigram_score")
		q = q.ColumnExpr("1::double precision AS combined_score")
		q = q.OrderExpr("document_id ASC")
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func (p *Provider) suggestRows(ctx context.Context, req types.SuggestRequest) ([]documentModel, error) {
	rows := []documentModel{}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, nil
	}
	q := p.db.NewSelect().
		Model(&rows).
		Where("index_name IN (?)", bun.List(req.Indexes)).
		ColumnExpr("0::double precision AS search_rank").
		ColumnExpr(
			"GREATEST(similarity(title, ?), similarity(COALESCE(fields->>'parent_title', ''), ?)) AS trigram_score",
			query, query,
		).
		ColumnExpr(
			"GREATEST(similarity(title, ?), similarity(COALESCE(fields->>'parent_title', ''), ?)) AS combined_score",
			query, query,
		).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.
				Where("title ILIKE ?", "%"+query+"%").
				WhereOr("COALESCE(fields->>'parent_title', '') ILIKE ?", "%"+query+"%").
				WhereOr("similarity(title, ?) >= ?", query, p.cfg.SuggestTrigramThreshold).
				WhereOr("similarity(COALESCE(fields->>'parent_title', ''), ?) >= ?", query, p.cfg.SuggestTrigramThreshold)
		}).
		OrderExpr("combined_score DESC, title ASC")
	if req.Locale != "" {
		q = q.Where("(locale = '' OR locale = ?)", req.Locale)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	q = q.Limit(limit * 4)
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func scoredDocument(row documentModel, query string, doc types.Document) (float64, bool) {
	if strings.TrimSpace(query) == "" {
		return 1, true
	}
	score := row.CombinedScore
	if score <= 0 {
		score = row.SearchRank*10 + row.TrigramScore
	}
	if score > 0 {
		return score, true
	}
	return scoreDocument(query, doc)
}

func validateSearchRequest(req types.SearchRequest) error {
	switch req.Mode {
	case "", types.SearchModeLexical:
		return nil
	case types.SearchModeSemantic:
		return errs.UnsupportedCapability("semantic_search", map[string]any{"mode": req.Mode})
	case types.SearchModeHybrid:
		return errs.UnsupportedCapability("hybrid_search", map[string]any{"mode": req.Mode})
	default:
		return errs.UnsupportedCapability("search_mode", map[string]any{"mode": req.Mode})
	}
}

func scoreDocument(query string, doc types.Document) (float64, bool) {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return 1, true
	}
	score := 0.0
	if strings.Contains(strings.ToLower(doc.Title), query) {
		score += 5
	}
	if strings.Contains(strings.ToLower(doc.Summary), query) {
		score += 2
	}
	if strings.Contains(strings.ToLower(doc.Body), query) {
		score += 3
	}
	if score == 0 {
		return 0, false
	}
	return score, true
}

func buildFacets(req types.SearchRequest, docs map[string]map[string]types.Document) []types.SearchFacet {
	if len(req.Facets) == 0 {
		return nil
	}
	out := make([]types.SearchFacet, 0, len(req.Facets))
	for _, facetReq := range req.Facets {
		filterExpr := req.Filters
		if facetReq.Disjunctive {
			filterExpr = types.RemoveFacetFilter(filterExpr, facetReq.Field)
		}
		counts := map[string]int{}
		for _, index := range req.Indexes {
			for _, doc := range docs[index] {
				if !matchesScope(req.Scope, doc) || !matchesLocale(req, doc) || !matchesFilter(filterExpr, doc) {
					continue
				}
				for _, value := range doc.Facets[facetReq.Field] {
					counts[value]++
				}
			}
		}
		out = append(out, types.BuildFacet(facetReq, counts, types.SelectedFacetValues(req.Filters, facetReq.Field)))
	}
	return out
}

func matchesScope(scope types.Scope, doc types.Document) bool {
	if scope.TenantID != "" && doc.Scope.TenantID != "" && scope.TenantID != doc.Scope.TenantID {
		return false
	}
	if scope.OrgID != "" && doc.Scope.OrgID != "" && scope.OrgID != doc.Scope.OrgID {
		return false
	}
	for key, want := range scope.Labels {
		if got, ok := doc.Scope.Labels[key]; !ok || got != want {
			return false
		}
	}
	return true
}

func matchesLocale(req types.SearchRequest, doc types.Document) bool {
	if req.Locale == "" && len(req.Locales) == 0 {
		return true
	}
	if doc.Locale == "" {
		return true
	}
	if req.Locale != "" && strings.EqualFold(req.Locale, doc.Locale) {
		return true
	}
	if len(req.Locales) == 0 {
		return false
	}
	return slices.Contains(req.Locales, doc.Locale)
}

func localeMatchLabel(req types.SearchRequest, got string) string {
	switch {
	case strings.TrimSpace(got) == "":
		return "any"
	case isExactLocaleMatch(req.Locale, got):
		return "exact"
	case slices.Contains(req.Locales, got):
		return "fallback"
	default:
		return "none"
	}
}

func isExactLocaleMatch(requested, got string) bool {
	if strings.TrimSpace(requested) == "" || strings.TrimSpace(got) == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(requested), strings.TrimSpace(got))
}

func matchesFilter(expr types.FilterExpr, doc types.Document) bool {
	if expr == nil {
		return true
	}
	switch v := expr.(type) {
	case types.AndExpr:
		for _, term := range v.Terms {
			if !matchesFilter(term, doc) {
				return false
			}
		}
		return true
	case types.OrExpr:
		for _, term := range v.Terms {
			if matchesFilter(term, doc) {
				return true
			}
		}
		return false
	case types.NotExpr:
		return !matchesFilter(v.Term, doc)
	case types.TermExpr:
		return termMatches(v, doc)
	case types.RangeExpr:
		value, ok := doc.Numeric[v.Field]
		if !ok {
			return false
		}
		if gte, ok := asFloat(v.GTE); ok && value < gte {
			return false
		}
		if lte, ok := asFloat(v.LTE); ok && value > lte {
			return false
		}
		return true
	case types.ExistsExpr:
		return fieldExists(doc, v.Field) == v.Exists
	default:
		return false
	}
}

func fieldExists(doc types.Document, field string) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	switch field {
	case "id":
		return strings.TrimSpace(doc.ID) != ""
	case "index":
		return strings.TrimSpace(doc.Index) != ""
	case "registration_key":
		return strings.TrimSpace(doc.RegistrationKey) != ""
	case "type":
		return strings.TrimSpace(doc.Type) != ""
	case "parent_id":
		return strings.TrimSpace(doc.ParentID) != ""
	case "source_type":
		return strings.TrimSpace(doc.SourceType) != ""
	case "source_id":
		return strings.TrimSpace(doc.SourceID) != ""
	case "source_version":
		return strings.TrimSpace(doc.SourceVersion) != ""
	case "title":
		return strings.TrimSpace(doc.Title) != ""
	case "summary":
		return strings.TrimSpace(doc.Summary) != ""
	case "body":
		return strings.TrimSpace(doc.Body) != ""
	case "url":
		return strings.TrimSpace(doc.URL) != ""
	case "anchor_url":
		return strings.TrimSpace(doc.AnchorURL) != ""
	case "locale":
		return strings.TrimSpace(doc.Locale) != ""
	case "start_ms":
		return doc.StartMS != nil
	case "end_ms":
		return doc.EndMS != nil
	case "scope_tenant_id":
		return strings.TrimSpace(doc.Scope.TenantID) != ""
	case "scope_org_id":
		return strings.TrimSpace(doc.Scope.OrgID) != ""
	case "scope_labels":
		return len(doc.Scope.Labels) > 0
	case "visibility_public":
		return true
	case "visibility_roles":
		return len(doc.Visibility.Roles) > 0
	case "visibility_permissions":
		return len(doc.Visibility.Permissions) > 0
	case "visibility_status":
		return strings.TrimSpace(doc.Visibility.Status) != ""
	}
	if values, ok := doc.Facets[field]; ok {
		return len(values) > 0
	}
	if _, ok := doc.Numeric[field]; ok {
		return true
	}
	if _, ok := doc.Booleans[field]; ok {
		return true
	}
	_, ok := doc.Fields[field]
	return ok
}

func termMatches(term types.TermExpr, doc types.Document) bool {
	values := doc.Facets[term.Field]
	if len(values) == 0 {
		if field, ok := doc.Fields[term.Field]; ok {
			values = []string{strings.TrimSpace(toString(field))}
		}
	}
	switch term.Op {
	case types.FilterOpNEQ:
		want := strings.TrimSpace(strings.ToLower(toString(term.Value)))
		for _, value := range values {
			if strings.EqualFold(strings.TrimSpace(value), want) {
				return false
			}
		}
		return len(values) > 0
	case types.FilterOpIn:
		return inMatches(values, term.Value)
	}
	want := strings.TrimSpace(strings.ToLower(toString(term.Value)))
	for _, value := range values {
		got := strings.ToLower(strings.TrimSpace(value))
		switch term.Op {
		case types.FilterOpEQ:
			if got == want {
				return true
			}
		case types.FilterOpContains:
			if strings.Contains(got, want) {
				return true
			}
		}
	}
	return false
}

func inMatches(values []string, raw any) bool {
	wanted := map[string]struct{}{}
	switch list := raw.(type) {
	case []string:
		for _, item := range list {
			wanted[strings.ToLower(strings.TrimSpace(item))] = struct{}{}
		}
	case []any:
		for _, item := range list {
			wanted[strings.ToLower(strings.TrimSpace(toString(item)))] = struct{}{}
		}
	default:
		wanted[strings.ToLower(strings.TrimSpace(toString(raw)))] = struct{}{}
	}
	for _, value := range values {
		if _, ok := wanted[strings.ToLower(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func toHit(doc types.Document, score float64, req types.SearchRequest) types.SearchHit {
	hit := types.SearchHit{
		ID:         doc.ID,
		Type:       doc.Type,
		Title:      doc.Title,
		Summary:    doc.Summary,
		URL:        doc.URL,
		Locale:     doc.Locale,
		Score:      score,
		BaseScore:  score,
		FinalScore: score,
		Fields:     cloneMap(doc.Fields),
		Document:   &doc,
		Retrieval: &types.AppliedRetrievalSignals{
			Mode:          types.SearchModeLexical,
			ProviderScore: &score,
			LexicalScore:  &score,
			Metadata: map[string]any{
				"locale_match":   localeMatchLabel(req, doc.Locale),
				"exact_locale":   isExactLocaleMatch(req.Locale, doc.Locale),
				"postgres_hit":   true,
				"transcript_hit": doc.Type == types.DocumentTypeTranscriptSegment,
			},
		},
	}
	if doc.ParentID != "" {
		hit.Parent = searchParent(doc)
	}
	if doc.StartMS != nil || doc.EndMS != nil || strings.TrimSpace(doc.AnchorURL) != "" {
		hit.Anchor = &types.MediaAnchor{
			ParentID:   firstNonEmptyString(doc.ParentID, doc.ID),
			ParentType: parentType(doc),
			URL:        firstNonEmptyString(doc.AnchorURL, doc.URL),
		}
		if doc.StartMS != nil {
			hit.Anchor.StartMS = *doc.StartMS
		}
		if doc.EndMS != nil {
			hit.Anchor.EndMS = *doc.EndMS
		}
	}
	if doc.Body != "" {
		hit.Snippet = &types.SearchSnippet{Text: doc.Body, Highlighted: doc.Body}
	}
	return hit
}

func sortHits(hits []types.SearchHit, req types.SearchRequest) {
	sorts := req.Sort
	sort.SliceStable(hits, func(i, j int) bool {
		leftExact := isExactLocaleMatch(req.Locale, hits[i].Locale)
		rightExact := isExactLocaleMatch(req.Locale, hits[j].Locale)
		if leftExact != rightExact {
			return leftExact
		}
		if ordered, ok := compareRequestedSorts(sorts, hits[i], hits[j]); ok {
			return ordered
		}
		if hits[i].FinalScore == hits[j].FinalScore {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].FinalScore > hits[j].FinalScore
	})
}

func compareRequestedSorts(sorts []types.Sort, left, right types.SearchHit) (bool, bool) {
	for _, sortField := range sorts {
		if strings.TrimSpace(sortField.Field) == "" {
			continue
		}
		leftNum, rightNum, numeric := sortableNumbers(left, right, sortField.Field)
		if numeric {
			if leftNum == rightNum {
				continue
			}
			if sortField.Direction == types.SortAsc {
				return leftNum < rightNum, true
			}
			return leftNum > rightNum, true
		}
		leftText := fieldValue(left, sortField.Field)
		rightText := fieldValue(right, sortField.Field)
		if leftText == rightText {
			continue
		}
		if sortField.Direction == types.SortAsc {
			return leftText < rightText, true
		}
		return leftText > rightText, true
	}
	return false, false
}

func sortableNumbers(left, right types.SearchHit, field string) (float64, float64, bool) {
	leftValue, leftOK := sortableNumber(left, field)
	rightValue, rightOK := sortableNumber(right, field)
	if !leftOK && !rightOK {
		return 0, 0, false
	}
	return leftValue, rightValue, true
}

func sortableNumber(hit types.SearchHit, field string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "start_ms":
		if hit.Anchor != nil {
			return float64(hit.Anchor.StartMS), true
		}
	case "end_ms":
		if hit.Anchor != nil {
			return float64(hit.Anchor.EndMS), true
		}
	}
	for _, source := range []map[string]any{hit.Fields, documentFields(hit.Document)} {
		if len(source) == 0 {
			continue
		}
		if raw, ok := source[field]; ok {
			if value, ok := asFloat(raw); ok {
				return value, true
			}
		}
	}
	if hit.Document != nil && hit.Document.Numeric != nil {
		if value, ok := hit.Document.Numeric[field]; ok {
			return value, true
		}
	}
	return 0, false
}

func fieldValue(hit types.SearchHit, field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "title":
		return strings.ToLower(strings.TrimSpace(hit.Title))
	case "locale":
		return strings.ToLower(strings.TrimSpace(hit.Locale))
	default:
		for _, source := range []map[string]any{hit.Fields, documentFields(hit.Document)} {
			if len(source) == 0 {
				continue
			}
			if raw, ok := source[field]; ok {
				return strings.ToLower(strings.TrimSpace(toString(raw)))
			}
		}
		return ""
	}
}

func documentFields(doc *types.Document) map[string]any {
	if doc == nil {
		return nil
	}
	return doc.Fields
}

func suggestHit(doc types.Document, preferParent bool) types.SuggestHit {
	parent := searchParent(doc)
	if preferParent && parent != nil {
		return types.SuggestHit{
			ID:       parent.ID,
			Type:     parent.Type,
			Title:    parent.Title,
			URL:      parent.URL,
			Locale:   doc.Locale,
			Score:    1,
			Parent:   parent,
			Document: &doc,
		}
	}
	return types.SuggestHit{
		ID:       doc.ID,
		Type:     doc.Type,
		Title:    doc.Title,
		URL:      doc.URL,
		Locale:   doc.Locale,
		Score:    1,
		Parent:   parent,
		Document: &doc,
	}
}

func searchParent(doc types.Document) *types.SearchParent {
	if doc.ParentID == "" {
		return nil
	}
	parent := &types.SearchParent{
		ID:    doc.ParentID,
		Type:  parentType(doc),
		Title: doc.Title,
		URL:   doc.URL,
	}
	if title, ok := doc.Fields["parent_title"]; ok && strings.TrimSpace(toString(title)) != "" {
		parent.Title = toString(title)
	}
	if url, ok := doc.Fields["parent_url"]; ok && strings.TrimSpace(toString(url)) != "" {
		parent.URL = toString(url)
	}
	if thumbnail, ok := doc.Fields["parent_thumbnail"]; ok {
		parent.Thumbnail = toString(thumbnail)
	}
	return parent
}

func parentType(doc types.Document) string {
	if value, ok := doc.Fields["parent_type"]; ok {
		if out := strings.TrimSpace(toString(value)); out != "" {
			return out
		}
	}
	if doc.ParentID != "" {
		return types.DocumentTypeVideo
	}
	if strings.TrimSpace(doc.Type) != "" {
		return doc.Type
	}
	return types.DocumentTypeVideo
}

func paginateHits(hits []types.SearchHit, page, perPage int) []types.SearchHit {
	if perPage <= 0 {
		return hits
	}
	start := (page - 1) * perPage
	if start >= len(hits) || start < 0 {
		return nil
	}
	end := min(start+perPage, len(hits))
	return hits[start:end]
}

func paginateGroups(groups []types.SearchGroup, page, perPage int) []types.SearchGroup {
	if perPage <= 0 {
		return groups
	}
	start := (page - 1) * perPage
	if start >= len(groups) || start < 0 {
		return nil
	}
	end := min(start+perPage, len(groups))
	return groups[start:end]
}

func flattenGroupHits(groups []types.SearchGroup) []types.SearchHit {
	out := []types.SearchHit{}
	for _, group := range groups {
		out = append(out, group.Hits...)
	}
	return out
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxFloat(values ...float64) float64 {
	best := 0.0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

func orderedHealthIndexes(requested []string, ensured map[string]types.IndexDefinition, counts map[string]int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(requested)+len(ensured)+len(counts))
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, name := range requested {
		appendName(name)
	}
	ensuredNames := make([]string, 0, len(ensured))
	for name := range ensured {
		ensuredNames = append(ensuredNames, name)
	}
	sort.Strings(ensuredNames)
	for _, name := range ensuredNames {
		appendName(name)
	}
	countNames := make([]string, 0, len(counts))
	for name := range counts {
		countNames = append(countNames, name)
	}
	sort.Strings(countNames)
	for _, name := range countNames {
		appendName(name)
	}
	return out
}

func facetDocumentKey(doc types.Document) string {
	return firstNonEmptyString(strings.TrimSpace(doc.RegistrationKey), "_default") + "\x00" + strings.TrimSpace(doc.ID)
}

func requestSearchConfig(req types.SearchRequest, fallback string) string {
	if req.Metadata != nil {
		for _, key := range []string{"search_config", "locale_analyzer"} {
			if value, ok := req.Metadata[key]; ok {
				return normalizeSearchConfig(toString(value), fallback)
			}
		}
	}
	return ""
}
