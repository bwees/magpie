package catalog

import (
	"embed"
	"io/fs"

	"magpie/internal/platform/db"
	"magpie/internal/platform/owner"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Migrations is Catalog's goose directory.
func Migrations() db.MigrationSet {
	sub, err := fs.Sub(embeddedMigrations, migrationsDir)
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail
	}
	return db.MigrationSet{Owner: owner.Catalog, FS: sub}
}
