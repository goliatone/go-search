package goadmin

import (
	"fmt"
	"strings"

	"github.com/goliatone/go-search/pkg/types"
)

type GlobalSearchResult struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Thumbnail   string `json:"thumbnail,omitempty"`
}

type SiteSearchRequest struct {
	Query    string              `json:"q"`
	Locale   string              `json:"locale,omitempty"`
	Page     int                 `json:"page,omitempty"`
	PerPage  int                 `json:"per_page,omitempty"`
	Sort     string              `json:"sort,omitempty"`
	Filters  map[string][]string `json:"filters,omitempty"`
	Actor    any                 `json:"actor,omitempty"`
	Request  any                 `json:"request,omitempty"`
	Metadata map[string]any      `json:"metadata,omitempty"`
}

type SiteSearchResultPage struct {
	Hits     []SiteSearchHit   `json:"hits"`
	Facets   []SiteSearchFacet `json:"facets,omitempty"`
	Page     int               `json:"page"`
	PerPage  int               `json:"per_page"`
	Total    int               `json:"total"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

type SiteSearchHit struct {
	ID       string         `json:"id"`
	Type     string         `json:"type,omitempty"`
	Title    string         `json:"title,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	URL      string         `json:"url,omitempty"`
	Locale   string         `json:"locale,omitempty"`
	Score    float64        `json:"score,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
	Snippet  string         `json:"snippet,omitempty"`
	ParentID string         `json:"parent_id,omitempty"`
}

type SiteSearchFacet struct {
	Name    string                `json:"name"`
	Buckets []SiteSearchFacetTerm `json:"buckets,omitempty"`
}

type SiteSearchFacetTerm struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type SiteSuggestRequest struct {
	Query    string              `json:"q"`
	Locale   string              `json:"locale,omitempty"`
	Limit    int                 `json:"limit,omitempty"`
	Filters  map[string][]string `json:"filters,omitempty"`
	Actor    any                 `json:"actor,omitempty"`
	Request  any                 `json:"request,omitempty"`
	Metadata map[string]any      `json:"metadata,omitempty"`
}

type SiteSuggestResult struct {
	Suggestions []string       `json:"suggestions"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func ToSearchRequest(indexes []string, req SiteSearchRequest) types.SearchRequest {
	return types.SearchRequest{
		Indexes: indexesFromMetadata(indexes, req.Metadata),
		Query:   strings.TrimSpace(req.Query),
		Locale:  strings.TrimSpace(req.Locale),
		Page:    positiveOr(req.Page, 1),
		PerPage: positiveOr(req.PerPage, 10),
		Sort:    parseSort(req.Sort),
		Filters: filterExprFromMap(req.Filters),
		Metadata: cloneMetadata(req.Metadata),
		Request: req.Request,
	}
}

func ToSuggestRequest(indexes []string, req SiteSuggestRequest) types.SuggestRequest {
	return types.SuggestRequest{
		Indexes:  indexesFromMetadata(indexes, req.Metadata),
		Query:    strings.TrimSpace(req.Query),
		Locale:   strings.TrimSpace(req.Locale),
		Limit:    positiveOr(req.Limit, 8),
		Metadata: cloneMetadata(req.Metadata),
		Actor:    types.ActorRef{},
	}
}

func GlobalResultsFromPage(page types.SearchResultPage, fallbackType string) []GlobalSearchResult {
	hits := page.Hits
	if len(page.Groups) > 0 {
		hits = make([]types.SearchHit, 0, len(page.Groups))
		for _, group := range page.Groups {
			if group.TopHit != nil {
				hits = append(hits, *group.TopHit)
			}
		}
	}
	results := make([]GlobalSearchResult, 0, len(hits))
	for _, hit := range hits {
		title := strings.TrimSpace(hit.Title)
		if title == "" && hit.Parent != nil {
			title = strings.TrimSpace(hit.Parent.Title)
		}
		thumbnail := ""
		if hit.Parent != nil {
			thumbnail = hit.Parent.Thumbnail
		}
		results = append(results, GlobalSearchResult{
			Type:        firstNonEmpty(hit.Type, fallbackType),
			ID:          hit.ID,
			Title:       title,
			Description: firstNonEmpty(hit.Summary, snippetText(hit)),
			URL:         firstNonEmpty(hit.URL, parentURL(hit)),
			Thumbnail:   thumbnail,
		})
	}
	return results
}

func SiteResultFromPage(page types.SearchResultPage) SiteSearchResultPage {
	out := SiteSearchResultPage{
		Hits:     make([]SiteSearchHit, 0, len(page.Hits)),
		Facets:   make([]SiteSearchFacet, 0, len(page.Facets)),
		Page:     page.Page,
		PerPage:  page.PerPage,
		Total:    page.Total,
		Metadata: cloneMetadata(page.Metadata),
	}
	for _, facet := range page.Facets {
		item := SiteSearchFacet{Name: facet.Field, Buckets: make([]SiteSearchFacetTerm, 0, len(facet.Values))}
		for _, value := range facet.Values {
			item.Buckets = append(item.Buckets, SiteSearchFacetTerm{Value: value.Value, Count: value.Count})
		}
		out.Facets = append(out.Facets, item)
	}
	for _, hit := range page.Hits {
		item := SiteSearchHit{
			ID:       hit.ID,
			Type:     hit.Type,
			Title:    firstNonEmpty(hit.Title, parentTitle(hit)),
			Summary:  hit.Summary,
			URL:      firstNonEmpty(hit.URL, parentURL(hit)),
			Locale:   hit.Locale,
			Score:    hit.Score,
			Fields:   cloneMetadata(hit.Fields),
			Snippet:  snippetText(hit),
			ParentID: parentID(hit),
		}
		if item.Fields == nil {
			item.Fields = map[string]any{}
		}
		if hit.Anchor != nil {
			item.Fields["anchor"] = hit.Anchor
		}
		if hit.Ranking != nil {
			item.Fields["ranking"] = hit.Ranking
		}
		out.Hits = append(out.Hits, item)
	}
	if len(page.Groups) > 0 {
		if out.Metadata == nil {
			out.Metadata = map[string]any{}
		}
		out.Metadata["groups"] = page.Groups
	}
	return out
}

func SiteSuggestResultFromSuggest(result types.SuggestResult) SiteSuggestResult {
	suggestions := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		suggestions = append(suggestions, strings.TrimSpace(item.Title))
	}
	return SiteSuggestResult{
		Suggestions: suggestions,
		Metadata:    cloneMetadata(result.Metadata),
	}
}

