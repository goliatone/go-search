package types

type SearchMode string

const (
	SearchModeLexical  SearchMode = "lexical"
	SearchModeSemantic SearchMode = "semantic"
	SearchModeHybrid   SearchMode = "hybrid"
)

type SearchRequest struct {
	Indexes        []string         `json:"indexes"`
	Query          string           `json:"query"`
	Locale         string           `json:"locale"`
	Locales        []string         `json:"locales"`
	Page           int              `json:"page"`
	PerPage        int              `json:"per_page"`
	Sort           []Sort           `json:"sort"`
	Filters        FilterExpr       `json:"filters"`
	Facets         []FacetRequest   `json:"facets"`
	GroupBy        string           `json:"group_by"`
	Highlight      []string         `json:"highlight"`
	IncludeFields  []string         `json:"include_fields"`
	RankingProfile string           `json:"ranking_profile"`
	Mode           SearchMode       `json:"mode"`
	Semantic       *SemanticRequest `json:"semantic"`
	Metadata       map[string]any   `json:"metadata"`
	Actor          ActorRef         `json:"actor"`
	Scope          Scope            `json:"scope"`
	Request        any              `json:"request"`
}

func (SearchRequest) Type() string { return "search::search" }

type SemanticRequest struct {
	Field             string         `json:"field"`
	QueryText         string         `json:"query_text"`
	QueryEmbedding    []float32      `json:"query_embedding"`
	K                 int            `json:"k"`
	DistanceThreshold *float64       `json:"distance_threshold"`
	Alpha             *float64       `json:"alpha"`
	Rerank            bool           `json:"rerank"`
	LocaleStrategy    string         `json:"locale_strategy"`
	Model             string         `json:"model"`
	Metadata          map[string]any `json:"metadata"`
}

type SuggestRequest struct {
	Indexes      []string       `json:"indexes"`
	Query        string         `json:"query"`
	Locale       string         `json:"locale"`
	Limit        int            `json:"limit"`
	PreferParent bool           `json:"prefer_parent"`
	Metadata     map[string]any `json:"metadata"`
	Actor        ActorRef       `json:"actor"`
	Scope        Scope          `json:"scope"`
}

func (SuggestRequest) Type() string { return "search::suggest" }

type HealthRequest struct {
	Indexes []string `json:"indexes"`
}

func (HealthRequest) Type() string { return "search::health" }

type StatsRequest struct {
	Indexes []string `json:"indexes"`
}

func (StatsRequest) Type() string { return "search::stats" }
