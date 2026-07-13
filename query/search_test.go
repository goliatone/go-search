package query

import (
	"context"
	"testing"

	"github.com/goliatone/go-search/indexing"
	"github.com/goliatone/go-search/locale"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/planner"
	"github.com/goliatone/go-search/providers/memory"
	"github.com/goliatone/go-search/ranking"
)

type staticEditorialStore struct {
	rules []types.EditorialRankRule
}

func TestCandidateWindowIsBounded(t *testing.T) {
	cfg := ranking.CandidateConfig{Multiplier: 5, MaxPerIndex: 250, MaxRefillRounds: 2}
	if got := candidateWindow(types.SearchRequest{Page: 1, PerPage: 20}, cfg); got != 100 {
		t.Fatalf("got %d", got)
	}
	if got := candidateWindow(types.SearchRequest{Page: 9, PerPage: 20}, cfg); got != 250 {
		t.Fatalf("cap got %d", got)
	}
	if got := candidateWindow(types.SearchRequest{Page: 1, PerPage: 20}, ranking.CandidateConfig{Multiplier: 1, MaxPerIndex: 5}); got != 5 {
		t.Fatalf("absolute cap got %d", got)
	}
	maxInt := int(^uint(0) >> 1)
	if got := candidateWindow(types.SearchRequest{Page: maxInt, PerPage: maxInt}, ranking.CandidateConfig{Multiplier: maxInt, MaxPerIndex: 250}); got != 250 {
		t.Fatalf("overflow-safe cap got %d", got)
	}
}
func BenchmarkCandidateWindow(b *testing.B) {
	cfg := ranking.CandidateConfig{Multiplier: 5, MaxPerIndex: 250, MaxRefillRounds: 2}
	req := types.SearchRequest{Page: 3, PerPage: 20}
	for i := 0; i < b.N; i++ {
		_ = candidateWindow(req, cfg)
	}
}

type countingBatchProvider struct {
	*memory.Provider
	searchCalls int
	batchCalls  int
	batchSize   int
}

type pagedCandidateProvider struct {
	*memory.Provider
	hits     map[string][]types.SearchHit
	accuracy map[string]types.TotalAccuracy
	requests []types.SearchRequest
}

type oversizedCandidateProvider struct {
	*memory.Provider
	hits []types.SearchHit
}

func (p *oversizedCandidateProvider) Search(_ context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	return types.SearchResultPage{Hits: append([]types.SearchHit(nil), p.hits...), Page: req.Page, PerPage: req.PerPage, Total: len(p.hits), TotalAccuracy: types.TotalAccuracyExact}, nil
}

func (p *oversizedCandidateProvider) SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
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

func (p *pagedCandidateProvider) Search(_ context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	p.requests = append(p.requests, req)
	index := req.Indexes[0]
	all := p.hits[index]
	start := (max(1, req.Page) - 1) * req.PerPage
	if start > len(all) {
		start = len(all)
	}
	end := min(len(all), start+req.PerPage)
	return types.SearchResultPage{Hits: append([]types.SearchHit(nil), all[start:end]...), Page: req.Page, PerPage: req.PerPage, Total: len(all), TotalAccuracy: p.accuracy[index]}, nil
}

func (p *pagedCandidateProvider) SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
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

