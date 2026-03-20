package types

import (
	"context"
	"time"
)

const (
	EditorialActionBoost = "boost"
	EditorialActionBury  = "bury"
	EditorialActionPin   = "pin"
	EditorialActionHide  = "hide"
)

type EditorialRankRule struct {
	ID             string         `json:"id"`
	TargetType     string         `json:"target_type"`
	TargetID       string         `json:"target_id"`
	ParentTargetID string         `json:"parent_target_id"`
	Action         string         `json:"action"`
	Weight         float64        `json:"weight"`
	Position       *int           `json:"position"`
	Enabled        bool           `json:"enabled"`
	Scope          EditorialScope `json:"scope"`
	StartsAt       *time.Time     `json:"starts_at"`
	EndsAt         *time.Time     `json:"ends_at"`
	Reason         string         `json:"reason"`
	Metadata       map[string]any `json:"metadata"`
}

type EditorialScope struct {
	Indexes        []string            `json:"indexes"`
	TenantID       string              `json:"tenant_id"`
	OrgID          string              `json:"org_id"`
	Locale         string              `json:"locale"`
	Topic          string              `json:"topic"`
	Query          string              `json:"query"`
	RankingProfile string              `json:"ranking_profile"`
	Filters        map[string][]string `json:"filters"`
}

type AppliedRankingSignals struct {
	Editorial []AppliedEditorialSignal `json:"editorial"`
	Metadata  map[string]any           `json:"metadata"`
}

type AppliedEditorialSignal struct {
	RuleID string  `json:"rule_id"`
	Action string  `json:"action"`
	Weight float64 `json:"weight"`
	Scope  string  `json:"scope"`
	Reason string  `json:"reason"`
}

type AppliedRetrievalSignals struct {
	Mode          SearchMode     `json:"mode"`
	ProviderScore *float64       `json:"provider_score"`
	SemanticScore *float64       `json:"semantic_score"`
	LexicalScore  *float64       `json:"lexical_score"`
	HybridScore   *float64       `json:"hybrid_score"`
	Distance      *float64       `json:"distance"`
	Field         string         `json:"field"`
	Model         string         `json:"model"`
	Metadata      map[string]any `json:"metadata"`
}

type EditorialRuleStore interface {
	ListApplicable(ctx context.Context, req SearchRequest) ([]EditorialRankRule, error)
	Upsert(ctx context.Context, rule EditorialRankRule) error
	Delete(ctx context.Context, id string) error
}

type GenerationStore interface {
	Get(ctx context.Context, index string) (int64, error)
	Bump(ctx context.Context, index string) (int64, error)
}
