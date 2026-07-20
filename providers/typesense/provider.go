package typesense

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-search/internal/errs"
	"github.com/goliatone/go-search/pkg/types"
	"github.com/goliatone/go-search/ranking"
	tstypesense "github.com/typesense/typesense-go/v3/typesense"
	tsapi "github.com/typesense/typesense-go/v3/typesense/api"
)

type Config struct {
	ServerURL                  string
	APIKey                     string
	NearestNode                string
	Nodes                      []string
	CollectionPrefix           string
	CollectionNamer            func(index string) string
	NumRetries                 int
	RetryInterval              time.Duration
	HealthcheckInterval        time.Duration
	ConnectionTimeout          time.Duration
	GroupedEvidenceLimit       int
	SuggestFetchMultiplier     int
	SuggestMinimumFetchLimit   int
	MultiSearchMinimumPerPage  int
	ExactGroupCountPageSize    int
	SuggestPreferParentFields  []string
	SuggestPreferParentWeights []int
	SuggestDocumentFields      []string
	SuggestDocumentWeights     []int
	Clock                      types.Clock
}

type Provider struct {
	client *tstypesense.Client
	cfg    Config

	mu      sync.RWMutex
	indexes map[string]managedIndex
}

type managedIndex struct {
	def            types.IndexDefinition
	collectionName string
	schemaHash     string
}

func New(cfg Config) (*Provider, error) {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.ServerURL) == "" {
		return nil, errs.ConfigurationError("typesense server url is required", nil)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errs.ConfigurationError("typesense api key is required", nil)
	}

	opts := []tstypesense.ClientOption{
		tstypesense.WithServer(strings.TrimSpace(cfg.ServerURL)),
		tstypesense.WithAPIKey(strings.TrimSpace(cfg.APIKey)),
	}
	if strings.TrimSpace(cfg.NearestNode) != "" {
		opts = append(opts, tstypesense.WithNearestNode(strings.TrimSpace(cfg.NearestNode)))
	}
	if len(cfg.Nodes) > 0 {
		opts = append(opts, tstypesense.WithNodes(cfg.Nodes))
	}
	if cfg.NumRetries > 0 {
		opts = append(opts, tstypesense.WithNumRetries(cfg.NumRetries))
	}
	if cfg.RetryInterval > 0 {
		opts = append(opts, tstypesense.WithRetryInterval(cfg.RetryInterval))
	}
	if cfg.HealthcheckInterval > 0 {
		opts = append(opts, tstypesense.WithHealthcheckInterval(cfg.HealthcheckInterval))
	}
	if cfg.ConnectionTimeout > 0 {
		opts = append(opts, tstypesense.WithConnectionTimeout(cfg.ConnectionTimeout))
	}

	return &Provider{
		client:  tstypesense.NewClient(opts...),
		cfg:     cfg,
		indexes: map[string]managedIndex{},
	}, nil
}

func (p *Provider) Name() string { return "typesense" }

func (p *Provider) Capabilities(context.Context) (types.CapabilitySet, error) {
	return types.CapabilitySet{
		Facets:               true,
		HierarchicalFacets:   true,
		DisjunctiveFacets:    true,
		EntityFacetCounts:    true,
		CrossIndexFacetUnion: false,
		Grouping:             true,
		Highlighting:         true,
		Snippets:             true,
		PrefixSearch:         true,
		WeightedQueryFields:  true,
		TextMatchControls:    true,
		SupportedSearchModes: []types.SearchMode{types.SearchModeLexical},
		Limitations: []types.CapabilityLimitation{
			{Capability: "cross_index_facet_union", Message: "callers must union the bounded per-index entity identity sets"},
			{
				Capability: "range_facets",
				Message:    "typesense provider supports range filtering but does not compute dedicated numeric/date range facet buckets in the canonical response yet",
			},
		},
		Metadata: map[string]any{
			"grouped_evidence_limit": p.cfg.GroupedEvidenceLimit,
		},
		EntityGrouping: true, ExactEntityCounts: true, BatchedEvidence: true,
	}, nil
}

