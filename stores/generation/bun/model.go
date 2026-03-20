package bunstore

import "github.com/uptrace/bun"

type GenerationModel struct {
	bun.BaseModel `bun:"table:search_generations,alias:sg"`
	IndexName     string `bun:"index_name,pk"`
	Generation    int64  `bun:",notnull"`
	LastIndexedAt int64  `bun:",notnull"`
}
