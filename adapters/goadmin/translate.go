package goadmin

import (
	"encoding/json"
	"fmt"
	"maps"
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
	Query         string              `json:"q"`
	Locale        string              `json:"locale,omitempty"`
	Page          int                 `json:"page,omitempty"`
	PerPage       int                 `json:"per_page,omitempty"`
	Sort          string              `json:"sort,omitempty"`
	Filters       map[string][]string `json:"filters,omitempty"`
	Ranges        []SiteSearchRange   `json:"ranges,omitempty"`
	Actor         any                 `json:"actor,omitempty"`
	Request       any                 `json:"request,omitempty"`
	Metadata      map[string]any      `json:"metadata,omitempty"`
	Variant       string              `json:"variant,omitempty"`
	MaxCandidates int                 `json:"max_candidates,omitempty"`
}

type SiteSearchRange struct {
	Field string `json:"field"`
	GTE   any    `json:"gte,omitempty"`
	LTE   any    `json:"lte,omitempty"`
}

type SiteSearchResultPage struct {
	Hits          []SiteSearchHit            `json:"hits"`
	Facets        []SiteSearchFacet          `json:"facets,omitempty"`
	Page          int                        `json:"page"`
	PerPage       int                        `json:"per_page"`
	Total         int                        `json:"total"`
	TotalAccuracy string                     `json:"total_accuracy,omitempty"`
	Counts        map[string]SiteSearchCount `json:"counts,omitempty"`
	Metadata      map[string]any             `json:"metadata,omitempty"`
}

type SiteSearchCount struct {
	Value      int    `json:"value"`
	Accuracy   string `json:"accuracy"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type SiteMatchEvidence struct {
	Exact      bool                        `json:"exact"`
	Status     string                      `json:"status,omitempty"`
	Locations  []SiteMatchEvidenceLocation `json:"locations"`
	Diagnostic string                      `json:"diagnostic,omitempty"`
}

type SiteMatchEvidenceLocation struct {
	Location string                    `json:"location"`
	Count    int                       `json:"count"`
	Samples  []SiteMatchEvidenceSample `json:"samples,omitempty"`
}

type SiteMatchEvidenceSample struct {
	DocumentID   string             `json:"document_id"`
	Field        string             `json:"field,omitempty"`
	Locale       string             `json:"locale,omitempty"`
	Snippet      *SiteSearchSnippet `json:"snippet,omitempty"`
	ChunkOrdinal *int               `json:"chunk_ordinal,omitempty"`
	Anchor       *types.MediaAnchor `json:"anchor,omitempty"`
}

type SiteSearchSnippet struct {
	Text        string `json:"text"`
	Highlighted string `json:"highlighted"`
}

type SiteSearchHit struct {
	ID              string             `json:"id"`
	Type            string             `json:"type,omitempty"`
	Title           string             `json:"title,omitempty"`
	Summary         string             `json:"summary,omitempty"`
	URL             string             `json:"url,omitempty"`
	Locale          string             `json:"locale,omitempty"`
	Score           float64            `json:"score,omitempty"`
	Fields          map[string]any     `json:"fields,omitempty"`
	Snippet         string             `json:"snippet,omitempty"`
	Highlighted     string             `json:"highlighted,omitempty"`
	ParentID        string             `json:"parent_id,omitempty"`
	ParentTitle     string             `json:"parent_title,omitempty"`
	ParentURL       string             `json:"parent_url,omitempty"`
	ParentThumbnail string             `json:"parent_thumbnail,omitempty"`
	ParentSummary   string             `json:"parent_summary,omitempty"`
	Anchor          any                `json:"anchor,omitempty"`
	Metadata        map[string]any     `json:"metadata,omitempty"`
	Evidence        *SiteMatchEvidence `json:"match_evidence,omitempty"`
}

type SiteSearchFacet struct {
	Name        string                `json:"name"`
	Kind        string                `json:"kind,omitempty"`
	Disjunctive bool                  `json:"disjunctive,omitempty"`
	Buckets     []SiteSearchFacetTerm `json:"buckets,omitempty"`
	Metadata    map[string]any        `json:"metadata,omitempty"`
}

type SiteSearchFacetTerm struct {
	Value       string         `json:"value"`
	Label       string         `json:"label,omitempty"`
	Count       int            `json:"count"`
	Selected    bool           `json:"selected,omitempty"`
	Path        []string       `json:"path,omitempty"`
	Level       int            `json:"level,omitempty"`
	ParentValue string         `json:"parent_value,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type SiteSuggestRequest struct {
	Query    string              `json:"q"`
	Locale   string              `json:"locale,omitempty"`
	Limit    int                 `json:"limit,omitempty"`
	Filters  map[string][]string `json:"filters,omitempty"`
	Actor    any                 `json:"actor,omitempty"`
	Request  any                 `json:"request,omitempty"`
	Metadata map[string]any      `json:"metadata,omitempty"`
	Variant  string              `json:"variant,omitempty"`
}