func (p *Provider) AggregateEvidence(ctx context.Context, in types.EvidenceRequest) (map[string]*types.MatchEvidenceSummary, error) {
	requests := make([]types.SearchRequest, 0, len(in.ResultIDs))
	for _, id := range in.ResultIDs {
		req := in.Search
		req.Page = 1
		req.PerPage = in.MaxSamplesPerLocation
		req.GroupBy = ""
		req.Facets = []types.FacetRequest{{Field: "match_location"}}
		term := types.TermExpr{Field: "result_id", Op: types.FilterOpEQ, Value: id}
		if req.Filters != nil {
			req.Filters = types.AndExpr{Terms: []types.FilterExpr{req.Filters, term}}
		} else {
			req.Filters = term
		}
		requests = append(requests, req)
	}
	pages, err := p.SearchBatch(ctx, requests)
	if err != nil {
		return nil, err
	}
	out := map[string]*types.MatchEvidenceSummary{}
	for i, page := range pages {
		if in.Guard != nil {
			visible := make([]types.SearchHit, 0, len(page.Hits))
			for _, hit := range page.Hits {
				if hit.Document != nil && in.Guard.AllowDocument(ctx, in.Search.Actor, hit.Document.Clone()) {
					visible = append(visible, hit)
				}
			}
			summary := ranking.AggregateEvidence(visible, in.MaxSamplesPerLocation)[in.ResultIDs[i]]
			if summary == nil {
				summary = &types.MatchEvidenceSummary{}
			}
			summary.Exact = false
			summary.Status = types.EvidenceStatusPartial
			summary.Diagnostic = "visibility guard prevents exact provider facet counts"
			out[in.ResultIDs[i]] = summary
			continue
		}
		summary := &types.MatchEvidenceSummary{Exact: true, Status: types.EvidenceStatusComplete}
		for _, facet := range page.Facets {
			if facet.Field != "match_location" {
				continue
			}
			for _, value := range facet.Values {
				location := types.MatchEvidenceLocation{Location: value.Value, Count: value.Count}
				for _, hit := range page.Hits {
					if hit.Document != nil && hit.Document.MatchLocation == value.Value && len(location.Samples) < in.MaxSamplesPerLocation {
						location.Samples = append(location.Samples, types.MatchEvidenceSample{DocumentID: hit.ID, Field: hit.Document.MatchField, Locale: hit.Locale, Snippet: types.BoundedSearchSnippet(hit.Snippet), ChunkOrdinal: hit.Document.ChunkOrdinal, Anchor: hit.Anchor})
					}
				}
				summary.Locations = append(summary.Locations, location)
			}
		}
		out[in.ResultIDs[i]] = summary
	}
	return out, nil
}

func (p *Provider) EnsureIndex(ctx context.Context, def types.IndexDefinition) error {
	if strings.TrimSpace(def.Name) == "" {
		return errs.ConfigurationError("index name is required", nil)
	}

	schema, schemaHash, err := buildCollectionSchema(p.cfg, def)
	if err != nil {
		return err
	}

	existing, err := p.client.Collection(schema.Name).Retrieve(ctx)
	if err != nil {
		if isTypesenseStatus(err, http.StatusNotFound) {
			if _, createErr := p.client.Collections().Create(ctx, schema); createErr != nil {
				return errs.Wrap(createErr, map[string]any{"index": def.Name, "collection": schema.Name})
			}
		} else {
			return errs.Wrap(err, map[string]any{"index": def.Name, "collection": schema.Name})
		}
	} else {
		existingHash := collectionResponseHash(existing)
		if existingHash != schemaHash {
			return errs.SchemaMismatch("typesense collection schema does not match the index definition", map[string]any{
				"index":           def.Name,
				"collection":      schema.Name,
				"expected_schema": schemaHash,
				"actual_schema":   existingHash,
			})
		}
	}

	p.mu.Lock()
	p.indexes[def.Name] = managedIndex{
		def:            def,
		collectionName: schema.Name,
		schemaHash:     schemaHash,
	}
	p.mu.Unlock()

	return nil
}

