package library_test

import (
	"context"
	"testing"

	"magpie/internal/library"
	"magpie/internal/library/api"
	"magpie/internal/library/api/contracttest"
	"magpie/internal/platform/db/dbtest"
)

func TestRepositoryMeetsTheContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, library.Migrations())
		if err := library.SeedFiles(context.Background(), d, f.Files); err != nil {
			t.Fatal(err)
		}
		return library.NewRepository(d)
	})
}
