package identity

import (
	"context"
	"testing"

	"magpie/internal/identity/api"
	"magpie/internal/identity/api/contracttest"
	"magpie/internal/platform/db/dbtest"

	"gorm.io/gorm"
)

// Seeds 11, 12 and 16: the real repository passes the same Reader suite as the fake.
func TestRepositoryMeetsTheContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T, f contracttest.Fixture) api.Reader {
		d := dbtest.Open(t, Migrations())
		repo := NewRepository(d)
		err := d.Write(context.Background(), func(tx *gorm.DB) error {
			for _, u := range f.Users {
				if err := repo.insertUser(tx, rowFromUser(u)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return repo
	})
}
