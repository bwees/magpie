package activity

import (
	"math"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
)

type searchFacet string

const (
	searchIdle            searchFacet = ""
	searchRunning         searchFacet = "RUNNING"
	searchNothingAccepted searchFacet = "NOTHING_ACCEPTED"
)

type downloadFacet string

const (
	downloadIdle   downloadFacet = ""
	downloadActive downloadFacet = "ACTIVE"
	downloadFailed downloadFacet = "FAILED"
	downloadDone   downloadFacet = "DONE"
)

type fileFacet string

const (
	fileNone    fileFacet = "NONE"
	filePresent fileFacet = "PRESENT"
)

const (
	detailUpgrade                 = "Upgrade"
	detailWaitingForJellyfin      = "Waiting for Jellyfin"
	detailNoAcceptedRelease       = "No accepted release"
	detailNoReleases              = "No releases found"
	detailWaitingForBetterRelease = "Waiting for a better release"
)

const percent = 100

// facets are what the pipeline knows about one movie or episode. The stage and chips derive from them.
type facets struct {
	Wanted   bool
	Search   searchFacet
	Download downloadFacet
	// DownloadID is the grab in progress while Download is ACTIVE.
	DownloadID domain.DownloadID
	File       fileFacet
	MediaFile  domain.MediaFileID
	Jellyfin   bool
	// Note is the latest event's own detail, such as a rejection or failure reason.
	Note      string
	Subs      *api.SubsChip
	Transcode *api.TranscodeChip
}

type stageOf struct {
	Stage  api.Stage
	Failed bool
	Detail string
}

// derive applies the stage rules top to bottom; the first match wins.
func derive(f facets) stageOf {
	switch {
	case f.Download == downloadActive:
		if f.File == filePresent {
			return stageOf{Stage: api.StageDownloading, Detail: detailUpgrade}
		}
		return stageOf{Stage: api.StageDownloading, Detail: f.Note}
	case f.File == fileNone && f.Download == downloadDone:
		return stageOf{Stage: api.StageImporting, Detail: f.Note}
	case f.File == fileNone && !f.Wanted:
		return stageOf{Stage: api.StageRequested}
	case f.File == fileNone && f.Download == downloadFailed:
		return stageOf{Stage: api.StageDownloading, Failed: true, Detail: f.Note}
	case f.File == fileNone && f.Search == searchRunning:
		return stageOf{Stage: api.StageSearching, Detail: f.Note}
	case f.File == fileNone && f.Search == searchNothingAccepted:
		return stageOf{Stage: api.StageSearching, Failed: true, Detail: f.Note}
	case f.File == fileNone:
		return stageOf{Stage: api.StageRequested, Detail: f.Note}
	case !f.Jellyfin:
		return stageOf{Stage: api.StageImporting, Detail: detailWaitingForJellyfin}
	default:
		return stageOf{Stage: api.StageAvailable}
	}
}

// savedPercentOf is how much smaller the new file is, rounded; nil when the old size is unknown.
func savedPercentOf(oldSize, newSize int64) *int {
	if oldSize <= 0 {
		return nil
	}
	saved := int(math.Round(float64(oldSize-newSize) * percent / float64(oldSize)))
	return &saved
}