func (p *Provider) Search(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	if len(req.Indexes) == 0 {
		return types.SearchResultPage{}, errs.UnknownIndex("", map[string]any{"reason": "no indexes requested"})
	}
	if len(req.Indexes) == 1 {
		return p.searchSingleIndex(ctx, req.Indexes[0], req)
	}
	return p.searchMultiIndex(ctx, req)
}

func (p *Provider) SearchBatch(ctx context.Context, requests []types.SearchRequest) ([]types.SearchResultPage, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	pages := make([]types.SearchResultPage, len(requests))
	type batchItem struct {
		position int
		runtime  managedIndex
		req      types.SearchRequest
		params   tsapi.MultiSearchCollectionParameters
	}
	batched := make([]batchItem, 0, len(requests))
	for i, req := range requests {
		if len(req.Indexes) != 1 {
			page, err := p.Search(ctx, req)
			if err != nil {
				return nil, err
			}
			pages[i] = page
			continue
		}
		runtime, err := p.runtimeFor(req.Indexes[0])
		if err != nil {
			return nil, err
		}
		params, err := compileSearchParams(p.cfg, runtime.def, req)
		if err != nil {
			return nil, err
		}
		multiParams, err := searchParamsToMulti(runtime.collectionName, params)
		if err != nil {
			return nil, err
		}
		batched = append(batched, batchItem{
			position: i,
			runtime:  runtime,
			req:      req,
			params:   multiParams,
		})
	}
	if len(batched) == 0 {
		return pages, nil
	}
	body := tsapi.MultiSearchSearchesParameter{Searches: make([]tsapi.MultiSearchCollectionParameters, 0, len(batched))}
	for _, item := range batched {
		body.Searches = append(body.Searches, item.params)
	}
	result, err := p.client.MultiSearch.Perform(ctx, nil, body)
	if err != nil {
		return nil, errs.Wrap(err, map[string]any{"provider": p.Name(), "mode": "batch"})
	}
	if len(result.Results) != len(batched) {
		return nil, errs.Wrap(errors.New("typesense multi search result count mismatch"), map[string]any{
			"expected": len(batched),
			"actual":   len(result.Results),
		})
	}
	for i, item := range batched {
		page, err := p.mapMultiSearchResult(item.runtime, item.req, result.Results[i])
		if err != nil {
			return nil, err
		}
		if page.Facets, err = p.disjunctiveFacets(ctx, item.runtime, item.req, page.Facets); err != nil {
			return nil, err
		}
		if item.req.GroupBy != "" {
			total, err := p.exactGroupCount(ctx, item.runtime, item.req)
			if err != nil {
				return nil, err
			}
			page.Total = total
		}
		pages[item.position] = page
	}
	return pages, nil
}

func (p *Provider) Suggest(ctx context.Context, req types.SuggestRequest) (types.SuggestResult, error) {
	if len(req.Indexes) == 0 {
		return types.SuggestResult{}, errs.UnknownIndex("", map[string]any{"reason": "no indexes requested"})
	}

	items := make([]types.SuggestHit, 0, max(req.Limit, 5))
	seen := map[string]struct{}{}
	for _, index := range req.Indexes {
		runtime, err := p.runtimeFor(index)
		if err != nil {
			return types.SuggestResult{}, err
		}
		params, err := compileSuggestParams(p.cfg, runtime.def, req)
		if err != nil {
			return types.SuggestResult{}, err
		}
		result, err := p.client.Collection(runtime.collectionName).Documents().Search(ctx, params)
		if err != nil {
			return types.SuggestResult{}, errs.Wrap(err, map[string]any{"index": index, "collection": runtime.collectionName})
		}
		hits := mapSuggestHits(result, req)
		for _, hit := range hits {
			if _, ok := seen[hit.ID]; ok {
				continue
			}
			seen[hit.ID] = struct{}{}
			items = append(items, hit)
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			if items[i].Title == items[j].Title {
				return items[i].ID < items[j].ID
			}
			return items[i].Title < items[j].Title
		}
		return items[i].Score > items[j].Score
	})
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}

	return types.SuggestResult{
		Items: items,
		Metadata: map[string]any{
			"provider": "typesense",
		},
	}, nil
}