func TestSearchCandidatesGroupsEntitiesBeforeCrossIndexRRF(t *testing.T) {
	provider := &pagedCandidateProvider{Provider: memory.New(memory.Config{}), hits: map[string][]types.SearchHit{
		"site":  {{ID: "site-doc", ResultID: "article:42", FinalScore: 10}},
		"media": {{ID: "media-chunk", ResultID: "article:42", FinalScore: 2}},
	}}
	profile := &ranking.RankingProfile{Indexes: map[string]ranking.IndexProfile{"site": {Weight: 1}, "media": {Weight: 1}}}
	search := &Search{provider: provider}
	page, _, err := search.searchCandidates(t.Context(), types.SearchRequest{Indexes: []string{"site", "media"}, Page: 1, PerPage: 10}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ResultID != "article:42" || page.Hits[0].Retrieval == nil || len(page.Hits[0].Retrieval.Contributions) != 2 {
		t.Fatalf("fused page = %#v", page)
	}
}

func TestSearchCandidatesClampsOversizedInitialProviderResult(t *testing.T) {
	provider := &oversizedCandidateProvider{Provider: memory.New(memory.Config{}), hits: []types.SearchHit{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}}}
	profile := &ranking.RankingProfile{Indexes: map[string]ranking.IndexProfile{"site": {Weight: 1}}, Candidates: ranking.CandidateConfig{Multiplier: 1, MaxPerIndex: 2, MaxRefillRounds: 1}}
	search := &Search{provider: provider}
	page, state, err := search.searchCandidates(t.Context(), types.SearchRequest{Indexes: []string{"site"}, Page: 1, PerPage: 2}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 2 || state.indexes[0].fetched != 2 || page.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("oversized provider result = %#v state=%#v", page, state.indexes[0])
	}
}

