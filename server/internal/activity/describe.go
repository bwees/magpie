package activity

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	acquisitionapi "magpie/internal/acquisition/api"
	"magpie/internal/activity/api"
	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	identityapi "magpie/internal/identity/api"
	libraryapi "magpie/internal/library/api"
	platformapi "magpie/internal/platform/api"
	"magpie/internal/platform/bus"
	requestsapi "magpie/internal/requests/api"
	subtitlesapi "magpie/internal/subtitles/api"
	transcodeapi "magpie/internal/transcode/api"
)

// subject is what an event is about. Events that carry only a media file, download or search ID
// are resolved to their title through activity_refs.
type subject struct {
	Title     *domain.TitleID
	Episodes  []domain.EpisodeID
	MediaFile *domain.MediaFileID
	Download  *domain.DownloadID
	Search    *domain.SearchID
}

// description is how one event reads on a timeline and in the history.
type description struct {
	subject
	Severity string
	Text     string
	Detail   string
}

const (
	languageSeparator = ", "
	missingSeparator  = " · "
	wordSeparator     = " "
	enumSeparator     = "_"
)

var describers = map[string]func(bus.Event) (description, error){}

// on registers how events of one payload type are described.
func on[T domain.Message](fn func(T) description) {
	var zero T
	describers[zero.Type()] = func(ev bus.Event) (description, error) {
		payload, err := bus.Decode[T](ev)
		if err != nil {
			return description{}, err
		}
		return fn(payload), nil
	}
}

// describe renders ev. An event it can't render keeps its raw type as text.
func describe(ev bus.Event) description {
	fn, ok := describers[ev.Type]
	if !ok {
		return description{Severity: api.SeverityInfo, Text: ev.Type}
	}
	d, err := fn(ev)
	if err != nil {
		slog.Warn("activity: rendering event", "type", ev.Type, "event", ev.ID, "error", err)
		return description{Severity: api.SeverityInfo, Text: ev.Type}
	}
	if d.Severity == "" {
		d.Severity = api.SeverityInfo
	}
	return d
}

func titled(title domain.TitleID, episodes ...domain.EpisodeID) subject {
	return subject{Title: &title, Episodes: episodes}
}

func ofFile(file domain.MediaFileID) subject {
	return subject{MediaFile: &file}
}

func episodeIDs(refs []domain.EpisodeRef) []domain.EpisodeID {
	ids := make([]domain.EpisodeID, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.Episode)
	}
	return ids
}

// humanize turns an UPPER_SNAKE enum value into lower-case words.
func humanize[T ~string](value T) string {
	return strings.ToLower(strings.ReplaceAll(string(value), enumSeparator, wordSeparator))
}

func capitalize(s string) string {
	first, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(first)) + s[size:]
}