func (p *Provider) UpsertDocuments(ctx context.Context, index string, docs []types.Document) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	return upsertDocuments(ctx, p.client, runtime, docs)
}

func (p *Provider) DeleteDocuments(ctx context.Context, index string, ids []string) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	return deleteDocuments(ctx, p.client, runtime, ids)
}

func (p *Provider) DeleteBySource(ctx context.Context, index, registrationKey string, sourceIDs []string) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	return deleteBySource(ctx, p.client, runtime, registrationKey, sourceIDs)
}

func (p *Provider) ReplaceDocuments(ctx context.Context, index, registrationKey string, sourceIDs []string, docs []types.Document) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	return replaceDocuments(ctx, p.client, runtime, registrationKey, sourceIDs, docs)
}

func (p *Provider) ResetRegistration(ctx context.Context, index, registrationKey string) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	return deleteByRegistration(ctx, p.client, runtime, registrationKey)
}

func (p *Provider) Health(ctx context.Context, req types.HealthRequest) (types.HealthStatus, error) {
	collections, err := p.client.Collections().Retrieve(ctx)
	if err != nil {
		return types.HealthStatus{}, errs.Wrap(err, map[string]any{"provider": p.Name()})
	}

	byName := map[string]*tsapi.CollectionResponse{}
	for _, collection := range collections {
		if collection != nil {
			byName[collection.Name] = collection
		}
	}

	p.mu.RLock()
	runtimes := make([]managedIndex, 0, len(p.indexes))
	for _, runtime := range p.indexes {
		if len(req.Indexes) > 0 && !contains(req.Indexes, runtime.def.Name) {
			continue
		}
		runtimes = append(runtimes, runtime)
	}
	p.mu.RUnlock()

	indexes := make([]types.IndexHealth, 0, len(runtimes))
	healthy := true
	for _, runtime := range runtimes {
		collection, ok := byName[runtime.collectionName]
		indexHealth := types.IndexHealth{
			Name:  runtime.def.Name,
			Ready: ok,
			Metadata: map[string]any{
				"collection_name": runtime.collectionName,
				"schema_hash":     runtime.schemaHash,
			},
		}
		if ok {
			if collection.NumDocuments != nil {
				indexHealth.Documents = int(*collection.NumDocuments)
			}
		} else {
			healthy = false
			indexHealth.Message = "collection not found"
		}
		indexes = append(indexes, indexHealth)
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		return indexes[i].Name < indexes[j].Name
	})

	return types.HealthStatus{
		Provider:  p.Name(),
		Healthy:   healthy,
		CheckedAt: p.cfg.Clock.Now(),
		Indexes:   indexes,
		Metadata: map[string]any{
			"server_url": strings.TrimSpace(p.cfg.ServerURL),
		},
	}, nil
}