func TestRefillCandidatesPagesEachIndexIndependentlyAndCapsRetention(t *testing.T) {
	provider := &pagedCandidateProvider{Provider: memory.New(memory.Config{}), hits: map[string][]types.SearchHit{
		"site":  {{ID: "s1"}, {ID: "s2"}, {ID: "s3"}, {ID: "s4"}},
		"media": {{ID: "m1"}, {ID: "m2"}, {ID: "m3"}, {ID: "m4"}},
	}}
	profile := &ranking.RankingProfile{Indexes: map[string]ranking.IndexProfile{"site": {Weight: 1}, "media": {Weight: 1}}}
	search := &Search{provider: provider}
	req := types.SearchRequest{Indexes: []string{"site", "media"}, Page: 1, PerPage: 2}
	_, state, err := search.searchCandidates(t.Context(), req, profile)
	if err != nil {
		t.Fatal(err)
	}
	page, err := search.refillCandidates(t.Context(), req, state, ranking.CandidateConfig{Multiplier: 1, MaxPerIndex: 3, MaxRefillRounds: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 4 || provider.requests[2].Indexes[0] != "site" || provider.requests[3].Indexes[0] != "media" || provider.requests[2].Page != 2 || provider.requests[3].Page != 2 {
		t.Fatalf("requests = %#v", provider.requests)
	}
	if got := page.Metadata["candidate_count"]; got != 6 {
		t.Fatalf("candidate_count = %#v", got)
	}
	if len(page.Hits) != 6 || page.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("page = %#v", page)
	}
}

func TestRefillCandidatesRecomputesEntityRRFOverAllRounds(t *testing.T) {
	provider := &pagedCandidateProvider{Provider: memory.New(memory.Config{}), hits: map[string][]types.SearchHit{
		"site":  {{ID: "site-shared", ResultID: "entity:shared"}, {ID: "site-only", ResultID: "entity:site"}},
		"media": {{ID: "media-only", ResultID: "entity:media"}, {ID: "media-shared", ResultID: "entity:shared"}},
	}}
	profile := &ranking.RankingProfile{Indexes: map[string]ranking.IndexProfile{"site": {Weight: 1}, "media": {Weight: 1}}}
	search := &Search{provider: provider}
	req := types.SearchRequest{Indexes: []string{"site", "media"}, Page: 1, PerPage: 1}
	_, state, err := search.searchCandidates(t.Context(), req, profile)
	if err != nil {
		t.Fatal(err)
	}
	page, err := search.refillCandidates(t.Context(), req, state, ranking.CandidateConfig{Multiplier: 1, MaxPerIndex: 2, MaxRefillRounds: 2})
	if err != nil {
		t.Fatal(err)
	}
	var shared *types.SearchHit
	for i := range page.Hits {
		if page.Hits[i].ResultID == "entity:shared" {
			shared = &page.Hits[i]
			break
		}
	}
	if shared == nil || shared.Retrieval == nil || len(shared.Retrieval.Contributions) != 2 || page.TotalAccuracy != types.TotalAccuracyExact {
		t.Fatalf("refilled entity fusion = %#v", page)
	}
}

func TestRefillCandidatesStopsExhaustedIndexesIndependently(t *testing.T) {
	provider := &pagedCandidateProvider{Provider: memory.New(memory.Config{}), hits: map[string][]types.SearchHit{
		"site":  {{ID: "s1"}},
		"media": {{ID: "m1"}, {ID: "m2"}, {ID: "m3"}},
	}}
	profile := &ranking.RankingProfile{Indexes: map[string]ranking.IndexProfile{"site": {Weight: 1}, "media": {Weight: 1}}}
	search := &Search{provider: provider}
	req := types.SearchRequest{Indexes: []string{"site", "media"}, Page: 1, PerPage: 1}
	_, state, err := search.searchCandidates(t.Context(), req, profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := search.refillCandidates(t.Context(), req, state, ranking.CandidateConfig{Multiplier: 1, MaxPerIndex: 3, MaxRefillRounds: 3}); err != nil {
		t.Fatal(err)
	}
	for _, request := range provider.requests[2:] {
		if request.Indexes[0] == "site" {
			t.Fatalf("exhausted site index was refilled: %#v", provider.requests)
		}
	}
}

func TestLegacyPostProcessingUsesBoundedPositiveCandidateWindow(t *testing.T) {
	got := candidateWindow(types.SearchRequest{Page: 1, PerPage: 20}, defaultCandidateConfig())
	if got <= 0 || got > 250 {
		t.Fatalf("legacy candidate window = %d", got)
	}
}

func TestRefillCandidatesNeverPromotesApproximateProviderTotal(t *testing.T) {
	provider := &pagedCandidateProvider{Provider: memory.New(memory.Config{}), hits: map[string][]types.SearchHit{
		"site": {{ID: "s1"}, {ID: "s2"}, {ID: "s3"}},
	}, accuracy: map[string]types.TotalAccuracy{"site": types.TotalAccuracyApproximate}}
	profile := &ranking.RankingProfile{Indexes: map[string]ranking.IndexProfile{"site": {Weight: 1}}}
	search := &Search{provider: provider}
	req := types.SearchRequest{Indexes: []string{"site"}, Page: 1, PerPage: 2}
	_, state, err := search.searchCandidates(t.Context(), req, profile)
	if err != nil {
		t.Fatal(err)
	}
	page, err := search.refillCandidates(t.Context(), req, state, ranking.CandidateConfig{Multiplier: 1, MaxPerIndex: 4, MaxRefillRounds: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("approximate provider total was promoted: %#v", page)
	}
}

func TestNormalizeLegacyCandidatePageReportsBoundedAccuracy(t *testing.T) {
	partial := normalizeLegacyCandidatePage(types.SearchResultPage{Hits: make([]types.SearchHit, 2), Total: 5, TotalAccuracy: types.TotalAccuracyExact}, 2)
	if partial.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("partial legacy page = %#v", partial)
	}
	exhausted := normalizeLegacyCandidatePage(types.SearchResultPage{Hits: make([]types.SearchHit, 2), Total: 2, TotalAccuracy: types.TotalAccuracyExact}, 2)
	if exhausted.TotalAccuracy != types.TotalAccuracyExact {
		t.Fatalf("exhausted legacy page = %#v", exhausted)
	}
	unknown := normalizeLegacyCandidatePage(types.SearchResultPage{Hits: make([]types.SearchHit, 2), Total: 2, TotalAccuracy: types.TotalAccuracy("future")}, 2)
	if unknown.TotalAccuracy != types.TotalAccuracyLowerBound {
		t.Fatalf("unknown provider accuracy was promoted: %#v", unknown)
	}
}

func (s staticEditorialStore) ListApplicable(context.Context, types.SearchRequest) ([]types.EditorialRankRule, error) {
	return append([]types.EditorialRankRule(nil), s.rules...), nil
}

func (s staticEditorialStore) Upsert(context.Context, types.EditorialRankRule) error {
	return nil
}

func (s staticEditorialStore) Delete(context.Context, string) error {
	return nil
}

func (p *countingBatchProvider) Search(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	p.searchCalls++
	return p.Provider.Search(ctx, req)
}

func (p *countingBatchProvider) SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
	p.batchCalls++
	p.batchSize = len(requests)
	out := make([]types.SearchResultPage, 0, len(requests))
	for _, req := range requests {
		page, err := p.Provider.Search(ctx, req)
		if err != nil {
			return nil, err
		}
		out = append(out, page)
	}
	return out, nil
}

func TestSearchGroupsAfterEditorialRanking(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:       "segment-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Ocean Wind",
			Body:     "prayer on the shore",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Ocean Wind",
				"parent_url":   "https://example.org/video-1",
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Mountain Prayer",
			Body:     "prayer in the mountains",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Mountain Prayer",
				"parent_url":   "https://example.org/video-2",
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
		Editorial: staticEditorialStore{rules: []types.EditorialRankRule{
			{
				ID:             "pin-video-2",
				ParentTargetID: "video-2",
				Action:         types.EditorialActionPin,
				Enabled:        true,
				Position:       new(0),
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Locale: "en"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 1,
		GroupBy: "parent_id",
	})
	if err != nil {
		t.Fatalf("query page 1: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected total group count 2, got %d", page.Total)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-2" {
		t.Fatalf("expected pinned group on page 1, got %+v", page.Groups)
	}
	page, err = search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    2,
		PerPage: 1,
		GroupBy: "parent_id",
	})
	if err != nil {
		t.Fatalf("query page 2: %v", err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-1" {
		t.Fatalf("expected second group on page 2, got %+v", page.Groups)
	}
}

func TestSearchHideRuleRemovesParentGroup(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{ID: "segment-1", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-1", Title: "Ocean Wind", Body: "prayer on the shore", Locale: "en"},
		{ID: "segment-2", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-2", Title: "Mountain Prayer", Body: "prayer in the mountains", Locale: "en"},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
		Editorial: staticEditorialStore{rules: []types.EditorialRankRule{
			{
				ID:             "hide-video-1",
				ParentTargetID: "video-1",
				Action:         types.EditorialActionHide,
				Enabled:        true,
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Locale: "en"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
		GroupBy: "parent_id",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-2" {
		t.Fatalf("groups = %#v", page.Groups)
	}
}

func TestSearchParentTargetDoesNotMatchUnrelatedHit(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{ID: "segment-1", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-1", Title: "Ocean Wind", Body: "prayer on the shore", Locale: "en"},
		{ID: "segment-2", Index: "media", Type: types.DocumentTypeTranscriptSegment, ParentID: "video-2", Title: "Mountain Prayer", Body: "prayer in the mountains", Locale: "en"},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
		Editorial: staticEditorialStore{rules: []types.EditorialRankRule{
			{
				ID:             "boost-video-2",
				ParentTargetID: "video-2",
				Action:         types.EditorialActionBoost,
				Weight:         100,
				Enabled:        true,
				Scope:          types.EditorialScope{Indexes: []string{"media"}, Locale: "en"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Hits) < 2 {
		t.Fatalf("hits = %#v", page.Hits)
	}
	if page.Hits[0].Parent != nil && page.Hits[0].Parent.ID != "video-2" {
		t.Fatalf("unexpected top hit after parent-target boost: %#v", page.Hits[0])
	}
	for _, hit := range page.Hits {
		if hit.Parent != nil && hit.Parent.ID == "video-1" && hit.FinalScore >= 100 {
			t.Fatalf("unexpected unrelated boost on hit %#v", hit)
		}
	}
}

func TestSearchGroupedDisjunctiveFacetsCountUniqueGroups(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:       "segment-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Architecture One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture One",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Teaching"},
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Architecture One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture One",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Teaching"},
			},
		},
		{
			ID:       "segment-3",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Architecture Two",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture Two",
				"parent_url":   "https://example.org/video-2",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Workshop"},
			},
		},
		{
			ID:       "segment-4",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-3",
			Title:    "UI One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "UI One",
				"parent_url":   "https://example.org/video-3",
			},
			Facets: map[string][]string{
				"topic":  {"ui"},
				"format": {"Teaching"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
		GroupBy: "parent_id",
		Filters: types.AndExpr{Terms: []types.FilterExpr{
			types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: "architecture"},
			types.TermExpr{Field: "format", Op: types.FilterOpEQ, Value: "Teaching"},
		}},
		Facets: []types.FacetRequest{
			{Field: "format", Disjunctive: true},
			{Field: "topic", Disjunctive: true},
		},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page.Groups) != 1 || page.Groups[0].Key != "video-1" {
		t.Fatalf("expected only teaching architecture group in results, got %+v", page.Groups)
	}
	formatFacet := facetByField(page.Facets, "format")
	if formatFacet == nil {
		t.Fatalf("missing format facet: %+v", page.Facets)
	}
	formatCounts := facetCounts(*formatFacet)
	if formatCounts["Teaching"] != 1 || formatCounts["Workshop"] != 1 {
		t.Fatalf("unexpected format counts: %+v", formatCounts)
	}
	if !facetSelected(*formatFacet, "Teaching") {
		t.Fatalf("expected selected teaching value: %+v", formatFacet.Values)
	}
	topicFacet := facetByField(page.Facets, "topic")
	if topicFacet == nil {
		t.Fatalf("missing topic facet: %+v", page.Facets)
	}
	topicCounts := facetCounts(*topicFacet)
	if topicCounts["architecture"] != 1 || topicCounts["ui"] != 1 {
		t.Fatalf("unexpected topic counts: %+v", topicCounts)
	}
	if !facetSelected(*topicFacet, "architecture") {
		t.Fatalf("expected selected architecture value: %+v", topicFacet.Values)
	}
}

func TestSearchGroupedDisjunctiveFacetsUseBatchProviderWhenAvailable(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media", GroupByDefault: "parent_id"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	baseProvider := memory.New(memory.Config{})
	provider := &countingBatchProvider{Provider: baseProvider}
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:       "segment-1",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-1",
			Title:    "Architecture One",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture One",
				"parent_url":   "https://example.org/video-1",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Teaching"},
			},
		},
		{
			ID:       "segment-2",
			Index:    "media",
			Type:     types.DocumentTypeTranscriptSegment,
			ParentID: "video-2",
			Title:    "Architecture Two",
			Body:     "prayer architecture",
			Locale:   "en",
			Fields: map[string]any{
				"parent_title": "Architecture Two",
				"parent_url":   "https://example.org/video-2",
			},
			Facets: map[string][]string{
				"topic":  {"architecture"},
				"format": {"Workshop"},
			},
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{
		Planner:  p,
		Provider: provider,
	})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	_, err = search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "en",
		Page:    1,
		PerPage: 10,
		GroupBy: "parent_id",
		Filters: types.TermExpr{Field: "topic", Op: types.FilterOpEQ, Value: "architecture"},
		Facets: []types.FacetRequest{
			{Field: "format", Disjunctive: true},
			{Field: "topic", Disjunctive: true},
		},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if provider.searchCalls != 1 {
		t.Fatalf("expected one primary search call, got %d", provider.searchCalls)
	}
	if provider.batchCalls != 1 || provider.batchSize != 2 {
		t.Fatalf("expected one batch call for two facets, got batchCalls=%d batchSize=%d", provider.batchCalls, provider.batchSize)
	}
}

func facetByField(facets []types.SearchFacet, field string) *types.SearchFacet {
	for i := range facets {
		if facets[i].Field == field {
			return &facets[i]
		}
	}
	return nil
}

func facetCounts(facet types.SearchFacet) map[string]int {
	out := make(map[string]int, len(facet.Values))
	for _, value := range facet.Values {
		out[value.Value] = value.Count
	}
	return out
}

func facetSelected(facet types.SearchFacet, needle string) bool {
	for _, value := range facet.Values {
		if value.Value == needle {
			return value.Selected
		}
	}
	return false
}

func TestSearchRejectsUnsupportedSemanticMode(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	p, err := planner.New(planner.Config{Registry: registry})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}
	_, err = search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Mode:    types.SearchModeSemantic,
		Semantic: &types.SemanticRequest{
			Field: "body",
		},
	})
	if err == nil {
		t.Fatalf("expected unsupported semantic mode error")
	}
}

