package typesense

import (
	"sort"
	"strconv"
	"strings"

	"github.com/goliatone/go-search/pkg/types"
	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

func mapSearchResult(result *tsapi.SearchResult, runtime managedIndex, req types.SearchRequest, cfg Config) types.SearchResultPage {
	cfg = normalizeConfig(cfg)
	page := types.SearchResultPage{
		Page:       valueOrDefault(resultPage(result), req.Page),
		PerPage:    valueOrDefault(resultPerPage(result), req.PerPage),
		DurationMS: int64(valueOrDefaultInt(resultSearchTime(result), 0)),
		Metadata: map[string]any{
			"provider":               "typesense",
			"collection_name":        runtime.collectionName,
			"schema_hash":            runtime.schemaHash,
			"grouped_evidence_limit": cfg.GroupedEvidenceLimit,
		},
	}
	page.Facets = mapFacets(result, req)

	if req.GroupBy != "" && result != nil && result.GroupedHits != nil {
		page.Groups = make([]types.SearchGroup, 0, len(*result.GroupedHits))
		for _, group := range *result.GroupedHits {
			mapped := mapGroupedHit(group, req)
			page.Groups = append(page.Groups, mapped)
		}
		page.Total = valueOrDefaultInt(resultFound(result), len(page.Groups))
		page.Hits = rankingFlatten(page.Groups)
		return page
	}

	page.Hits = mapResultHits(resultHits(result), req)
	page.Total = valueOrDefaultInt(resultFound(result), len(page.Hits))
	return page
}

func mapFacets(result *tsapi.SearchResult, req types.SearchRequest) []types.SearchFacet {
	if result == nil || result.FacetCounts == nil {
		return nil
	}
	requestByField := map[string]types.FacetRequest{}
	for _, facet := range req.Facets {
		requestByField[facet.Field] = facet
	}
	out := make([]types.SearchFacet, 0, len(*result.FacetCounts))
	for _, facet := range *result.FacetCounts {
		item := types.SearchFacet{}
		if facet.FieldName != nil {
			item.Field = *facet.FieldName
		}
		request := requestByField[item.Field]
		request.Field = item.Field
		counts := map[string]int{}
		if facet.Counts != nil {
			for _, value := range *facet.Counts {
				if value.Value == nil {
					continue
				}
				count := 0
				if value.Count != nil {
					count = *value.Count
				}
				counts[*value.Value] = count
			}
		}
		out = append(out, types.BuildFacet(request, counts, types.SelectedFacetValues(req.Filters, item.Field)))
	}
	return out
}

func mapGroupedHit(group tsapi.SearchGroupedHit, req types.SearchRequest) types.SearchGroup {
	hits := mapResultHits(group.Hits, req)
	groupKey := ""
	if len(group.GroupKey) > 0 {
		groupKey = stringify(group.GroupKey[0])
	}
	out := types.SearchGroup{
		Key:   groupKey,
		Hits:  hits,
		Count: len(hits),
	}
	if group.Found != nil {
		out.Count = *group.Found
	}
	if len(hits) > 0 {
		top := hits[0]
		out.TopHit = &top
		out.Parent = hits[0].Parent
		if out.Parent == nil {
			out.Parent = defaultParent(hits[0])
		}
	}
	return out
}

func mapResultHits(hits []tsapi.SearchResultHit, req types.SearchRequest) []types.SearchHit {
	out := make([]types.SearchHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, mapResultHit(hit, req))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return compareSearchHits(req, out[i], out[j])
	})
	return out
}