// HealthDefinitions inspects the durable Typesense collections represented by
// definitions without registering them in this provider instance. Collection
// retrieval resolves aliases, while schema hashing deliberately ignores the
// physical collection name, so the result is valid for generation-backed
// deployments as well as direct collections.
func (p *Provider) HealthDefinitions(ctx context.Context, definitions []types.IndexDefinition) (types.HealthStatus, error) {
	indexes := make([]types.IndexHealth, 0, len(definitions))
	healthy := true
	for _, def := range definitions {
		schema, expectedHash, err := buildCollectionSchema(p.cfg, def)
		if err != nil {
			return types.HealthStatus{}, err
		}

		indexHealth := types.IndexHealth{
			Name: def.Name,
			Metadata: map[string]any{
				"collection_name":      schema.Name,
				"schema_hash":          expectedHash,
				"expected_schema_hash": expectedHash,
			},
		}
		collection, err := p.client.Collection(schema.Name).Retrieve(ctx)
		if err != nil {
			if !isTypesenseStatus(err, http.StatusNotFound) {
				return types.HealthStatus{}, errs.Wrap(err, map[string]any{
					"provider":   p.Name(),
					"index":      def.Name,
					"collection": schema.Name,
				})
			}
			healthy = false
			indexHealth.Message = "collection not found"
			indexes = append(indexes, indexHealth)
			continue
		}

		actualHash := collectionResponseHash(collection)
		indexHealth.Metadata["actual_schema_hash"] = actualHash
		indexHealth.Metadata["active_collection_name"] = collection.Name
		indexHealth.Metadata["schema_match"] = actualHash == expectedHash
		if collection.NumDocuments != nil {
			indexHealth.Documents = int(*collection.NumDocuments)
		}
		if actualHash != expectedHash {
			healthy = false
			indexHealth.Message = "collection schema does not match index definition"
		} else {
			indexHealth.Ready = true
		}
		indexes = append(indexes, indexHealth)
	}

	sort.SliceStable(indexes, func(i, j int) bool {
		return indexes[i].Name < indexes[j].Name
	})
	return types.HealthStatus{
		Provider:  p.Name(),
		Healthy:   healthy,
		CheckedAt: p.cfg.Clock.Now(),
		Indexes:   indexes,
		Metadata: map[string]any{
			"server_url": strings.TrimSpace(p.cfg.ServerURL),
			"mode":       "definition_inspection",
		},
	}, nil
}

func (p *Provider) searchSingleIndex(ctx context.Context, index string, req types.SearchRequest) (types.SearchResultPage, error) {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return types.SearchResultPage{}, err
	}

	params, err := compileSearchParams(p.cfg, runtime.def, req)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	result, err := p.client.Collection(runtime.collectionName).Documents().Search(ctx, params)
	if err != nil {
		return types.SearchResultPage{}, errs.Wrap(err, map[string]any{"index": index, "collection": runtime.collectionName})
	}
	page := mapSearchResult(result, runtime, req, p.cfg)
	if hasEntityFacetRequests(req.Facets) {
		page.Facets, err = p.entityFacets(ctx, runtime, req)
		if err != nil {
			return types.SearchResultPage{}, err
		}
	} else if page.Facets, err = p.disjunctiveFacets(ctx, runtime, req, page.Facets); err != nil {
		return types.SearchResultPage{}, err
	}
	if req.GroupBy != "" {
		total, err := p.exactGroupCount(ctx, runtime, req)
		if err != nil {
			return types.SearchResultPage{}, err
		}
		page.Total = total
	}
	return page, nil
}

func hasEntityFacetRequests(requests []types.FacetRequest) bool {
	for _, request := range requests {
		if request.CountBy == types.FacetCountByResultID {
			return true
		}
	}
	return false
}

func (p *Provider) entityFacets(ctx context.Context, runtime managedIndex, req types.SearchRequest) ([]types.SearchFacet, error) {
	out := make([]types.SearchFacet, 0, len(req.Facets))
	for _, facetReq := range req.Facets {
		if facetReq.CountBy != types.FacetCountByResultID {
			continue
		}
		filter := req.Filters
		if facetReq.Disjunctive {
			filter = types.RemoveFacetFilter(filter, facetReq.Field)
		}
		identities := map[string]map[string]struct{}{}
		fetched, total := 0, 0
		for pageNumber := 1; fetched < facetReq.IdentityLimit; pageNumber++ {
			perPage := min(250, facetReq.IdentityLimit-fetched)
			probe := req
			probe.Page = pageNumber
			probe.PerPage = perPage
			probe.GroupBy = ""
			probe.Facets = nil
			probe.Highlight = nil
			probe.Filters = filter
			params, err := compileSearchParams(p.cfg, runtime.def, probe)
			if err != nil {
				return nil, err
			}
			result, err := p.client.Collection(runtime.collectionName).Documents().Search(ctx, params)
			if err != nil {
				return nil, errs.Wrap(err, map[string]any{"index": runtime.def.Name, "facet": facetReq.Field})
			}
			mapped := mapSearchResult(result, runtime, probe, p.cfg)
			total = mapped.Total
			for _, hit := range mapped.Hits {
				if hit.Document == nil {
					continue
				}
				id := ranking.ResultID(hit)
				for _, value := range hit.Document.Facets[facetReq.Field] {
					set := identities[value]
					if set == nil {
						set = map[string]struct{}{}
						identities[value] = set
					}
					set[id] = struct{}{}
				}
			}
			fetched += len(mapped.Hits)
			if len(mapped.Hits) == 0 || fetched >= total {
				break
			}
		}
		facet := types.BuildEntityFacet(facetReq, identities, types.SelectedFacetValues(req.Filters, facetReq.Field))
		if fetched < total {
			facet.Accuracy = types.FacetCountAccuracyLowerBound
			for i := range facet.Values {
				facet.Values[i].EntityIDsComplete = false
			}
		}
		out = append(out, facet)
	}
	return out, nil
}

