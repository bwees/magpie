package contracttest

import (
	"testing"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
)

func TestImporterFakeMeetsTheContract(t *testing.T) {
	RunImporter(t, func(*testing.T, domain.TitleID) api.Importer { return NewImporterFake() })
}