func filterExprFromMap(filters map[string][]string) types.FilterExpr {
	if len(filters) == 0 {
		return nil
	}
	terms := make([]types.FilterExpr, 0, len(filters))
	for field, values := range filters {
		clean := compact(values)
		switch len(clean) {
		case 0:
			continue
		case 1:
			terms = append(terms, types.TermExpr{Field: strings.TrimSpace(field), Op: types.FilterOpEQ, Value: clean[0]})
		default:
			terms = append(terms, types.TermExpr{Field: strings.TrimSpace(field), Op: types.FilterOpIn, Value: clean})
		}
	}
	switch len(terms) {
	case 0:
		return nil
	case 1:
		return terms[0]
	default:
		return types.AndExpr{Terms: terms}
	}
}

func parseSort(raw string) []types.Sort {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.SplitN(raw, ":", 2)
	sortDef := types.Sort{Field: strings.TrimSpace(parts[0]), Direction: types.SortAsc}
	if len(parts) > 1 && strings.EqualFold(strings.TrimSpace(parts[1]), string(types.SortDesc)) {
		sortDef.Direction = types.SortDesc
	}
	if sortDef.Field == "" {
		return nil
	}
	return []types.Sort{sortDef}
}

func indexesFromMetadata(indexes []string, metadata map[string]any) []string {
	if len(indexes) > 0 {
		return append([]string(nil), indexes...)
	}
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["collections"]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return compact(value)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, strings.TrimSpace(asString(item)))
		}
		return compact(out)
	default:
		return nil
	}
}

func compact(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func snippetText(hit types.SearchHit) string {
	if hit.Snippet != nil && strings.TrimSpace(hit.Snippet.Text) != "" {
		return hit.Snippet.Text
	}
	return ""
}

func parentURL(hit types.SearchHit) string {
	if hit.Parent == nil {
		return ""
	}
	return hit.Parent.URL
}

func parentTitle(hit types.SearchHit) string {
	if hit.Parent == nil {
		return ""
	}
	return hit.Parent.Title
}

func parentID(hit types.SearchHit) string {
	if hit.Parent == nil {
		return ""
	}
	return hit.Parent.ID
}

func asString(value any) string {
	switch raw := value.(type) {
	case string:
		return raw
	default:
		return fmt.Sprint(raw)
	}
}