func (p *Provider) searchMultiIndex(ctx context.Context, req types.SearchRequest) (types.SearchResultPage, error) {
	aggregate := types.SearchResultPage{
		Page:    req.Page,
		PerPage: req.PerPage,
		Metadata: map[string]any{
			"provider": "typesense",
		},
	}
	allHits := []types.SearchHit{}
	facets := map[string]map[string]int{}
	total := 0
	duration := int64(0)

	fetchReq := req
	if fetchReq.Page < 1 {
		fetchReq.Page = 1
	}
	if fetchReq.PerPage <= 0 {
		fetchReq.PerPage = p.cfg.MultiSearchMinimumPerPage
	}
	fetchReq.Page = 1
	fetchReq.PerPage = max(req.Page*req.PerPage, fetchReq.PerPage)

	for _, index := range req.Indexes {
		page, err := p.searchSingleIndex(ctx, index, fetchReq)
		if err != nil {
			return types.SearchResultPage{}, err
		}
		total += page.Total
		duration += page.DurationMS
		allHits = append(allHits, page.Hits...)
		mergeFacets(facets, page.Facets)
	}

	if req.GroupBy != "" {
		allGroups := ranking.GroupHits(allHits)
		sort.SliceStable(allGroups, func(i, j int) bool {
			return compareSearchHits(req, *allGroups[i].TopHit, *allGroups[j].TopHit)
		})
		aggregate.Total = len(allGroups)
		aggregate.Groups = ranking.PaginateGroups(allGroups, req.Page, req.PerPage)
		aggregate.Hits = ranking.FlattenGroupHits(aggregate.Groups)
	} else {
		sort.SliceStable(allHits, func(i, j int) bool {
			return compareSearchHits(req, allHits[i], allHits[j])
		})
		aggregate.Total = total
		aggregate.Hits = ranking.PaginateHits(allHits, req.Page, req.PerPage)
	}
	aggregate.DurationMS = duration
	aggregate.Facets = flattenFacetMap(req, facets)
	return aggregate, nil
}

func (p *Provider) disjunctiveFacets(ctx context.Context, runtime managedIndex, req types.SearchRequest, current []types.SearchFacet) ([]types.SearchFacet, error) {
	needsRefresh := false
	byField := map[string]types.SearchFacet{}
	for _, facet := range current {
		byField[facet.Field] = facet
	}
	for _, facetReq := range req.Facets {
		if !facetReq.Disjunctive {
			continue
		}
		needsRefresh = true
		countReq := req
		countReq.Page = 1
		countReq.PerPage = 1
		countReq.GroupBy = ""
		countReq.Highlight = nil
		countReq.Filters = types.RemoveFacetFilter(req.Filters, facetReq.Field)
		countReq.Facets = []types.FacetRequest{facetReq}
		params, err := compileSearchParams(p.cfg, runtime.def, countReq)
		if err != nil {
			return nil, err
		}
		result, err := p.client.Collection(runtime.collectionName).Documents().Search(ctx, params)
		if err != nil {
			return nil, errs.Wrap(err, map[string]any{
				"index":      runtime.def.Name,
				"collection": runtime.collectionName,
				"facet":      facetReq.Field,
			})
		}
		refreshed := mapFacets(result, countReq)
		if len(refreshed) > 0 {
			byField[facetReq.Field] = refreshed[0]
		}
	}
	if !needsRefresh {
		return current, nil
	}
	out := make([]types.SearchFacet, 0, len(req.Facets))
	for _, facetReq := range req.Facets {
		if facet, ok := byField[facetReq.Field]; ok {
			out = append(out, facet)
		}
	}
	return out, nil
}

