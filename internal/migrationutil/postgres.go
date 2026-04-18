package migrationutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

func RequirePostgresDialect(_ context.Context, db *bun.DB) (string, error) {
	if db == nil || db.Dialect() == nil {
		return "postgres", nil
	}
	name := strings.ToLower(strings.TrimSpace(db.Dialect().Name().String()))
	switch name {
	case "postgres", "postgresql", "pg", "pgdialect":
		return "postgres", nil
	default:
		return "", fmt.Errorf("postgres-only migrations require a postgres dialect, got %q", name)
	}
}
