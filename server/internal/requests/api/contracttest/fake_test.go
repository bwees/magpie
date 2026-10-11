package contracttest

import (
	"testing"

	"magpie/internal/requests/api"
)

func TestFakeMeetsTheContract(t *testing.T) {
	Run(t, func(_ *testing.T, f Fixture) api.Reader { return NewFake(f) })
}