func (p *Provider) mapMultiSearchResult(runtime managedIndex, req types.SearchRequest, item tsapi.MultiSearchResultItem) (types.SearchResultPage, error) {
	if item.Error != nil {
		code := int64(0)
		if item.Code != nil {
			code = *item.Code
		}
		return types.SearchResultPage{}, errs.Wrap(errors.New(*item.Error), map[string]any{
			"index":      runtime.def.Name,
			"collection": runtime.collectionName,
			"code":       code,
		})
	}
	body, err := json.Marshal(item)
	if err != nil {
		return types.SearchResultPage{}, err
	}
	var result tsapi.SearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		return types.SearchResultPage{}, err
	}
	return mapSearchResult(&result, runtime, req, p.cfg), nil
}

func searchParamsToMulti(collectionName string, params *tsapi.SearchCollectionParams) (tsapi.MultiSearchCollectionParameters, error) {
	if params == nil {
		return tsapi.MultiSearchCollectionParameters{}, nil
	}
	body, err := json.Marshal(params)
	if err != nil {
		return tsapi.MultiSearchCollectionParameters{}, err
	}
	var out tsapi.MultiSearchCollectionParameters
	if err := json.Unmarshal(body, &out); err != nil {
		return tsapi.MultiSearchCollectionParameters{}, err
	}
	out.Collection = &collectionName
	return out, nil
}

func (p *Provider) exactGroupCount(ctx context.Context, runtime managedIndex, req types.SearchRequest) (int, error) {
	countReq := req
	countReq.Page = 1
	countReq.PerPage = p.cfg.ExactGroupCountPageSize
	params, err := compileSearchParams(p.cfg, runtime.def, countReq)
	if err != nil {
		return 0, err
	}
	includeFields := "id,parent_id"
	highlightFields := "none"
	params.IncludeFields = &includeFields
	params.HighlightFields = &highlightFields
	params.FacetBy = nil
	params.MaxFacetValues = nil

	total := 0
	for pageNumber := 1; ; pageNumber++ {
		params.Page = new(pageNumber)
		result, err := p.client.Collection(runtime.collectionName).Documents().Search(ctx, params)
		if err != nil {
			return 0, errs.Wrap(err, map[string]any{"index": runtime.def.Name, "collection": runtime.collectionName, "page": pageNumber})
		}
		if result == nil || result.GroupedHits == nil || len(*result.GroupedHits) == 0 {
			return total, nil
		}
		total += len(*result.GroupedHits)
		if len(*result.GroupedHits) < countReq.PerPage {
			return total, nil
		}
	}
}

func (p *Provider) runtimeFor(index string) (managedIndex, error) {
	p.mu.RLock()
	runtime, ok := p.indexes[index]
	p.mu.RUnlock()
	if !ok {
		return managedIndex{}, errs.UnknownIndex(index, nil)
	}
	return runtime, nil
}

func (p *Provider) dropIndex(ctx context.Context, index string) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	if _, err := p.client.Collection(runtime.collectionName).Delete(ctx); err != nil && !isTypesenseStatus(err, http.StatusNotFound) {
		return err
	}
	p.mu.Lock()
	delete(p.indexes, index)
	p.mu.Unlock()
	return nil
}

