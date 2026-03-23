package memory

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/ranking"
)

type Provider struct {
	mu      sync.RWMutex
	indexes map[string]types.IndexDefinition
	docs    map[string]map[string]types.Document
	clock   types.Clock
}

type Config struct {
	Clock types.Clock
}

func New(cfg Config) *Provider {
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return &Provider{
		indexes: map[string]types.IndexDefinition{},
		docs:    map[string]map[string]types.Document{},
		clock:   cfg.Clock,
	}
}

func (p *Provider) Name() string { return "memory" }

func (p *Provider) Capabilities(context.Context) (types.CapabilitySet, error) {
	return types.CapabilitySet{
		Facets:               true,
		HierarchicalFacets:   true,
		DisjunctiveFacets:    true,
		Grouping:             true,
		Highlighting:         true,
		Snippets:             true,
		PrefixSearch:         true,
		SupportedSearchModes: []types.SearchMode{types.SearchModeLexical},
		Limitations: []types.CapabilityLimitation{
			{
				Capability: "range_facets",
				Message:    "memory provider supports range filtering but does not compute dedicated numeric/date range facet buckets",
			},
		},
	}, nil
}

func (p *Provider) EnsureIndex(_ context.Context, def types.IndexDefinition) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.indexes[def.Name] = def
	if _, ok := p.docs[def.Name]; !ok {
		p.docs[def.Name] = map[string]types.Document{}
	}
	return nil
}

func (p *Provider) UpsertDocuments(_ context.Context, index string, docs []types.Document) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.docs[index]; !ok {
		p.docs[index] = map[string]types.Document{}
	}
	for _, doc := range docs {
		doc.Index = index
		p.docs[index][internalDocumentKey(doc)] = doc.Clone()
	}
	return nil
}

func (p *Provider) ReplaceDocuments(_ context.Context, index, registrationKey string, sourceIDs []string, docs []types.Document) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.docs[index]; !ok {
		p.docs[index] = map[string]types.Document{}
	}
	sourceSet := map[string]struct{}{}
	for _, id := range sourceIDs {
		sourceSet[id] = struct{}{}
	}
	if len(sourceSet) > 0 {
		for id, doc := range p.docs[index] {
			if !registrationMatches(doc, registrationKey) {
				continue
			}
			if _, ok := sourceSet[doc.SourceID]; ok {
				delete(p.docs[index], id)
			}
		}
	}
	for _, doc := range docs {
		doc.Index = index
		doc.RegistrationKey = firstNonEmptyString(doc.RegistrationKey, registrationKey)
		p.docs[index][internalDocumentKey(doc)] = doc.Clone()
	}
	return nil
}

func (p *Provider) DeleteDocuments(_ context.Context, index string, ids []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	want := map[string]struct{}{}
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			want[trimmed] = struct{}{}
		}
	}
	for key, doc := range p.docs[index] {
		if _, ok := want[doc.ID]; ok {
			delete(p.docs[index], key)
		}
	}
	return nil
}

func (p *Provider) DeleteBySource(_ context.Context, index, registrationKey string, sourceIDs []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	sourceSet := map[string]struct{}{}
	for _, id := range sourceIDs {
		sourceSet[id] = struct{}{}
	}
	for id, doc := range p.docs[index] {
		if !registrationMatches(doc, registrationKey) {
			continue
		}
		if _, ok := sourceSet[doc.SourceID]; ok {
			delete(p.docs[index], id)
		}
	}
	return nil
}

func internalDocumentKey(doc types.Document) string {
	return firstNonEmptyString(strings.TrimSpace(doc.RegistrationKey), "_default") + "\x00" + strings.TrimSpace(doc.ID)
}

func registrationMatches(doc types.Document, registrationKey string) bool {
	return strings.TrimSpace(doc.RegistrationKey) == strings.TrimSpace(registrationKey)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (p *Provider) Search(_ context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := validateSearchRequest(req); err != nil {
		return types.SearchResultPage{}, err
	}
	started := p.clock.Now()
	hits := []types.SearchHit{}
	for _, index := range req.Indexes {
		for _, doc := range p.docs[index] {
			if !matchesScope(req.Scope, doc) || !matchesLocale(req, doc) || !matchesFilter(req.Filters, doc) {
				continue
			}
			score, ok := scoreDocument(req.Query, doc)
			if !ok {
				continue
			}
			hits = append(hits, toHit(doc, score, req))
		}
	}
	sortHits(hits, req)
	page := types.SearchResultPage{
		Hits:       paginateHits(hits, req.Page, req.PerPage),
		Facets:     buildFacets(req, p.docs),
		Page:       req.Page,
		PerPage:    req.PerPage,
		Total:      len(hits),
		DurationMS: p.clock.Now().Sub(started).Milliseconds(),
	}
	if req.GroupBy != "" {
		page.Groups = paginateGroups(ranking.GroupHits(hits), req.Page, req.PerPage)
		page.Total = len(ranking.GroupHits(hits))
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

func (p *Provider) Suggest(_ context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	items := []types.SuggestHit{}
	seen := map[string]struct{}{}
	for _, index := range req.Indexes {
		for _, doc := range p.docs[index] {
			if !matchesScope(req.Scope, doc) {
				continue
			}
			if req.Locale != "" && doc.Locale != "" && req.Locale != doc.Locale {
				continue
			}
			if !strings.Contains(strings.ToLower(doc.Title), strings.ToLower(req.Query)) {
				continue
			}
			item := suggestHit(doc, req.PreferParent)
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			items = append(items, item)
		}
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

func (p *Provider) Health(_ context.Context, req types.HealthRequest) (types.HealthStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	indexes := make([]types.IndexHealth, 0, len(p.docs))
	requested := make(map[string]struct{}, len(req.Indexes))
	for _, name := range req.Indexes {
		requested[name] = struct{}{}
	}
	for name, docs := range p.docs {
		if len(requested) > 0 {
			if _, ok := requested[name]; !ok {
				continue
			}
		}
		indexes = append(indexes, types.IndexHealth{Name: name, Ready: true, Documents: len(docs)})
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		return indexes[i].Name < indexes[j].Name
	})
	return types.HealthStatus{
		Provider:  p.Name(),
		Healthy:   true,
		CheckedAt: p.clock.Now(),
		Indexes:   indexes,
	}, nil
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
		_, fieldExists := doc.Fields[v.Field]
		return fieldExists == v.Exists
	default:
		return false
	}
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
				"memory_hit":     true,
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
				if _, ok := scoreDocument(req.Query, doc); !ok {
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

func documentFields(doc *types.Document) map[string]any {
	if doc == nil {
		return nil
	}
	return doc.Fields
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
