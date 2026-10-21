package activity

import (
	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/platform/bus"
	subtitlesapi "magpie/internal/subtitles/api"
	transcodeapi "magpie/internal/transcode/api"
)

// rowTarget selects the pipeline rows an event changes. Exactly one way of selecting is set:
// a file, the rows downloading a download now, a download or search (falling back to the title's rows),
// or the title itself.
type rowTarget struct {
	Title domain.TitleID
	// Movie selects the movie's single row.
	Movie bool
	// Episodes selects these episodes' rows; empty selects every row of the title.
	Episodes    []domain.EpisodeID
	File        domain.MediaFileID
	Downloading domain.DownloadID
	Download    domain.DownloadID
	Search      domain.SearchID
}

// pipelineChange is one facet change. Create adds the target's missing rows first.
type pipelineChange struct {
	Target rowTarget
	Create bool
	Apply  func(*facets)
}

var pipelineChanges = map[string]func(bus.Event) ([]pipelineChange, error){}

// onPipeline registers how events of one payload type change pipeline facets.
func onPipeline[T domain.Message](fn func(T) []pipelineChange) {
	var zero T
	pipelineChanges[zero.Type()] = func(ev bus.Event) ([]pipelineChange, error) {
		payload, err := bus.Decode[T](ev)
		if err != nil {
			return nil, err
		}
		return fn(payload), nil
	}
}

func change(target rowTarget, apply func(*facets)) []pipelineChange {
	return []pipelineChange{{Target: target, Apply: apply}}
}

func setWanted(wanted bool) func(*facets) {
	return func(f *facets) { f.Wanted = wanted }
}

func setTranscode(chip api.TranscodeChip) func(*facets) {
	return func(f *facets) { f.Transcode = &chip }
}