func TestSearchPrefersExactLocaleAndAnnotatesFallbackOrigins(t *testing.T) {
	registry := indexing.NewRegistry()
	def := types.IndexDefinition{Name: "media"}
	if err := registry.Register(def, nil); err != nil {
		t.Fatalf("register index: %v", err)
	}
	provider := memory.New(memory.Config{})
	if err := provider.EnsureIndex(context.Background(), def); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := provider.UpsertDocuments(context.Background(), "media", []types.Document{
		{
			ID:     "doc-exact",
			Index:  "media",
			Title:  "Oracion exacta",
			Body:   "prayer in spanish",
			Locale: "es",
		},
		{
			ID:     "doc-fallback",
			Index:  "media",
			Title:  "Fallback prayer",
			Body:   "prayer in english",
			Locale: "en",
		},
	}); err != nil {
		t.Fatalf("upsert docs: %v", err)
	}

	runtime, err := locale.NewI18nRuntimeFromCultureData("../testdata/locale_search_culture.json", "en")
	if err != nil {
		t.Fatalf("new locale runtime: %v", err)
	}
	p, err := planner.New(planner.Config{
		Registry:      registry,
		LocaleRuntime: runtime,
		LocalePolicy: planner.LocalePolicy{
			MatchStrategy:   locale.MatchExactOrParent,
			Scope:           locale.ScopeActiveOnly,
			ExpandFallbacks: true,
			IncludeDefault:  true,
		},
	})
	if err != nil {
		t.Fatalf("new planner: %v", err)
	}
	search, err := NewSearch(SearchConfig{Planner: p, Provider: provider})
	if err != nil {
		t.Fatalf("new search query: %v", err)
	}

	page, err := search.Query(context.Background(), types.SearchRequest{
		Indexes: []string{"media"},
		Query:   "prayer",
		Locale:  "es",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("search query: %v", err)
	}

	if len(page.Hits) != 2 {
		t.Fatalf("expected two hits, got %+v", page.Hits)
	}
	if page.Hits[0].ID != "doc-exact" {
		t.Fatalf("expected exact hit first, got %+v", page.Hits)
	}
	if page.Hits[0].Retrieval == nil || page.Hits[0].Retrieval.Metadata["locale_match"] != "exact" {
		t.Fatalf("exact hit metadata = %+v", page.Hits[0].Retrieval)
	}
	if origin := page.Hits[0].Retrieval.Metadata["locale_origin"]; origin != "matched" {
		t.Fatalf("exact hit locale origin = %#v", origin)
	}
	if page.Hits[1].Retrieval == nil || page.Hits[1].Retrieval.Metadata["locale_match"] != "fallback" {
		t.Fatalf("fallback hit metadata = %+v", page.Hits[1].Retrieval)
	}
	if origin := page.Hits[1].Retrieval.Metadata["locale_origin"]; origin != "default" {
		t.Fatalf("fallback hit locale origin = %#v", origin)
	}
}
