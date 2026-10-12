package contracttest

import (
	"testing"

	"magpie/internal/library/api"
)

func TestImporterFakeMeetsTheContract(t *testing.T) {
	RunImporter(t, func(_ *testing.T, w ImporterWorld) api.Importer { return NewImporterFake(w) })
}
