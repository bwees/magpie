package flows_test

import (
	"net/http"
	"slices"
	"testing"

	"magpie/internal/acquisition"
	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/catalog"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
)

const (
	profileFlowTmdb = domain.TmdbID(9009)
	profileFlowName = "Profile Flow"

	catalogTitlesPath   = "/api/catalog/titles/"
	acquisitionSettings = "/api/acquisition/settings"
)

func (s *stack) presetProfile(preset acquisitionapi.QualityPreset) domain.ProfileID {
	s.t.Helper()
	summaries := getJSON[[]acquisition.ProfileSummaryResponse](s.harness, profilesPath)
	i := slices.IndexFunc(summaries, func(p acquisition.ProfileSummaryResponse) bool { return p.Preset == preset })
	if i < 0 {
		s.t.Fatalf("profiles = %+v, want the %s preset", summaries, preset)
	}
	return summaries[i].ID
}

// A title on a deleted profile moves to the default profile, and searching it still works.
func TestDeletingAProfileMovesItsTitlesToTheDefaultAndTheyStillSearch(t *testing.T) {
	s := newStack(t)
	s.movieOnTMDB(profileFlowTmdb, profileFlowName, movieYear)
	s.request(s.session, profileFlowTmdb)
	title := s.titleAdded(profileFlowTmdb)
	uhd := s.presetProfile(acquisitionapi.Preset2160p)
	if rec := s.do(http.MethodPatch, catalogTitlesPath+string(title), catalog.UpdateTitleRequest{ProfileID: &uhd}); rec.Code != http.StatusOK {
		t.Fatalf("move title to 2160p = %d: %s", rec.Code, rec.Body)
	}

	if rec := s.do(http.MethodDelete, profilesPath+"/"+string(uhd), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete profile = %d: %s", rec.Code, rec.Body)
	}

	fallback := getJSON[acquisition.AcquisitionSettingsResponse](s.harness, acquisitionSettings).DefaultProfileID
	awaitEvent(s.harness, catalogapi.TypeProfileChanged, func(e catalogapi.ProfileChanged) bool {
		return e == catalogapi.ProfileChanged{Title: title, From: uhd, To: fallback}
	})
	if got := getJSON[catalog.TitleResponse](s.harness, catalogTitlesPath+string(title)); got.ProfileID != fallback {
		t.Errorf("title profile = %s, want the default %s", got.ProfileID, fallback)
	}
	if search := s.searchByHand(title); search.Profile.ID != fallback {
		t.Errorf("search profile = %s, want the default %s", search.Profile.ID, fallback)
	}
}
