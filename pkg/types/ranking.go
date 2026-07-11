package types

import (
	"context"
	"strings"
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
	Editorial []AppliedEditorialSignal    `json:"editorial"`
	Signals   []AppliedSignalContribution `json:"signals,omitempty"`
	Diversity []DiversityEvidence         `json:"diversity,omitempty"`
	Metadata  map[string]any              `json:"metadata"`
}
type DiversityEvidence struct {
	Kind       string  `json:"kind"`
	Key        string  `json:"key"`
	Occurrence int     `json:"occurrence,omitempty"`
	Penalty    float64 `json:"penalty,omitempty"`
	Suppressed bool    `json:"suppressed,omitempty"`
}
type AppliedSignalContribution struct {
	ID           string  `json:"id"`
	Value        float64 `json:"value"`
	Contribution float64 `json:"contribution"`
	Reason       string  `json:"reason,omitempty"`
}

type AppliedEditorialSignal struct {
	RuleID string  `json:"rule_id"`
	Action string  `json:"action"`
	Weight float64 `json:"weight"`
	Scope  string  `json:"scope"`
	Reason string  `json:"reason"`
}

type AppliedRetrievalSignals struct {
	Mode          SearchMode              `json:"mode"`
	ProviderScore *float64                `json:"provider_score"`
	SemanticScore *float64                `json:"semantic_score"`
	LexicalScore  *float64                `json:"lexical_score"`
	HybridScore   *float64                `json:"hybrid_score"`
	Distance      *float64                `json:"distance"`
	Field         string                  `json:"field"`
	Model         string                  `json:"model"`
	Metadata      map[string]any          `json:"metadata"`
	Contributions []RetrievalContribution `json:"contributions,omitempty"`
}

type RetrievalContribution struct {
	Index         string   `json:"index"`
	ProviderRank  int      `json:"provider_rank"`
	ProviderScore *float64 `json:"provider_score,omitempty"`
	Contribution  float64  `json:"contribution"`
}

type EditorialRuleStore interface {
	ListApplicable(ctx context.Context, req SearchRequest) ([]EditorialRankRule, error)
	Upsert(ctx context.Context, rule EditorialRankRule) error
	Delete(ctx context.Context, id string) error
}

type EditorialRuleAdminStore interface {
	EditorialRuleStore
	List(ctx context.Context, req EditorialRuleListRequest) ([]EditorialRankRule, error)
	SetEnabled(ctx context.Context, id string, enabled bool) error
}

type EditorialRuleListRequest struct {
	Indexes []string `json:"indexes"`
	Locale  string   `json:"locale"`
	Enabled *bool    `json:"enabled"`
}

func (EditorialRuleListRequest) Type() string { return "search::list_editorial_rules" }

type UpsertEditorialRuleInput struct {
	Rule EditorialRankRule `json:"rule"`
}

func (UpsertEditorialRuleInput) Type() string { return "search::upsert_editorial_rule" }

type DeleteEditorialRuleInput struct {
	ID string `json:"id"`
}

func (DeleteEditorialRuleInput) Type() string { return "search::delete_editorial_rule" }

type SetEditorialRuleEnabledInput struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

func (SetEditorialRuleEnabledInput) Type() string { return "search::set_editorial_rule_enabled" }

func (rule EditorialRankRule) RequiresParentTarget() bool {
	return strings.TrimSpace(rule.ParentTargetID) == "" && strings.TrimSpace(rule.TargetID) != ""
}

type GenerationStore interface {
	Get(ctx context.Context, index string) (int64, error)
	Bump(ctx context.Context, index string) (int64, error)
}