func mapResultHit(hit tsapi.SearchResultHit, req types.SearchRequest) types.SearchHit {
	doc := mapDocument(hit.Document)
	baseScore := documentScore(hit)
	out := types.SearchHit{
		ID:         doc.ID,
		Type:       doc.Type,
		Title:      firstNonEmpty(doc.Title, fieldString(doc.Fields, "parent_title")),
		Summary:    doc.Summary,
		URL:        firstNonEmpty(doc.AnchorURL, doc.URL),
		Locale:     doc.Locale,
		Score:      baseScore,
		BaseScore:  baseScore,
		FinalScore: baseScore,
		Parent:     mapParent(doc),
		Anchor:     mapAnchor(doc),
		Snippet:    mapSnippet(hit, doc),
		Document:   &doc,
		Retrieval: &types.AppliedRetrievalSignals{
			Mode:          types.SearchModeLexical,
			ProviderScore: new(baseScore),
			LexicalScore:  new(baseScore),
			Metadata: map[string]any{
				"locale_match":    localeMatchLabel(req.Locale, doc.Locale),
				"exact_locale":    isExactLocaleMatch(req.Locale, doc.Locale),
				"collection_hit":  true,
				"transcript_hit":  doc.Type == types.DocumentTypeTranscriptSegment,
				"typesense_score": baseScore,
			},
		},
	}
	if out.Parent == nil {
		out.Parent = defaultParent(out)
	}
	return out
}

