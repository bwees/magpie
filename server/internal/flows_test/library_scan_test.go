package flows_test

import (
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/catalog"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/transcode"
	transcodeapi "magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

const (
	exactTmdb   = domain.TmdbID(7001)
	exactName   = "Exact Flow"
	exactYear   = 2023
	exactFolder = "Exact Flow (2023) [tmdbid-7001]"
	exactVideo  = "Exact Flow (2023).mkv"

	// The folder says "Alien" and TMDB only knows "Aliens": alike, but not enough to match without asking.
	fuzzyTmdb   = domain.TmdbID(7002)
	fuzzyName   = "Aliens"
	fuzzyYear   = 1986
	fuzzyFolder = "Alien (1986)"
	fuzzyVideo  = "Alien (1986).mkv"

	transcodeBacklogPath = "/api/transcode/backlog"
)

// scanned is one folder that a scan linked into the library.
type scanned struct {
	folder string
	title  domain.TitleID
	file   libraryapi.FileImported
}

// placeMovie puts a movie folder holding one video in the Movies root folder.
func (s *stack) placeMovie(folder, video string) string {
	s.t.Helper()
	path := filepath.Join(s.movies, folder, video)
	s.writeVideo(path, h264Movie())
	return path
}

// linked waits until a folder's movie is added as LIBRARY_SCAN and its video linked in place.
func (s *stack) linked(tmdb domain.TmdbID, folder string) scanned {
	s.t.Helper()
	added := awaitEvent(s.harness, catalogapi.TypeTitleAdded, func(e catalogapi.TitleAdded) bool { return e.External.Tmdb == tmdb })
	file := awaitEvent(s.harness, libraryapi.TypeFileImported, func(e libraryapi.FileImported) bool { return e.Title == added.Title })
	return scanned{folder: filepath.Join(s.movies, folder), title: added.Title, file: file}
}

// scannedProblems lists what is wrong if a linked folder didn't follow flow 7 to a queued backlog job.
func (s *stack) scannedProblems(found scanned, video string) []string {
	s.t.Helper()
	var problems []string
	added := published(s.harness, catalogapi.TypeTitleAdded, func(e catalogapi.TitleAdded) bool { return e.Title == found.title })
	if len(added) != 1 || added[0].Origin != domain.OriginLibraryScan || added[0].ScanFolder != found.folder {
		problems = append(problems, fmt.Sprintf("TitleAdded = %+v, want one LIBRARY_SCAN for %s", added, found.folder))
	}
	matched := published(s.harness, catalogapi.TypeScanFolderMatched, func(e catalogapi.ScanFolderMatched) bool { return e.Title == found.title })
	if len(matched) != 1 || matched[0].Folder != found.folder {
		problems = append(problems, fmt.Sprintf("ScanFolderMatched = %+v, want one for %s", matched, found.folder))
	}
	imported := published(s.harness, libraryapi.TypeFileImported, func(e libraryapi.FileImported) bool { return e.Title == found.title })
	if len(imported) != 1 || imported[0].Origin != domain.OriginLibraryScan || imported[0].Download != nil ||
		imported[0].Path != filepath.Join(found.folder, video) {
		problems = append(problems, fmt.Sprintf("FileImported = %+v, want one LIBRARY_SCAN linked in place", imported))
	}
	met := published(s.harness, acquisitionapi.TypeCutoffMet, func(e acquisitionapi.CutoffMet) bool { return e.File == found.file.MediaFile })
	if len(met) != 1 || met[0].Origin != domain.OriginLibraryScan {
		problems = append(problems, fmt.Sprintf("CutoffMet = %+v, want one LIBRARY_SCAN; cutoff %+v", met, s.cutoff(found.title)))
	}
	queued := published(s.harness, transcodeapi.TypeJobQueued, func(e transcodeapi.JobQueued) bool { return e.MediaFile == found.file.MediaFile })
	if len(queued) != 1 || queued[0].Origin != domain.OriginLibraryScan || queued[0].Kind != transcodeapi.JobKindTranscode {
		problems = append(problems, fmt.Sprintf("JobQueued = %+v, want one LIBRARY_SCAN transcode", queued))
	}
	return problems
}

// Flow 7: a scan finds two movie folders. The one named with its TMDB ID is matched at once; the one only
// like a TMDB title waits in the review queue until the admin picks the match. Both are linked in place,
// never searched, meet their cutoff and queue transcodes in the backlog behind downloads.
func TestLibraryScanLinksFoldersAndQueuesBacklogTranscodes(t *testing.T) {
	s := newStack(t)
	agent := connectAgent(s.harness, agentproto.CapEncHEVC10)
	s.drain(agent)
	s.movieOnTMDB(exactTmdb, exactName, exactYear)
	s.movieOnTMDB(fuzzyTmdb, fuzzyName, fuzzyYear)
	s.placeMovie(exactFolder, exactVideo)
	s.placeMovie(fuzzyFolder, fuzzyVideo)

	s.scan()
	exact := s.linked(exactTmdb, exactFolder)

	var reviews []catalog.ScanReviewResponse
	eventually(t, "the unsure folder waits for review", func() bool {
		reviews = getJSON[[]catalog.ScanReviewResponse](s.harness, scanReviewPath)
		return len(reviews) == 1
	})
	review := reviews[0]
	if review.FolderPath != filepath.Join(s.movies, fuzzyFolder) ||
		!slices.ContainsFunc(review.Candidates, func(c catalog.ScanCandidateResponse) bool { return c.TmdbID == fuzzyTmdb }) {
		t.Fatalf("review = %+v, want %s with %s as a candidate", review, fuzzyFolder, fuzzyName)
	}
	if added := published(s.harness, catalogapi.TypeTitleAdded, func(e catalogapi.TitleAdded) bool { return e.External.Tmdb == fuzzyTmdb }); len(added) != 0 {
		t.Fatalf("the unsure folder was added before review: %+v", added)
	}
	rec := s.do(http.MethodPost, scanReviewPath+"/"+review.ID+"/match", catalog.MatchScanReviewRequest{TmdbID: fuzzyTmdb})
	if rec.Code != http.StatusOK {
		t.Fatalf("match review = %d: %s", rec.Code, rec.Body)
	}
	fuzzy := s.linked(fuzzyTmdb, fuzzyFolder)

	settledScan := func() []string {
		problems := append(s.scannedProblems(exact, exactVideo), s.scannedProblems(fuzzy, fuzzyVideo)...)
		if searches := s.outbox(acquisitionapi.TypeSearchStarted); len(searches) != 0 {
			problems = append(problems, fmt.Sprintf("searches started: %v, want none", searches))
		}
		if backlog := getJSON[transcode.BacklogResponse](s.harness, transcodeBacklogPath); backlog.Backlog != 2 {
			problems = append(problems, fmt.Sprintf("backlog = %+v, want both files queued", backlog))
		}
		return problems
	}
	settles(t, "the scanned folders", settledScan)

	s.redeliverAll()
	settles(t, "the scanned folders after redelivery", settledScan)
}
