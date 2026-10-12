package subtitles

import (
	"slices"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	"magpie/internal/subtitles/api"
)

// WantedStatus is one wanted track's progress for a file.
type WantedStatus string

const (
	StatusPending        WantedStatus = "PENDING"
	StatusSearching      WantedStatus = "SEARCHING"
	StatusWaitingWhisper WantedStatus = "WAITING_WHISPER"
	StatusMissing        WantedStatus = "MISSING"
	StatusSatisfied      WantedStatus = "SATISFIED"
)

// InProgress reports whether the track still has a search or a Whisper job ahead of it.
func (s WantedStatus) InProgress() bool {
	return s == StatusPending || s == StatusSearching || s == StatusWaitingWhisper
}

// Coverage maps a status to what the title page shows.
func (s WantedStatus) Coverage() api.CoverageStatus {
	switch s {
	case StatusSatisfied:
		return api.CoverageSatisfied
	case StatusMissing:
		return api.CoverageMissing
	default:
		return api.CoveragePending
	}
}

// Priority orders downloads when the daily quota is short.
type Priority string

const (
	PriorityImport  Priority = "IMPORT"
	PriorityManual  Priority = "MANUAL"
	PriorityUpgrade Priority = "UPGRADE"
	PriorityBacklog Priority = "BACKLOG"
)

// priorityOrder is the quota order: new imports, then manual requests, then upgrades, then the backlog.
var priorityOrder = []Priority{PriorityImport, PriorityManual, PriorityUpgrade, PriorityBacklog}

func (p Priority) rank() int { return slices.Index(priorityOrder, p) }

// sourcePrecedence decides which subtitle satisfies a track when several do.
var sourcePrecedence = []api.Source{api.SourceEmbedded, api.SourceRelease, api.SourceProvider, api.SourceWhisper}

// tracks expands a wanted-language list into wanted tracks: each language, plus its forced track when wanted.
func tracks(list []catalogapi.WantedLanguage) []wantedTrack {
	out := make([]wantedTrack, 0, len(list))
	for _, lang := range list {
		out = append(out, wantedTrack{Language: lang.Language, HearingImpairedOK: lang.HearingImpaired})
		if lang.Forced {
			out = append(out, wantedTrack{Language: lang.Language, Forced: true, HearingImpairedOK: lang.HearingImpaired})
		}
	}
	return out
}

// covering returns the subtitle that satisfies a track, by source precedence, or nil.
func covering(subs []subtitleRow, language domain.Language, forced bool) *subtitleRow {
	var best *subtitleRow
	for i := range subs {
		sub := &subs[i]
		if sub.Language != language || sub.Forced != forced {
			continue
		}
		if best == nil || slices.Index(sourcePrecedence, sub.Source) < slices.Index(sourcePrecedence, best.Source) {
			best = sub
		}
	}
	return best
}

// reconcile brings a file's wanted rows in line with the wanted list and the subtitles it has. A track that
// lost its subtitle goes back to PENDING with the given priority. It returns the rows to save and the rows
// of tracks no longer wanted.
func reconcile(file domain.MediaFileID, existing []wantedRow, list []catalogapi.WantedLanguage, subs []subtitleRow, priority Priority, now time.Time) (keep, drop []wantedRow) {
	wanted := tracks(list)
	for position, track := range wanted {
		i := slices.IndexFunc(existing, func(r wantedRow) bool { return r.Language == track.Language && r.Forced == track.Forced })
		row := wantedRow{
			MediaFileID: file, Language: track.Language, Forced: track.Forced,
			Status: StatusPending, Priority: priority, CreatedAt: now,
		}
		if i >= 0 {
			row = existing[i]
		}
		row.Position = position
		row.HearingImpaired = track.HearingImpairedOK
		if sub := covering(subs, track.Language, track.Forced); sub != nil {
			row.satisfy(sub.Source)
		} else if row.Status == StatusSatisfied || i < 0 {
			row.reopen(priority)
		}
		keep = append(keep, row)
	}
	for _, row := range existing {
		if !slices.ContainsFunc(wanted, func(t wantedTrack) bool { return t.Language == row.Language && t.Forced == row.Forced }) {
			drop = append(drop, row)
		}
	}
	return keep, drop
}

func (r *wantedRow) satisfy(source api.Source) {
	r.Status, r.Source, r.Reason, r.NextAttemptAt, r.Candidate = StatusSatisfied, &source, "", nil, nil
}

// reopen queues the track for a search now.
func (r *wantedRow) reopen(priority Priority) {
	r.Status, r.Source, r.Reason, r.NextAttemptAt, r.Candidate, r.Priority = StatusPending, nil, "", nil, nil, priority
	r.WhisperAttempts = 0
}

func (r wantedRow) track() wantedTrack {
	return wantedTrack{Language: r.Language, Forced: r.Forced, HearingImpairedOK: r.HearingImpaired}
}

// settledLanguages splits a file's wanted languages into satisfied and missing, in wanted order. A language
// is missing when any of its tracks is.
func settledLanguages(rows []wantedRow) (satisfied, missing []domain.Language) {
	satisfied, missing = []domain.Language{}, []domain.Language{}
	ordered := slices.Clone(rows)
	slices.SortStableFunc(ordered, func(a, b wantedRow) int { return a.Position - b.Position })
	for _, row := range ordered {
		if row.Status == StatusMissing && !slices.Contains(missing, row.Language) {
			missing = append(missing, row.Language)
		}
	}
	for _, row := range ordered {
		if row.Status == StatusSatisfied && !slices.Contains(missing, row.Language) && !slices.Contains(satisfied, row.Language) {
			satisfied = append(satisfied, row.Language)
		}
	}
	return satisfied, missing
}
