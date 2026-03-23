package postgres

import (
	"embed"

	persistence "github.com/goliatone/go-persistence-bun"
)

//go:embed migrations/postgres/*.sql
var migrationsFS embed.FS

func Migrations() *persistence.Migrations {
	manager := persistence.NewMigrations()
	manager.RegisterSQLMigrations(migrationsFS)
	return manager
}
