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
	ID             string
	TargetType     string
	TargetID       string
	ParentTargetID string
	Action         string
	Weight         float64
	Position       *int
	Enabled        bool
	Scope          EditorialScope
	StartsAt       *time.Time
	EndsAt         *time.Time
	Reason         string
	Metadata       map[string]any
}

type EditorialScope struct {
	Indexes        []string
	TenantID       string
	OrgID          string
	Locale         string
	Topic          string
	Query          string
	RankingProfile string
	Filters        map[string][]string
}

type AppliedRankingSignals struct {
	Editorial []AppliedEditorialSignal
	Metadata  map[string]any
}

type AppliedEditorialSignal struct {
	RuleID string
	Action string
	Weight float64
	Scope  string
	Reason string
}

type AppliedRetrievalSignals struct {
	Mode          SearchMode
	ProviderScore *float64
	SemanticScore *float64
	LexicalScore  *float64
	HybridScore   *float64
	Distance      *float64
	Field         string
	Model         string
	Metadata      map[string]any
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
