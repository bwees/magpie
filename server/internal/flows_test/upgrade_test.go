package flows_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"magpie/internal/acquisition"
	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/library"
	libraryapi "magpie/internal/library/api"
	subtitlesapi "magpie/internal/subtitles/api"
	transcodeapi "magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

const (
	upgradeTmdb     = domain.TmdbID(3003)
	upgradeName     = "Upgrade Flow"
	hdRelease       = "Upgrade.Flow.2024.720p.WEB-DL.DDP5.1.H.264-OLD"
	fullHDRelease   = "Upgrade.Flow.2024.1080p.WEB-DL.DDP5.1.H.264-NEW"
	hdGroup         = "720p"
	hdResolution    = 720
	spanishSidecar  = "Upgrade.Flow.2024.720p.WEB-DL.DDP5.1.H.264-OLD.es.srt"
	sidecarLanguage = domain.Language("es")
	// The old release is an .mp4, so the new .mkv lands next to it rather than over it.
	oldVideoExt = ".mp4"
)

// hdMovie is the 720p version of h264Movie.
func hdMovie() libraryapi.MediaInfo {
	info := h264Movie()
	info.Width, info.Height = hdWidth, hdHeight
	return info
}

// Flow 3: a 720p file below the 1080p cutoff is replaced by a 1080p release from the RSS feed. Library
// imports the new file over it and deletes the old one; Subtitles removes the old file's sidecar and
// Transcode cancels the Whisper job it had for the old file.
func TestRSSUpgradeReplacesTheFileAndCleansUpAfterIt(t *testing.T) {
	s := newStack(t)
	agent := connectAgent(s.harness, agentproto.CapWhisperVulkan)
	s.drain(agent)
	s.enableWhisper()
	s.editDefaultProfile(func(p *acquisition.ProfileRequest) {
		p.Items = append(p.Items, s.qualityGroup(hdGroup, hdResolution))
	})
	s.movieOnTMDB(upgradeTmdb, upgradeName, movieYear)
	first := release(hdRelease, releaseSeeders)
	s.answer(first)
	s.request(s.session, upgradeTmdb)
	title := s.titleAdded(upgradeTmdb)
	old := s.downloadNamed(first, hdMovie(), hdRelease+oldVideoExt, spanishSidecar)
	if len(old.Sidecars) != 1 || old.Sidecars[0].Language != sidecarLanguage || !exists(old.Sidecars[0].Path) {
		t.Fatalf("old import = %+v, want its Spanish sidecar placed", old)
	}
	oldSidecar := old.Sidecars[0].Path
	whisper := awaitEvent(s.harness, transcodeapi.TypeJobQueued, func(e transcodeapi.JobQueued) bool {
		return e.MediaFile == old.MediaFile && e.Kind == transcodeapi.JobKindWhisper
	})
	if cutoff := s.cutoff(title); len(cutoff) != 1 || cutoff[0].CutoffMet {
		t.Fatalf("cutoff = %+v, want the 720p file below it", cutoff)
	}

	better := release(fullHDRelease, releaseSeeders)
	s.prowlarr.SetRSS(flowIndexerID, better)
	if err := s.searches.SyncFeed(context.Background(), flowIndexerID); err != nil {
		t.Fatal(err)
	}
	if grab := s.grabbed(better); grab.Trigger != acquisitionapi.TriggerRSS {
		t.Fatalf("grab = %+v, want an RSS grab", grab)
	}
	upgraded := s.download(better, englishSubbedMovie())

	replaced := func() []string {
		var problems []string
		if upgraded.Replaces == nil || *upgraded.Replaces != old.MediaFile {
			problems = append(problems, fmt.Sprintf("FileImported.Replaces = %v, want the old file", upgraded.Replaces))
		}
		deleted := published(s.harness, libraryapi.TypeFileDeleted, func(e libraryapi.FileDeleted) bool { return e.MediaFile == old.MediaFile })
		if len(deleted) != 1 || deleted[0].Reason != libraryapi.DeleteUpgraded || deleted[0].Path != old.Path {
			problems = append(problems, fmt.Sprintf("FileDeleted = %+v, want one UPGRADED for the old file", deleted))
		}
		files := getJSON[[]library.MediaFileResponse](s.harness, libraryFilesPath+titleQuery+string(title))
		if len(files) != 1 || files[0].ID != upgraded.MediaFile || exists(old.Path) || !exists(upgraded.Path) {
			problems = append(problems, fmt.Sprintf("library files = %+v, old on disk %v; want only the new file", files, exists(old.Path)))
		}
		if exists(oldSidecar) {
			problems = append(problems, "the old file's sidecar is still on disk")
		}
		if job := s.transcodeJob(whisper.Job); job.State != transcodeapi.StateCancelled {
			problems = append(problems, fmt.Sprintf("old file's Whisper job = %+v, want CANCELLED", job))
		}
		cutoff := s.cutoff(title)
		if len(cutoff) != 1 || cutoff[0].File != upgraded.MediaFile || !cutoff[0].CutoffMet {
			problems = append(problems, fmt.Sprintf("cutoff = %+v, want only the new file, at its cutoff", cutoff))
		}
		settled := published(s.harness, subtitlesapi.TypeCoverageSettled, func(e subtitlesapi.CoverageSettled) bool { return e.MediaFile == upgraded.MediaFile })
		if len(settled) != 1 || !slices.Equal(settled[0].Satisfied, []domain.Language{english}) {
			problems = append(problems, fmt.Sprintf("new file's coverage = %+v, want English settled once", settled))
		}
		return problems
	}
	settles(t, "the upgrade", replaced)

	s.redeliverAll()
	settles(t, "the upgrade after redelivery", replaced)
}