func languages(langs []domain.Language) string {
	words := make([]string, 0, len(langs))
	for _, lang := range langs {
		words = append(words, string(lang))
	}
	return strings.Join(words, languageSeparator)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

var originText = map[domain.Origin]string{
	domain.OriginRequest:     "Added from a request",
	domain.OriginManual:      "Added",
	domain.OriginLibraryScan: "Found in a library scan",
	domain.OriginMigration:   "Imported from Sonarr/Radarr",
}

var triggerText = map[acquisitionapi.SearchTrigger]string{
	acquisitionapi.TriggerRSS:         "RSS",
	acquisitionapi.TriggerAuto:        "Automatic",
	acquisitionapi.TriggerUserAuto:    "Search button",
	acquisitionapi.TriggerInteractive: "Interactive",
	acquisitionapi.TriggerRegrab:      "Re-grab",
}

var issueActionText = map[string]string{
	requestsapi.IssueActionRegrab:         "Blocklist & re-grab requested",
	requestsapi.IssueActionRerunSubtitles: "Subtitle re-run requested",
}

var serviceText = map[platformapi.Service]string{
	platformapi.ServiceTMDB:          "TMDB",
	platformapi.ServiceQBittorrent:   "qBittorrent",
	platformapi.ServiceProwlarr:      "Prowlarr",
	platformapi.ServiceJellyfin:      "Jellyfin",
	platformapi.ServiceOpenSubtitles: "OpenSubtitles",
}

func serviceName(s platformapi.Service) string {
	if name, ok := serviceText[s]; ok {
		return name
	}
	return string(s)
}

var jobKindText = map[transcodeapi.JobKind]string{
	transcodeapi.JobKindTranscode: "Transcode",
	transcodeapi.JobKindWhisper:   "Whisper",
}

func init() {
	on(func(e identityapi.UserCreated) description {
		return description{Text: "User created", Detail: humanize(e.Source)}
	})
	on(func(e platformapi.ServiceHealthChanged) description {
		if e.Healthy {
			return description{Text: serviceName(e.Service) + " reachable again"}
		}
		return description{Severity: api.SeverityError, Text: serviceName(e.Service) + " unreachable", Detail: e.Error}
	})

	on(func(e catalogapi.TitleAdded) description {
		return description{subject: titled(e.Title, e.Monitored...), Text: originText[e.Origin]}
	})
	on(func(e catalogapi.TitleAddFailed) description {
		return description{Severity: api.SeverityError, Text: fmt.Sprintf("Couldn't add TMDB %d", e.Tmdb), Detail: humanize(e.Reason)}
	})
	on(func(e catalogapi.MonitoringChanged) description {
		d := description{subject: titled(e.Title, append(e.Added, e.Removed...)...)}
		switch {
		case e.Movie != nil && *e.Movie:
			d.Text = "Monitored"
		case e.Movie != nil:
			d.Text = "Unmonitored"
		case len(e.Added) > 0:
			d.Text = "Monitoring " + plural(len(e.Added), "more episode", "more episodes")
		default:
			d.Text = "Stopped monitoring " + plural(len(e.Removed), "episode", "episodes")
		}
		return d
	})
	on(func(e catalogapi.ProfileChanged) description {
		return description{subject: titled(e.Title), Text: "Quality profile changed"}
	})
	on(func(e catalogapi.ScanFolderMatched) description {
		return description{subject: titled(e.Title), Text: "Matched a library folder", Detail: e.Folder}
	})
	on(func(e catalogapi.LanguagesChanged) description {
		if e.Title == nil {
			return description{Text: "Wanted languages changed"}
		}
		langs := make([]domain.Language, 0, len(e.Languages))
		for _, l := range e.Languages {
			langs = append(langs, l.Language)
		}
		return description{subject: titled(*e.Title), Text: "Wanted languages changed", Detail: languages(langs)}
	})
	on(func(e catalogapi.EpisodesAnnounced) description {
		ids := make([]domain.EpisodeID, 0, len(e.Episodes))
		for _, ref := range e.Episodes {
			ids = append(ids, ref.Episode)
		}
		return description{subject: titled(e.Title, ids...), Text: plural(len(ids), "new episode announced", "new episodes announced")}
	})
	on(func(e catalogapi.WantedReleased) description {
		if len(e.Episodes) == 0 {
			return description{subject: titled(e.Title), Text: "Released"}
		}
		return description{subject: titled(e.Title, e.Episodes...), Text: plural(len(e.Episodes), "episode released", "episodes released")}
	})
	on(func(e catalogapi.EpisodeGroupChanged) description {
		return description{subject: titled(e.Title), Text: "Episode order changed"}
	})
	on(func(e catalogapi.TitleRemoved) description {
		return description{subject: titled(e.Title), Text: "Removed"}
	})

	on(func(e requestsapi.RequestCreated) description {
		return description{Text: fmt.Sprintf("New %s request for TMDB %d", humanize(e.Kind), e.Tmdb)}
	})
	on(func(e requestsapi.RequestApproved) description {
		return description{Text: fmt.Sprintf("Request approved for TMDB %d", e.Tmdb)}
	})
	on(func(e requestsapi.RequestDeclined) description {
		return description{Text: "Request declined", Detail: e.Reason}
	})
	on(func(requestsapi.RequestCancelled) description {
		return description{Text: "Request cancelled"}
	})
	on(func(e requestsapi.RequestAvailable) description {
		return description{subject: titled(e.Title), Text: "Request available"}
	})
	on(func(e requestsapi.IssueReported) description {
		d := description{subject: titled(e.Title), Text: capitalize(humanize(e.Kind) + " issue reported")}
		if e.Episode != nil {
			d.Episodes = []domain.EpisodeID{*e.Episode}
		}
		return d
	})
	on(func(requestsapi.IssueCommented) description {
		return description{Text: "Issue comment"}
	})
	on(func(e requestsapi.IssueActionRequested) description {
		d := description{subject: titled(e.Title), Text: issueActionText[e.Action]}
		if e.Episode != nil {
			d.Episodes = []domain.EpisodeID{*e.Episode}
		}
		return d
	})
	on(func(requestsapi.IssueResolved) description {
		return description{Text: "Issue resolved"}
	})

	on(func(e acquisitionapi.SearchStarted) description {
		d := description{subject: titled(e.Title, e.Episodes...), Text: "Search started", Detail: triggerText[e.Trigger]}
		d.Search = &e.Search
		return d
	})
	on(func(e acquisitionapi.SearchCompleted) description {
		d := description{subject: titled(e.Title), Detail: fmt.Sprintf("%d of %d releases accepted", e.Accepted, e.Total)}
		d.Search = &e.Search
		switch e.Outcome {
		case acquisitionapi.OutcomeNothingAccepted:
			d.Severity = api.SeverityWarn
			d.Text = detailNoAcceptedRelease
		case acquisitionapi.OutcomePendingDelay:
			d.Text = detailWaitingForBetterRelease
		default:
			d.Text = "Search finished"
		}
		return d
	})
	on(func(e acquisitionapi.ReleaseGrabbed) description {
		d := description{subject: titled(e.Title, e.Episodes...), Text: "Grabbed from " + e.Indexer, Detail: e.Release}
		if e.Quality.Name != "" {
			d.Text = "Grabbed " + e.Quality.Name + " from " + e.Indexer
		}
		d.Download = &e.Download
		return d
	})
	on(func(e acquisitionapi.ReleaseRejected) description {
		detail := e.Release
		if e.BlockedFile != nil {
			detail += "\n" + *e.BlockedFile
		}
		return description{subject: titled(e.Title), Severity: api.SeverityWarn, Text: rejectedNote(e.Reason), Detail: detail}
	})
	on(func(e acquisitionapi.DownloadCompleted) description {
		d := description{subject: titled(e.Title, e.Episodes...), Text: "Download finished", Detail: e.ReleaseName}
		d.Download = &e.Download
		return d
	})
	on(func(e acquisitionapi.DownloadFailed) description {
		d := description{subject: titled(e.Title), Severity: api.SeverityError, Text: "Download failed", Detail: e.Reason}
		d.Download = &e.Download
		return d
	})
	on(func(e acquisitionapi.CutoffMet) description {
		return description{subject: titled(e.Title, e.Episodes...), Text: "Cutoff met", Detail: e.Quality.Name}
	})

	on(func(e libraryapi.FileImported) description {
		d := description{subject: titled(e.Title, episodeIDs(e.Episodes)...), Text: "Imported", Detail: e.ReleaseName}
		if e.Replaces != nil {
			d.Text = "Upgraded"
		}
		if e.Quality.Name != "" {
			d.Text += " " + e.Quality.Name
		}
		if d.Detail == "" {
			d.Detail = e.Path
		}
		d.MediaFile = &e.MediaFile
		return d
	})
	on(func(e libraryapi.ImportFailed) description {
		return description{subject: subject{Download: &e.Download}, Severity: api.SeverityError, Text: "Import failed", Detail: e.Reason}
	})
	on(func(e libraryapi.ImportRemoved) description {
		return description{subject: titled(e.Title), Text: "Import removed and blocklisted", Detail: e.ReleaseName}
	})
	on(func(e libraryapi.FileTranscoded) description {
		d := description{subject: ofFile(e.MediaFile), Text: "Transcoded"}
		if saved := savedPercentOf(e.OldSize, e.NewSize); saved != nil {
			d.Text = fmt.Sprintf("Transcoded, saved %d%%", *saved)
		}
		return d
	})
	on(func(e libraryapi.TranscodeSwapRejected) description {
		return description{subject: ofFile(e.MediaFile), Severity: api.SeverityWarn, Text: "Transcode discarded", Detail: humanize(e.Reason)}
	})
	on(func(e libraryapi.FileDeleted) description {
		d := description{subject: titled(e.Title, episodeIDs(e.Episodes)...), Text: "File deleted", Detail: e.Path}
		if e.Reason == libraryapi.DeleteUpgraded {
			d.Text = "Old file replaced"
		}
		return d
	})
	on(func(e libraryapi.FileRenamed) description {
		return description{subject: titled(e.Title, episodeIDs(e.Episodes)...), Text: "Renamed to " + filepath.Base(e.NewPath), Detail: e.OldPath}
	})
	on(func(e libraryapi.ItemAvailable) description {
		return description{subject: titled(e.Title, e.Episodes...), Text: "Available in Jellyfin"}
	})
	on(func(e libraryapi.FolderFound) description {
		return description{Text: "Found a library folder", Detail: e.Path}
	})

	on(func(e subtitlesapi.SubtitleAdded) description {
		return description{subject: ofFile(e.MediaFile), Text: "Subtitle added: " + string(e.Language), Detail: humanize(e.Source)}
	})
	on(func(e subtitlesapi.SubtitleMissing) description {
		return description{subject: ofFile(e.MediaFile), Text: "No subtitle found: " + string(e.Language), Detail: humanize(e.Reason)}
	})
	on(func(e subtitlesapi.CoverageSettled) description {
		d := description{subject: titled(e.Title), Text: "Subtitles settled", Detail: subsSummary(e.Satisfied, e.Missing)}
		d.MediaFile = &e.MediaFile
		return d
	})

	on(func(e transcodeapi.JobQueued) description {
		d := description{subject: titled(e.Title), Text: jobKindText[e.Kind] + " queued"}
		d.MediaFile = &e.MediaFile
		return d
	})
	on(func(e transcodeapi.JobStarted) description {
		return description{subject: ofFile(e.MediaFile), Text: jobKindText[e.Kind] + " started", Detail: fmt.Sprintf("Attempt %d", e.Attempt)}
	})
	on(func(e transcodeapi.TranscodeVerified) description {
		return description{subject: ofFile(e.MediaFile), Text: "Transcode verified"}
	})
	on(func(e transcodeapi.TranscodeSkipped) description {
		return description{subject: ofFile(e.MediaFile), Text: "Original kept", Detail: skipDetail(e)}
	})
	on(func(e transcodeapi.JobFailed) description {
		d := description{subject: ofFile(e.MediaFile), Severity: api.SeverityWarn, Text: jobKindText[e.Kind] + " failed, retrying", Detail: e.Reason}
		if e.Final {
			d.Severity = api.SeverityError
			d.Text = jobKindText[e.Kind] + " failed"
		}
		return d
	})
	on(func(e transcodeapi.WhisperCompleted) description {
		return description{subject: ofFile(e.MediaFile), Text: "Whisper subtitle ready: " + string(e.Language)}
	})
	on(func(e transcodeapi.AgentOnline) description {
		return description{Text: "Agent online", Detail: e.Hostname}
	})
	on(func(transcodeapi.AgentOffline) description {
		return description{Severity: api.SeverityWarn, Text: "Agent offline"}
	})
}

func rejectedNote(reason acquisitionapi.RejectionReason) string {
	return "Rejected: " + humanize(reason)
}

func skipDetail(e transcodeapi.TranscodeSkipped) string {
	if e.Detail != "" {
		return e.Detail
	}
	return humanize(e.Reason)
}

func subsSummary(satisfied, missing []domain.Language) string {
	summary := languages(satisfied)
	if len(missing) > 0 {
		if summary != "" {
			summary += missingSeparator
		}
		summary += languages(missing) + " missing"
	}
	return summary
}
