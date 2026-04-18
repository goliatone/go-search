package postgres

import (
	"fmt"
	"strings"

	"github.com/goliatone/go-search/pkg/types"
)

const defaultTableName = "search_documents"

type Config struct {
	DB                      BunDB
	TableName               string
	SearchConfig            string
	TrigramThreshold        float64
	SuggestTrigramThreshold float64
	SchemaManagement        SchemaManagementMode
	Clock                   types.Clock
}

type SchemaManagementMode string

const (
	SchemaManagementAuto     SchemaManagementMode = "auto"
	SchemaManagementExternal SchemaManagementMode = "external"
)

func normalizeConfig(cfg Config) (Config, error) {
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
	mode, err := normalizeSchemaManagementMode(cfg.SchemaManagement)
	if err != nil {
		return cfg, err
	}
	cfg.SchemaManagement = mode
	return cfg, nil
}

func normalizeSchemaManagementMode(mode SchemaManagementMode) (SchemaManagementMode, error) {
	switch SchemaManagementMode(strings.TrimSpace(strings.ToLower(string(mode)))) {
	case "", SchemaManagementAuto:
		return SchemaManagementAuto, nil
	case SchemaManagementExternal:
		return SchemaManagementExternal, nil
	default:
		return "", fmt.Errorf("invalid schema management mode %q", mode)
	}
}
