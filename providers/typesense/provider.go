package typesense

import (
	"context"
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
		Grouping:             true,
		Highlighting:         true,
		Snippets:             true,
		PrefixSearch:         true,
		SupportedSearchModes: []types.SearchMode{types.SearchModeLexical},
		Metadata: map[string]any{
			"grouped_evidence_limit": p.cfg.GroupedEvidenceLimit,
		},
	}, nil
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

func (p *Provider) DeleteBySource(ctx context.Context, index string, sourceIDs []string) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	return deleteBySource(ctx, p.client, runtime, sourceIDs)
}

func (p *Provider) ReplaceDocuments(ctx context.Context, index string, sourceIDs []string, docs []types.Document) error {
	runtime, err := p.runtimeFor(index)
	if err != nil {
		return err
	}
	return replaceDocuments(ctx, p.client, runtime, sourceIDs, docs)
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
	if req.GroupBy != "" {
		total, err := p.exactGroupCount(ctx, runtime, req)
		if err != nil {
			return types.SearchResultPage{}, err
		}
		page.Total = total
	}
	return page, nil
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
	allGroups := []types.SearchGroup{}
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
		for _, group := range page.Groups {
			allGroups = append(allGroups, group)
		}
		mergeFacets(facets, page.Facets)
	}

	if req.GroupBy != "" {
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
	aggregate.Facets = flattenFacetMap(facets)
	return aggregate, nil
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
		params.Page = intPtr(pageNumber)
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
	if left.Score == right.Score {
		return left.ID < right.ID
	}
	return left.Score > right.Score
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

func flattenFacetMap(in map[string]map[string]int) []types.SearchFacet {
	out := make([]types.SearchFacet, 0, len(in))
	fields := make([]string, 0, len(in))
	for field := range in {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		values := make([]types.SearchFacetValue, 0, len(in[field]))
		for value, count := range in[field] {
			values = append(values, types.SearchFacetValue{Value: value, Count: count})
		}
		sort.SliceStable(values, func(i, j int) bool {
			if values[i].Count == values[j].Count {
				return values[i].Value < values[j].Value
			}
			return values[i].Count > values[j].Count
		})
		out = append(out, types.SearchFacet{Field: field, Values: values})
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