type SiteSuggestResult struct {
	Suggestions []string       `json:"suggestions"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func ToSearchRequest(indexes []string, req SiteSearchRequest) types.SearchRequest {
	metadata := cloneMetadata(req.Metadata)
	if variant := strings.TrimSpace(req.Variant); variant != "" {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["search_variant"] = variant
	}
	if req.MaxCandidates > 0 {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["max_candidates"] = req.MaxCandidates
	}
	return types.SearchRequest{
		Indexes:  indexesFromMetadata(indexes, req.Metadata),
		Query:    strings.TrimSpace(req.Query),
		Locale:   strings.TrimSpace(req.Locale),
		Page:     positiveOr(req.Page, 1),
		PerPage:  positiveOr(req.PerPage, 10),
		Sort:     parseSort(req.Sort),
		Filters:  combineFilterExprs(filterExprFromMap(req.Filters), filterExprFromRanges(req.Ranges)),
		Metadata: metadata,
		Actor:    actorRefFromAny(req.Actor),
		Request:  req.Request,
	}
}

func ToSuggestRequest(indexes []string, req SiteSuggestRequest) types.SuggestRequest {
	metadata := cloneMetadata(req.Metadata)
	if variant := strings.TrimSpace(req.Variant); variant != "" {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["search_variant"] = variant
	}
	return types.SuggestRequest{
		Indexes:  indexesFromMetadata(indexes, req.Metadata),
		Query:    strings.TrimSpace(req.Query),
		Locale:   strings.TrimSpace(req.Locale),
		Limit:    positiveOr(req.Limit, 8),
		Metadata: metadata,
		Actor:    actorRefFromAny(req.Actor),
	}
}

func actorRefFromAny(value any) types.ActorRef {
	switch actor := value.(type) {
	case types.ActorRef:
		actor.Metadata = cloneMetadata(actor.Metadata)
		return actor
	case *types.ActorRef:
		if actor == nil {
			return types.ActorRef{}
		}
		out := *actor
		out.Metadata = cloneMetadata(actor.Metadata)
		return out
	}

	payload := actorPayload(value)
	if len(payload) == 0 {
		return types.ActorRef{}
	}
	metadata, _ := payloadMapValue(payload, "metadata")
	metadata = cloneMetadata(metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	for key, item := range payload {
		switch normalizeActorKey(key) {
		case "metadata", "userid", "actorid", "subject", "id", "tenantid", "tenant", "organizationid", "orgid", "organization", "org":
			continue
		default:
			metadata[canonicalActorMetadataKey(key)] = item
		}
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	return types.ActorRef{
		UserID:   actorPayloadString(payload, "user_id", "actor_id", "subject", "id"),
		TenantID: actorPayloadString(payload, "tenant_id", "tenant"),
		OrgID:    actorPayloadString(payload, "organization_id", "org_id", "organization", "org"),
		Metadata: metadata,
	}
}

func actorPayload(value any) map[string]any {
	if value == nil {
		return nil
	}
	if payload, ok := value.(map[string]any); ok {
		out := make(map[string]any, len(payload))
		maps.Copy(out, payload)
		return out
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil
	}
	return out
}

func actorPayloadString(payload map[string]any, keys ...string) string {
	for _, wanted := range keys {
		for key, value := range payload {
			if normalizeActorKey(key) != normalizeActorKey(wanted) {
				continue
			}
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func payloadMapValue(payload map[string]any, wanted string) (map[string]any, bool) {
	for key, value := range payload {
		if normalizeActorKey(key) != normalizeActorKey(wanted) {
			continue
		}
		mapped, ok := value.(map[string]any)
		return mapped, ok
	}
	return nil, false
}

func normalizeActorKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer("_", "", "-", "", ".", "").Replace(value)
}

func canonicalActorMetadataKey(value string) string {
	switch normalizeActorKey(value) {
	case "role":
		return "role"
	case "resourceroles":
		return "resource_roles"
	case "impersonatorid":
		return "impersonator_id"
	case "isimpersonated", "impersonated":
		return "is_impersonated"
	default:
		return value
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
		Hits:          make([]SiteSearchHit, 0, len(page.Hits)),
		Facets:        make([]SiteSearchFacet, 0, len(page.Facets)),
		Page:          page.Page,
		PerPage:       page.PerPage,
		Total:         page.Total,
		TotalAccuracy: string(page.TotalAccuracy),
		Counts:        translateCounts(page.Counts),
		Metadata:      cloneMetadata(page.Metadata),
	}
	for _, facet := range page.Facets {
		item := SiteSearchFacet{
			Name:        facet.Field,
			Kind:        string(facet.Kind),
			Disjunctive: facet.Disjunctive,
			Buckets:     make([]SiteSearchFacetTerm, 0, len(facet.Values)),
			Metadata:    cloneMetadata(facet.Metadata),
		}
		for _, value := range facet.Values {
			item.Buckets = append(item.Buckets, SiteSearchFacetTerm{
				Value:       value.Value,
				Label:       value.Label,
				Count:       value.Count,
				Selected:    value.Selected,
				Path:        append([]string(nil), value.Path...),
				Level:       value.Level,
				ParentValue: value.ParentValue,
				Metadata:    cloneMetadata(value.Metadata),
			})
		}
		out.Facets = append(out.Facets, item)
	}
	for _, hit := range page.Hits {
		metadata := map[string]any{}
		if hit.Ranking != nil {
			metadata["ranking"] = hit.Ranking
		}
		if hit.Retrieval != nil {
			metadata["retrieval"] = hit.Retrieval
		}
		if hit.Document != nil {
			metadata["document"] = hit.Document
		}
		if len(metadata) == 0 {
			metadata = nil
		}
		item := SiteSearchHit{
			ID:              hit.ID,
			Type:            hit.Type,
			Title:           firstNonEmpty(hit.Title, parentTitle(hit)),
			Summary:         hit.Summary,
			URL:             firstNonEmpty(hit.URL, parentURL(hit)),
			Locale:          hit.Locale,
			Score:           hit.Score,
			Fields:          cloneMetadata(hit.Fields),
			Snippet:         snippetText(hit),
			Highlighted:     highlightedSnippet(hit),
			ParentID:        parentID(hit),
			ParentTitle:     parentTitle(hit),
			ParentURL:       parentURL(hit),
			ParentThumbnail: parentThumbnail(hit),
			ParentSummary:   strings.TrimSpace(asString(hit.Fields["parent_summary"])),
			Anchor:          hit.Anchor,
			Metadata:        metadata,
			Evidence:        translateEvidence(hit.Evidence),
		}
		if item.Fields == nil {
			item.Fields = map[string]any{}
		}
		if hit.Anchor != nil {
			item.Fields["anchor"] = hit.Anchor
		}
		if item.ParentTitle != "" {
			item.Fields["parent_title"] = item.ParentTitle
		}
		if item.ParentURL != "" {
			item.Fields["parent_url"] = item.ParentURL
		}
		if item.ParentThumbnail != "" {
			item.Fields["parent_thumbnail"] = item.ParentThumbnail
		}
		if item.ParentSummary != "" {
			item.Fields["parent_summary"] = item.ParentSummary
		}
		if item.Highlighted != "" {
			item.Fields["highlighted"] = item.Highlighted
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

func translateCounts(in map[string]types.SearchCount) map[string]SiteSearchCount {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]SiteSearchCount, len(in))
	for key, count := range in {
		out[key] = SiteSearchCount{Value: count.Value, Accuracy: string(count.Accuracy), Diagnostic: count.Diagnostic}
	}
	return out
}

func translateEvidence(in *types.MatchEvidenceSummary) *SiteMatchEvidence {
	if in == nil {
		return nil
	}
	out := &SiteMatchEvidence{Exact: in.Exact, Status: string(in.Status), Diagnostic: in.Diagnostic, Locations: make([]SiteMatchEvidenceLocation, 0, len(in.Locations))}
	for _, location := range in.Locations {
		mapped := SiteMatchEvidenceLocation{Location: location.Location, Count: location.Count, Samples: make([]SiteMatchEvidenceSample, 0, len(location.Samples))}
		for _, sample := range location.Samples {
			item := SiteMatchEvidenceSample{DocumentID: sample.DocumentID, Field: sample.Field, Locale: sample.Locale, ChunkOrdinal: cloneInt(sample.ChunkOrdinal), Anchor: cloneAnchor(sample.Anchor)}
			if sample.Snippet != nil {
				item.Snippet = &SiteSearchSnippet{Text: sample.Snippet.Text, Highlighted: sample.Snippet.Highlighted}
			}
			mapped.Samples = append(mapped.Samples, item)
		}
		out.Locations = append(out.Locations, mapped)
	}
	return out
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneAnchor(value *types.MediaAnchor) *types.MediaAnchor {
	if value == nil {
		return nil
	}
	out := *value
	return &out
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

func filterExprFromRanges(ranges []SiteSearchRange) types.FilterExpr {
	if len(ranges) == 0 {
		return nil
	}
	terms := make([]types.FilterExpr, 0, len(ranges))
	for _, item := range ranges {
		field := strings.TrimSpace(item.Field)
		if field == "" || (item.GTE == nil && item.LTE == nil) {
			continue
		}
		terms = append(terms, types.RangeExpr{
			Field: field,
			GTE:   item.GTE,
			LTE:   item.LTE,
		})
	}
	return combineFilterExprs(terms...)
}

func combineFilterExprs(exprs ...types.FilterExpr) types.FilterExpr {
	terms := make([]types.FilterExpr, 0, len(exprs))
	for _, expr := range exprs {
		if expr == nil {
			continue
		}
		terms = append(terms, expr)
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
	raw, ok := metadata["indexes"]
	if !ok {
		raw, ok = metadata["collections"]
	}
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

func highlightedSnippet(hit types.SearchHit) string {
	if hit.Snippet != nil && strings.TrimSpace(hit.Snippet.Highlighted) != "" {
		return hit.Snippet.Highlighted
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

func parentThumbnail(hit types.SearchHit) string {
	if hit.Parent == nil {
		return ""
	}
	return hit.Parent.Thumbnail
}

func asString(value any) string {
	switch raw := value.(type) {
	case string:
		return raw
	default:
		return fmt.Sprint(raw)
	}
}
