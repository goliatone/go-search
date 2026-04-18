package postgres

import (
	"embed"
	"io/fs"

	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/goliatone/go-search/internal/migrationutil"
)

//go:embed migrations
var migrationsFS embed.FS

func GetMigrationsFS() (fs.FS, error) {
	return fs.Sub(migrationsFS, "migrations")
}

func Migrations() *persistence.Migrations {
	manager := persistence.NewMigrations()
	root, err := GetMigrationsFS()
	if err == nil {
		manager.RegisterDialectMigrations(
			root,
			persistence.WithDialectResolver(migrationutil.RequirePostgresDialect),
			persistence.WithValidationTargets(),
			persistence.WithValidateOnMigrate(true),
		)
	}
	return manager
}
