package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/ranking"
)

type Provider struct {
	mu      sync.RWMutex
	indexes map[string]types.IndexDefinition
	docs    map[string]map[string]types.Document
}

func New() *Provider {
	return &Provider{
		indexes: map[string]types.IndexDefinition{},
		docs:    map[string]map[string]types.Document{},
	}
}

func (p *Provider) Name() string { return "memory" }

func (p *Provider) Capabilities(context.Context) (types.CapabilitySet, error) {
	return types.CapabilitySet{
		Facets:               true,
		Grouping:             true,
		Highlighting:         true,
		Snippets:             true,
		PrefixSearch:         true,
		SupportedSearchModes: []types.SearchMode{types.SearchModeLexical},
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
		p.docs[index][doc.ID] = doc.Clone()
	}
	return nil
}

func (p *Provider) DeleteDocuments(_ context.Context, index string, ids []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range ids {
		delete(p.docs[index], id)
	}
	return nil
}

func (p *Provider) DeleteBySource(_ context.Context, index string, sourceIDs []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	sourceSet := map[string]struct{}{}
	for _, id := range sourceIDs {
		sourceSet[id] = struct{}{}
	}
	for id, doc := range p.docs[index] {
		if _, ok := sourceSet[doc.SourceID]; ok {
			delete(p.docs[index], id)
		}
	}
	return nil
}

func (p *Provider) Search(_ context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	started := time.Now()
	hits := []types.SearchHit{}
	for _, index := range req.Indexes {
		for _, doc := range p.docs[index] {
			if !matchesLocale(req, doc) || !matchesFilter(req.Filters, doc) {
				continue
			}
			score, ok := scoreDocument(req.Query, doc)
			if !ok {
				continue
			}
			hits = append(hits, toHit(doc, score))
		}
	}
	sortHits(hits, req.Sort)
	page := types.SearchResultPage{
		Hits:       paginateHits(hits, req.Page, req.PerPage),
		Facets:     buildFacets(req, hits),
		Page:       req.Page,
		PerPage:    req.PerPage,
		Total:      len(hits),
		DurationMS: time.Since(started).Milliseconds(),
	}
	if req.GroupBy != "" {
		page.Groups = paginateGroups(ranking.GroupHits(hits), req.Page, req.PerPage)
		page.Total = len(ranking.GroupHits(hits))
		page.Hits = flattenGroupHits(page.Groups)
	}
	return page, nil
}

func (p *Provider) Suggest(_ context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	items := []types.SuggestHit{}
	for _, index := range req.Indexes {
		for _, doc := range p.docs[index] {
			if req.Locale != "" && doc.Locale != "" && req.Locale != doc.Locale {
				continue
			}
			if req.PreferParent && doc.ParentID != "" {
				continue
			}
			if !strings.Contains(strings.ToLower(doc.Title), strings.ToLower(req.Query)) {
				continue
			}
			items = append(items, types.SuggestHit{
				ID:     doc.ID,
				Type:   doc.Type,
				Title:  doc.Title,
				URL:    doc.URL,
				Locale: doc.Locale,
				Score:  1,
			})
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

func (p *Provider) Health(_ context.Context) (types.HealthStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	indexes := make([]types.IndexHealth, 0, len(p.docs))
	for name, docs := range p.docs {
		indexes = append(indexes, types.IndexHealth{Name: name, Ready: true, Documents: len(docs)})
	}
	return types.HealthStatus{
		Provider:  p.Name(),
		Healthy:   true,
		CheckedAt: time.Now(),
		Indexes:   indexes,
	}, nil
}

func matchesLocale(req types.SearchRequest, doc types.Document) bool {
	if req.Locale != "" && doc.Locale != "" && req.Locale != doc.Locale {
		return false
	}
	if len(req.Locales) == 0 || doc.Locale == "" {
		return true
	}
	for _, locale := range req.Locales {
		if locale == doc.Locale {
			return true
		}
	}
	return false
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
	want := strings.TrimSpace(strings.ToLower(toString(term.Value)))
	for _, value := range values {
		got := strings.ToLower(strings.TrimSpace(value))
		switch term.Op {
		case types.FilterOpEQ:
			if got == want {
				return true
			}
		case types.FilterOpNEQ:
			if got != want {
				return true
			}
		case types.FilterOpContains:
			if strings.Contains(got, want) {
				return true
			}
		case types.FilterOpIn:
			if got == want {
				return true
			}
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

func toHit(doc types.Document, score float64) types.SearchHit {
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
	}
	if doc.ParentID != "" {
		hit.Parent = &types.SearchParent{
			ID:    doc.ParentID,
			Type:  types.DocumentTypeVideo,
			Title: doc.Title,
			URL:   doc.URL,
		}
	}
	if doc.StartMS != nil && doc.EndMS != nil {
		hit.Anchor = &types.MediaAnchor{
			ParentID: doc.ParentID,
			StartMS:  *doc.StartMS,
			EndMS:    *doc.EndMS,
			URL:      doc.AnchorURL,
		}
	}
	if doc.Body != "" {
		hit.Snippet = &types.SearchSnippet{Text: doc.Body, Highlighted: doc.Body}
	}
	return hit
}

func buildFacets(req types.SearchRequest, hits []types.SearchHit) []types.SearchFacet {
	if len(req.Facets) == 0 {
		return nil
	}
	out := make([]types.SearchFacet, 0, len(req.Facets))
	for _, facetReq := range req.Facets {
		counts := map[string]int{}
		for _, hit := range hits {
			if hit.Document == nil {
				continue
			}
			for _, value := range hit.Document.Facets[facetReq.Field] {
				counts[value]++
			}
		}
		values := make([]types.SearchFacetValue, 0, len(counts))
		for value, count := range counts {
			values = append(values, types.SearchFacetValue{Value: value, Count: count})
		}
		sort.SliceStable(values, func(i, j int) bool {
			if values[i].Count == values[j].Count {
				return values[i].Value < values[j].Value
			}
			return values[i].Count > values[j].Count
		})
		if facetReq.Limit > 0 && len(values) > facetReq.Limit {
			values = values[:facetReq.Limit]
		}
		out = append(out, types.SearchFacet{Field: facetReq.Field, Values: values})
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
	end := start + perPage
	if end > len(hits) {
		end = len(hits)
	}
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
	end := start + perPage
	if end > len(groups) {
		end = len(groups)
	}
	return groups[start:end]
}

func flattenGroupHits(groups []types.SearchGroup) []types.SearchHit {
	out := []types.SearchHit{}
	for _, group := range groups {
		out = append(out, group.Hits...)
	}
	return out
}

func sortHits(hits []types.SearchHit, sorts []types.Sort) {
	if len(sorts) == 0 {
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].FinalScore == hits[j].FinalScore {
				return hits[i].ID < hits[j].ID
			}
			return hits[i].FinalScore > hits[j].FinalScore
		})
		return
	}
	sort.SliceStable(hits, func(i, j int) bool {
		for _, s := range sorts {
			left := fieldValue(hits[i], s.Field)
			right := fieldValue(hits[j], s.Field)
			if left == right {
				continue
			}
			if s.Direction == types.SortAsc {
				return left < right
			}
			return left > right
		}
		return hits[i].ID < hits[j].ID
	})
}

func fieldValue(hit types.SearchHit, field string) string {
	switch field {
	case "title":
		return hit.Title
	case "locale":
		return hit.Locale
	default:
		if hit.Document != nil && hit.Document.Fields != nil {
			return toString(hit.Document.Fields[field])
		}
		return ""
	}
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
	for k, v := range in {
		out[k] = v
	}
	return out
}
