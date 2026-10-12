package subtitles_test

import (
	"context"
	"testing"

	"magpie/internal/platform/db/dbtest"
	"magpie/internal/subtitles"
	"magpie/internal/subtitles/api"
	"magpie/internal/subtitles/api/contracttest"
)

func TestRepositoryMeetsTheContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, subtitles.Migrations())
		if err := subtitles.SeedReader(context.Background(), d, f); err != nil {
			t.Fatal(err)
		}
		return subtitles.NewRepository(d)
	})
}