func init() {
	onPipeline(func(e catalogapi.TitleAdded) []pipelineChange {
		target := rowTarget{Title: e.Title, Movie: e.Kind == domain.MediaKindMovie, Episodes: e.Monitored}
		return []pipelineChange{{Target: target, Create: true, Apply: setWanted(true)}}
	})
	onPipeline(func(e catalogapi.MonitoringChanged) []pipelineChange {
		if e.Movie != nil {
			return []pipelineChange{{Target: rowTarget{Title: e.Title, Movie: true}, Create: *e.Movie, Apply: setWanted(*e.Movie)}}
		}
		var changes []pipelineChange
		if len(e.Added) > 0 {
			changes = append(changes, pipelineChange{Target: rowTarget{Title: e.Title, Episodes: e.Added}, Create: true, Apply: setWanted(true)})
		}
		if len(e.Removed) > 0 {
			changes = append(changes, pipelineChange{Target: rowTarget{Title: e.Title, Episodes: e.Removed}, Apply: setWanted(false)})
		}
		return changes
	})

	onPipeline(func(e catalogapi.EpisodesUnreleased) []pipelineChange {
		return change(rowTarget{Title: e.Title, Episodes: e.Episodes}, func(f *facets) {
			f.Search, f.Note = searchIdle, ""
		})
	})

	onPipeline(func(e acquisitionapi.SearchStarted) []pipelineChange {
		return change(rowTarget{Title: e.Title, Episodes: e.Episodes}, func(f *facets) {
			f.Search, f.Note = searchRunning, ""
		})
	})
	onPipeline(func(e acquisitionapi.SearchCompleted) []pipelineChange {
		return change(rowTarget{Title: e.Title, Search: e.Search}, func(f *facets) {
			switch e.Outcome {
			case acquisitionapi.OutcomeNothingAccepted:
				f.Search, f.Note = searchNothingAccepted, detailNoAcceptedRelease
			case acquisitionapi.OutcomeNoReleases:
				f.Search, f.Note = searchIdle, detailNoReleases
			case acquisitionapi.OutcomePendingDelay:
				f.Search, f.Note = searchIdle, detailWaitingForBetterRelease
			default:
				f.Search, f.Note = searchIdle, ""
			}
		})
	})
	onPipeline(func(e acquisitionapi.ReleaseRejected) []pipelineChange {
		note := rejectedNote(e.Reason)
		changes := change(rowTarget{Title: e.Title}, func(f *facets) {
			if f.Search == searchRunning || f.Download == downloadActive {
				f.Note = note
			}
		})
		if e.Download == nil {
			return changes
		}
		// Dropped rows go first: a row takes one change per event, so the title's note-only change would claim them.
		dropped := pipelineChange{Target: rowTarget{Downloading: *e.Download}, Apply: func(f *facets) {
			f.Download, f.Note = downloadIdle, note
		}}
		return append([]pipelineChange{dropped}, changes...)
	})
	onPipeline(func(e acquisitionapi.ReleaseGrabbed) []pipelineChange {
		return change(rowTarget{Title: e.Title, Episodes: e.Episodes}, func(f *facets) {
			f.Download, f.DownloadID, f.Note = downloadActive, e.Download, ""
		})
	})
	onPipeline(func(e acquisitionapi.DownloadFailed) []pipelineChange {
		return change(rowTarget{Downloading: e.Download}, func(f *facets) {
			f.Download, f.Note = downloadFailed, e.Reason
		})
	})
	onPipeline(func(e acquisitionapi.DownloadRemoved) []pipelineChange {
		return change(rowTarget{Downloading: e.Download}, func(f *facets) { f.Download = downloadIdle })
	})
	onPipeline(func(e acquisitionapi.DownloadCompleted) []pipelineChange {
		return change(rowTarget{Title: e.Title, Episodes: e.Episodes}, func(f *facets) {
			f.Download, f.Note = downloadDone, ""
		})
	})

	onPipeline(func(e libraryapi.FileImported) []pipelineChange {
		return change(rowTarget{Title: e.Title, Episodes: episodeIDs(e.Episodes)}, func(f *facets) {
			f.File, f.MediaFile, f.Jellyfin = filePresent, e.MediaFile, false
			f.Download, f.Note = downloadIdle, ""
			f.Subs = &api.SubsChip{State: api.SubsPending, Satisfied: []domain.Language{}, Missing: []domain.Language{}}
			f.Transcode = nil
		})
	})
	onPipeline(func(e libraryapi.ImportFailed) []pipelineChange {
		return change(rowTarget{Download: e.Download}, func(f *facets) {
			f.Download, f.Note = downloadFailed, e.Reason
		})
	})
	onPipeline(func(e libraryapi.ItemAvailable) []pipelineChange {
		return change(rowTarget{Title: e.Title, Episodes: e.Episodes}, func(f *facets) { f.Jellyfin = true })
	})
	onPipeline(func(e libraryapi.FileTranscoded) []pipelineChange {
		return change(rowTarget{File: e.MediaFile}, setTranscode(api.TranscodeChip{
			State: api.TranscodeDone, Saved: savedPercentOf(e.OldSize, e.NewSize),
		}))
	})
	onPipeline(func(e libraryapi.FileDeleted) []pipelineChange {
		if e.Reason == libraryapi.DeleteUpgraded {
			return nil
		}
		return change(rowTarget{File: e.MediaFile}, func(f *facets) {
			f.File, f.MediaFile, f.Jellyfin = fileNone, "", false
			f.Subs, f.Transcode = nil, nil
		})
	})

	onPipeline(func(e subtitlesapi.CoverageSettled) []pipelineChange {
		return change(rowTarget{File: e.MediaFile}, func(f *facets) {
			f.Subs = &api.SubsChip{State: api.SubsSettled, Satisfied: nonNil(e.Satisfied), Missing: nonNil(e.Missing)}
		})
	})

	onPipeline(func(e transcodeapi.JobQueued) []pipelineChange {
		if e.Kind != transcodeapi.JobKindTranscode {
			return nil
		}
		return change(rowTarget{File: e.MediaFile}, setTranscode(api.TranscodeChip{State: api.TranscodeQueued}))
	})
	onPipeline(func(e transcodeapi.JobStarted) []pipelineChange {
		if e.Kind != transcodeapi.JobKindTranscode {
			return nil
		}
		return change(rowTarget{File: e.MediaFile}, setTranscode(api.TranscodeChip{State: api.TranscodeRunning, Job: &e.Job}))
	})
	onPipeline(func(e transcodeapi.TranscodeSkipped) []pipelineChange {
		return change(rowTarget{File: e.MediaFile}, setTranscode(api.TranscodeChip{State: api.TranscodeSkipped, Reason: skipDetail(e)}))
	})
	onPipeline(func(e transcodeapi.JobFailed) []pipelineChange {
		switch {
		case e.Kind != transcodeapi.JobKindTranscode:
			return nil
		case !e.Final:
			return change(rowTarget{File: e.MediaFile}, setTranscode(api.TranscodeChip{State: api.TranscodeQueued}))
		}
		return change(rowTarget{File: e.MediaFile}, setTranscode(api.TranscodeChip{State: api.TranscodeFailed, Reason: e.Reason}))
	})
	onPipeline(func(e transcodeapi.JobCancelled) []pipelineChange {
		return change(rowTarget{File: e.MediaFile}, func(f *facets) { f.Transcode = nil })
	})
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
