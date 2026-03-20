package bunstore

import "github.com/uptrace/bun"

type RuleModel struct {
	bun.BaseModel  `bun:"table:search_editorial_rules,alias:ser"`
	ID             string              `bun:",pk"`
	TargetType     string              `bun:",notnull"`
	TargetID       string              `bun:",notnull"`
	ParentTargetID string              `bun:",nullzero"`
	Action         string              `bun:",notnull"`
	Weight         float64             `bun:",notnull"`
	Position       *int                `bun:",nullzero"`
	Enabled        bool                `bun:",notnull"`
	Indexes        []string            `bun:"indexes,array"`
	TenantID       string              `bun:",nullzero"`
	OrgID          string              `bun:",nullzero"`
	Locale         string              `bun:",nullzero"`
	Topic          string              `bun:",nullzero"`
	Query          string              `bun:",nullzero"`
	RankingProfile string              `bun:",nullzero"`
	Filters        map[string][]string `bun:"filters,type:jsonb"`
	StartsAtUnix   *int64              `bun:",nullzero"`
	EndsAtUnix     *int64              `bun:",nullzero"`
	Reason         string              `bun:",nullzero"`
	Metadata       map[string]any      `bun:"metadata,type:jsonb"`
}
