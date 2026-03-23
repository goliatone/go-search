package postgres

import (
	"strings"
	"time"

	"github.com/goliatone/go-search/pkg/types"
)

const defaultTableName = "search_documents"

type Config struct {
	DB                      BunDB
	TableName               string
	SearchConfig            string
	TrigramThreshold        float64
	SuggestTrigramThreshold float64
	Clock                   types.Clock
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.TableName) == "" {
		cfg.TableName = defaultTableName
	}
	if strings.TrimSpace(cfg.SearchConfig) == "" {
		cfg.SearchConfig = "simple"
	}
	if cfg.TrigramThreshold <= 0 {
		cfg.TrigramThreshold = 0.12
	}
	if cfg.SuggestTrigramThreshold <= 0 {
		cfg.SuggestTrigramThreshold = 0.18
	}
	if cfg.Clock == nil {
		cfg.Clock = types.SystemClock()
	}
	return cfg
}

func defaultNow() time.Time {
	return time.Now().UTC()
}
