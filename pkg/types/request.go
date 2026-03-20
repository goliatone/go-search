package types

type SearchMode string

const (
	SearchModeLexical  SearchMode = "lexical"
	SearchModeSemantic SearchMode = "semantic"
	SearchModeHybrid   SearchMode = "hybrid"
)

type SearchRequest struct {
	Indexes        []string
	Query          string
	Locale         string
	Locales        []string
	Page           int
	PerPage        int
	Sort           []Sort
	Filters        FilterExpr
	Facets         []FacetRequest
	GroupBy        string
	Highlight      []string
	IncludeFields  []string
	RankingProfile string
	Mode           SearchMode
	Semantic       *SemanticRequest
	Metadata       map[string]any
	Actor          ActorRef
	Scope          Scope
	Request        any
}

func (SearchRequest) Type() string { return "search::search" }

type SemanticRequest struct {
	Field             string
	QueryText         string
	QueryEmbedding    []float32
	K                 int
	DistanceThreshold *float64
	Alpha             *float64
	Rerank            bool
	LocaleStrategy    string
	Model             string
	Metadata          map[string]any
}

type SuggestRequest struct {
	Indexes      []string
	Query        string
	Locale       string
	Limit        int
	PreferParent bool
	Metadata     map[string]any
	Actor        ActorRef
	Scope        Scope
}

func (SuggestRequest) Type() string { return "search::suggest" }

type HealthRequest struct {
	Indexes []string
}

func (HealthRequest) Type() string { return "search::health" }

type StatsRequest struct {
	Indexes []string
}

func (StatsRequest) Type() string { return "search::stats" }