func mapSuggestHits(result *tsapi.SearchResult, req types.SuggestRequest) []types.SuggestHit {
	hits := mapResultHits(resultHits(result), types.SearchRequest{Locale: req.Locale})
	out := make([]types.SuggestHit, 0, len(hits))
	seen := map[string]struct{}{}
	for _, hit := range hits {
		item := types.SuggestHit{
			ID:     hit.ID,
			Type:   hit.Type,
			Title:  hit.Title,
			URL:    hit.URL,
			Locale: hit.Locale,
			Score:  hit.Score,
			Parent: hit.Parent,
		}
		if req.PreferParent && hit.Parent != nil && hit.Parent.ID != "" {
			item.ID = hit.Parent.ID
			item.Type = hit.Parent.Type
			item.Title = firstNonEmpty(hit.Parent.Title, hit.Title)
			item.URL = firstNonEmpty(hit.Parent.URL, hit.URL)
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func mapDocument(raw *map[string]any) types.Document {
	if raw == nil {
		return types.Document{}
	}
	doc := types.Document{
		ID:              firstNonEmpty(fieldStringValue(*raw, "document_id"), fieldStringValue(*raw, "id")),
		Index:           fieldStringValue(*raw, "index"),
		RegistrationKey: fieldStringValue(*raw, "registration_key"),
		Type:            fieldStringValue(*raw, "type"),
		ParentID:        fieldStringValue(*raw, "parent_id"),
		SourceType:      fieldStringValue(*raw, "source_type"),
		SourceID:        fieldStringValue(*raw, "source_id"),
		Title:           fieldStringValue(*raw, "title"),
		Summary:         fieldStringValue(*raw, "summary"),
		Body:            fieldStringValue(*raw, "body"),
		URL:             fieldStringValue(*raw, "url"),
		AnchorURL:       fieldStringValue(*raw, "anchor_url"),
		Locale:          fieldStringValue(*raw, "locale"),
		Fields:          map[string]any{},
		Metadata:        map[string]any{},
		Scope: types.Scope{
			TenantID: fieldStringValue(*raw, "scope_tenant_id"),
			OrgID:    fieldStringValue(*raw, "scope_org_id"),
			Labels:   scopeLabelsFromValue((*raw)["scope_labels"]),
		},
		Visibility: types.Visibility{
			Public:      boolFieldValue(*raw, "visibility_public"),
			Roles:       stringSliceFieldValueOrNil(*raw, "visibility_roles"),
			Permissions: stringSliceFieldValueOrNil(*raw, "visibility_permissions"),
			Status:      fieldStringValue(*raw, "visibility_status"),
		},
	}
	if value, ok := int64FieldValue(*raw, "start_ms"); ok {
		doc.StartMS = &value
		doc.Numeric = ensureNumeric(doc.Numeric)
		doc.Numeric["start_ms"] = float64(value)
	}
	if value, ok := int64FieldValue(*raw, "end_ms"); ok {
		doc.EndMS = &value
		doc.Numeric = ensureNumeric(doc.Numeric)
		doc.Numeric["end_ms"] = float64(value)
	}
	reserved := map[string]struct{}{}
	for _, field := range []string{
		"index",
		"type",
		"parent_id",
		"parent_type",
		"source_type",
		"source_id",
		"title",
		"summary",
		"body",
		"url",
		"anchor_url",
		"locale",
		"start_ms",
		"end_ms",
		"parent_title",
		"parent_summary",
		"parent_url",
		"parent_thumbnail",
		"track_kind",
		"source_format",
		"scope_tenant_id",
		"scope_org_id",
		"scope_labels",
		"visibility_public",
		"visibility_roles",
		"visibility_permissions",
		"visibility_status",
	} {
		reserved[field] = struct{}{}
		if value, ok := (*raw)[field]; ok {
			doc.Fields[field] = value
			doc.Metadata[field] = value
		}
	}
	reserved["id"] = struct{}{}
	reserved["document_id"] = struct{}{}
	reserved["registration_key"] = struct{}{}
	if value, ok := stringSliceFieldValue(*raw, "topic"); ok {
		doc.Facets = map[string][]string{"topic": value}
		reserved["topic"] = struct{}{}
	}
	for field, value := range *raw {
		if _, ok := reserved[field]; ok {
			continue
		}
		switch typed := value.(type) {
		case []string:
			if doc.Facets == nil {
				doc.Facets = map[string][]string{}
			}
			doc.Facets[field] = append([]string(nil), typed...)
			doc.Fields[field] = append([]string(nil), typed...)
			doc.Metadata[field] = append([]string(nil), typed...)
		case []any:
			values := make([]string, 0, len(typed))
			allStrings := true
			for _, item := range typed {
				text, ok := item.(string)
				if !ok {
					allStrings = false
					break
				}
				values = append(values, text)
			}
			if allStrings {
				if doc.Facets == nil {
					doc.Facets = map[string][]string{}
				}
				doc.Facets[field] = append([]string(nil), values...)
				doc.Fields[field] = append([]string(nil), values...)
				doc.Metadata[field] = append([]string(nil), values...)
				continue
			}
			doc.Fields[field] = value
			doc.Metadata[field] = value
		case int, int32, int64, float32, float64:
			doc.Numeric = ensureNumeric(doc.Numeric)
			doc.Numeric[field] = asFloat64(typed)
			doc.Fields[field] = value
			doc.Metadata[field] = value
		default:
			doc.Fields[field] = value
			doc.Metadata[field] = value
		}
	}
	return doc
}

func mapParent(doc types.Document) *types.SearchParent {
	parentID := firstNonEmpty(doc.ParentID, doc.ID)
	parentTitle := firstNonEmpty(fieldString(doc.Fields, "parent_title"), doc.Title)
	parentURL := firstNonEmpty(fieldString(doc.Fields, "parent_url"), doc.URL)
	if parentID == "" && parentTitle == "" && parentURL == "" {
		return nil
	}
	return &types.SearchParent{
		ID:        parentID,
		Type:      parentType(doc),
		Title:     parentTitle,
		URL:       parentURL,
		Thumbnail: fieldString(doc.Fields, "parent_thumbnail"),
	}
}

func defaultParent(hit types.SearchHit) *types.SearchParent {
	return &types.SearchParent{
		ID:    hit.ID,
		Type:  hit.Type,
		Title: hit.Title,
		URL:   hit.URL,
	}
}

func mapAnchor(doc types.Document) *types.MediaAnchor {
	if doc.StartMS == nil && doc.EndMS == nil && doc.AnchorURL == "" {
		return nil
	}
	anchor := &types.MediaAnchor{
		ParentID:   firstNonEmpty(doc.ParentID, doc.ID),
		ParentType: parentType(doc),
		URL:        firstNonEmpty(doc.AnchorURL, doc.URL),
	}
	if doc.StartMS != nil {
		anchor.StartMS = *doc.StartMS
	}
	if doc.EndMS != nil {
		anchor.EndMS = *doc.EndMS
	}
	return anchor
}

func parentType(doc types.Document) string {
	if value := strings.TrimSpace(fieldString(doc.Fields, "parent_type")); value != "" {
		return value
	}
	if strings.TrimSpace(doc.ParentID) != "" {
		return types.DocumentTypeVideo
	}
	if strings.TrimSpace(doc.Type) != "" {
		return doc.Type
	}
	return types.DocumentTypeVideo
}

func mapSnippet(hit tsapi.SearchResultHit, doc types.Document) *types.SearchSnippet {
	if hit.Highlights != nil {
		for _, field := range []string{"body", "summary", "title"} {
			for _, highlight := range *hit.Highlights {
				if highlight.Field == nil || *highlight.Field != field {
					continue
				}
				snippet := firstNonEmpty(ptrString(highlight.Snippet), ptrString(highlight.Value))
				if snippet == "" {
					continue
				}
				return &types.SearchSnippet{
					Text:        highlightTextForField(doc, field),
					Highlighted: snippet,
				}
			}
		}
	}
	return nil
}

func highlightTextForField(doc types.Document, field string) string {
	switch field {
	case "body":
		return doc.Body
	case "summary":
		return doc.Summary
	case "title":
		return doc.Title
	default:
		return fieldString(doc.Fields, field)
	}
}

func documentScore(hit tsapi.SearchResultHit) float64 {
	if hit.TextMatchInfo != nil && hit.TextMatchInfo.Score != nil {
		if parsed, err := strconv.ParseFloat(*hit.TextMatchInfo.Score, 64); err == nil {
			return parsed
		}
	}
	if hit.TextMatch != nil {
		return float64(*hit.TextMatch)
	}
	return 0
}

func localeMatchLabel(requested, got string) string {
	switch {
	case strings.TrimSpace(got) == "":
		return "any"
	case requested == "":
		return "none"
	case isExactLocaleMatch(requested, got):
		return "exact"
	default:
		return "fallback"
	}
}

func isExactLocaleMatch(requested, got string) bool {
	return requested != "" && strings.EqualFold(strings.TrimSpace(requested), strings.TrimSpace(got))
}

func rankingFlatten(groups []types.SearchGroup) []types.SearchHit {
	out := []types.SearchHit{}
	for _, group := range groups {
		out = append(out, group.Hits...)
	}
	return out
}

func resultHits(result *tsapi.SearchResult) []tsapi.SearchResultHit {
	if result == nil || result.Hits == nil {
		return nil
	}
	return *result.Hits
}

func resultFound(result *tsapi.SearchResult) *int {
	if result == nil {
		return nil
	}
	return result.Found
}

func resultPage(result *tsapi.SearchResult) *int {
	if result == nil {
		return nil
	}
	return result.Page
}

func resultPerPage(result *tsapi.SearchResult) *int {
	if result == nil || result.RequestParams == nil {
		return nil
	}
	return &result.RequestParams.PerPage
}

func resultSearchTime(result *tsapi.SearchResult) *int {
	if result == nil {
		return nil
	}
	return result.SearchTimeMs
}

func valueOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func valueOrDefaultInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func fieldString(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	return fieldStringValue(fields, key)
}

func fieldStringValue(fields map[string]any, key string) string {
	value, ok := fields[key]
	if !ok {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return stringify(v)
	}
}

func int64FieldValue(fields map[string]any, key string) (int64, bool) {
	value, ok := fields[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func stringSliceFieldValue(fields map[string]any, key string) ([]string, bool) {
	value, ok := fields[key]
	if !ok {
		return nil, false
	}
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...), true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, stringify(item))
		}
		return out, true
	default:
		return nil, false
	}
}

func stringSliceFieldValueOrNil(fields map[string]any, key string) []string {
	values, ok := stringSliceFieldValue(fields, key)
	if !ok {
		return nil
	}
	return values
}

func boolFieldValue(fields map[string]any, key string) bool {
	value, ok := fields[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func scopeLabelsFromValue(value any) map[string]string {
	switch typed := value.(type) {
	case []string:
		return decodeScopeLabels(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringify(item))
		}
		return decodeScopeLabels(out)
	default:
		return nil
	}
}

func decodeScopeLabels(values []string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, item := range values {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ensureNumeric(in map[string]float64) map[string]float64 {
	if in == nil {
		return map[string]float64{}
	}
	return in
}

func asFloat64(value any) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