func (p *Provider) cleanup(ctx context.Context) error {
	p.mu.RLock()
	indexes := make([]string, 0, len(p.indexes))
	for index := range p.indexes {
		indexes = append(indexes, index)
	}
	p.mu.RUnlock()
	var joined error
	for _, index := range indexes {
		if err := p.dropIndex(ctx, index); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func isTypesenseStatus(err error, status int) bool {
	var httpErr *tstypesense.HTTPError
	return errors.As(err, &httpErr) && httpErr.Status == status
}

func compareSearchHits(req types.SearchRequest, left, right types.SearchHit) bool {
	leftExact := isExactLocaleMatch(req.Locale, left.Locale)
	rightExact := isExactLocaleMatch(req.Locale, right.Locale)
	if leftExact != rightExact {
		return leftExact
	}
	if ordered, ok := compareRequestedSorts(req.Sort, left, right); ok {
		return ordered
	}
	if left.Score == right.Score {
		return left.ID < right.ID
	}
	return left.Score > right.Score
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
		leftText := sortableText(left, sortField.Field)
		rightText := sortableText(right, sortField.Field)
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
	if hit.Document != nil && hit.Document.Numeric != nil {
		if value, ok := hit.Document.Numeric[field]; ok {
			return value, true
		}
	}
	if hit.Document != nil && hit.Document.Fields != nil {
		if value, ok := hit.Document.Fields[field]; ok {
			return asFloat64(value), true
		}
	}
	return 0, false
}

func sortableText(hit types.SearchHit, field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "title":
		return strings.ToLower(strings.TrimSpace(hit.Title))
	case "locale":
		return strings.ToLower(strings.TrimSpace(hit.Locale))
	default:
		if hit.Document != nil && hit.Document.Fields != nil {
			if raw, ok := hit.Document.Fields[field]; ok {
				return strings.ToLower(strings.TrimSpace(stringify(raw)))
			}
		}
		return ""
	}
}

func mergeFacets(dst map[string]map[string]int, input []types.SearchFacet) {
	for _, facet := range input {
		if _, ok := dst[facet.Field]; !ok {
			dst[facet.Field] = map[string]int{}
		}
		for _, value := range facet.Values {
			dst[facet.Field][value.Value] += value.Count
		}
	}
}

func flattenFacetMap(req types.SearchRequest, in map[string]map[string]int) []types.SearchFacet {
	out := make([]types.SearchFacet, 0, len(in))
	requests := map[string]types.FacetRequest{}
	fields := make([]string, 0, len(req.Facets))
	for _, request := range req.Facets {
		requests[request.Field] = request
		if _, ok := in[request.Field]; ok {
			fields = append(fields, request.Field)
		}
	}
	if len(fields) == 0 {
		for field := range in {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	for _, field := range fields {
		request := requests[field]
		request.Field = field
		out = append(out, types.BuildFacet(request, in[field], types.SelectedFacetValues(req.Filters, field)))
	}
	return out
}

func normalizeConfig(cfg Config) Config {
	if cfg.GroupedEvidenceLimit <= 0 {
		cfg.GroupedEvidenceLimit = 5
	}
	if cfg.SuggestFetchMultiplier <= 0 {
		cfg.SuggestFetchMultiplier = 4
	}
	if cfg.SuggestMinimumFetchLimit <= 0 {
		cfg.SuggestMinimumFetchLimit = 10
	}
	if cfg.MultiSearchMinimumPerPage <= 0 {
		cfg.MultiSearchMinimumPerPage = 20
	}
	if cfg.ExactGroupCountPageSize <= 0 {
		cfg.ExactGroupCountPageSize = 250
	}
	if len(cfg.SuggestPreferParentFields) == 0 {
		cfg.SuggestPreferParentFields = []string{"title", "parent_title"}
	}
	if len(cfg.SuggestPreferParentWeights) == 0 {
		cfg.SuggestPreferParentWeights = []int{3, 5}
	}
	if len(cfg.SuggestDocumentFields) == 0 {
		cfg.SuggestDocumentFields = []string{"title", "parent_title", "body"}
	}
	if len(cfg.SuggestDocumentWeights) == 0 {
		cfg.SuggestDocumentWeights = []int{5, 3, 1}
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return cfg
}
