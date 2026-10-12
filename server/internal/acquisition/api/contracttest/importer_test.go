package contracttest

import (
	"testing"

	"magpie/internal/acquisition/api"
)

func TestImporterFakeMeetsTheContract(t *testing.T) {
	RunImporter(t, func(*testing.T) api.Importer { return NewImporterFake() })
}
