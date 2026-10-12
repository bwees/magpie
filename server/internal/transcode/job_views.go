package transcode

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/transcode/api"
)

// JobView is a job with what the GPU jobs list shows about it.
type JobView struct {
	ID          domain.JobID
	Kind        api.JobKind
	State       api.JobState
	MediaFile   domain.MediaFileID
	Title       domain.TitleID
	TitleName   string
	Episodes    string // S01E02, S01E02-E03; empty for a movie
	FileName    string
	Origin      domain.Origin
	TargetCodec api.TargetCodec
	Language    domain.Language // WHISPER jobs
	Agent       *domain.AgentID
	AgentName   string
	Attempt     int
	Progress    float64
	// Prefetched is a job an agent fetches while its slot is busy; it encodes once the slot frees.
	Prefetched bool
	Error      string
	QueuedAt   time.Time
	NotBefore  *time.Time
	FinishedAt *time.Time
}

// JobList is a page of jobs with the queue's totals.
type JobList struct {
	Jobs   []JobView
	Counts map[api.JobState]int
	// Backlog is the queued jobs of files found by a library scan or a migration.
	Backlog int
}

const defaultJobListLimit = 200

// Jobs lists jobs for the GPU jobs tab: those agents hold first, then the queue, then finished ones.
func (s *JobService) Jobs(ctx context.Context, filter JobFilter) (JobList, error) {
	if filter.Limit <= 0 {
		filter.Limit = defaultJobListLimit
	}
	rows, err := s.deps.Jobs.list(ctx, filter)
	if err != nil {
		return JobList{}, err
	}
	counts, err := s.deps.Jobs.counts(ctx)
	if err != nil {
		return JobList{}, err
	}
	var backlog int64
	err = s.deps.DB.Read(ctx).Model(&jobRow{}).Where("state = ? AND kind = ? AND origin <> ?", api.StateQueued, api.JobKindTranscode, domain.OriginDownload).Count(&backlog).Error
	if err != nil {
		return JobList{}, err
	}
	agents, err := s.deps.Agents.agents(s.deps.DB.Read(ctx), nil)
	if err != nil {
		return JobList{}, err
	}
	hostnames := map[domain.AgentID]string{}
	for _, a := range agents {
		hostnames[a.ID] = a.Hostname
	}
	names := map[domain.TitleID]string{}
	views := make([]JobView, 0, len(rows))
	for _, row := range rows {
		view, err := s.view(ctx, row, names, hostnames)
		if err != nil {
			return JobList{}, err
		}
		views = append(views, view)
	}
	return JobList{Jobs: views, Counts: counts, Backlog: int(backlog)}, nil
}

// Job returns one job as the list shows it.
func (s *JobService) Job(ctx context.Context, id domain.JobID) (JobView, error) {
	row, err := jobIn(s.deps.DB.Read(ctx), id)
	if err != nil {
		return JobView{}, err
	}
	hostnames := map[domain.AgentID]string{}
	if agent := displayedAgent(row); agent != nil {
		if a, err := s.deps.Agents.row(s.deps.DB.Read(ctx), *agent); err == nil {
			hostnames[*agent] = a.Hostname
		}
	}
	return s.view(ctx, row, map[domain.TitleID]string{}, hostnames)
}

// displayedAgent is the agent running the job, or the one that ran its last attempt.
func displayedAgent(row jobRow) *domain.AgentID {
	if row.AgentID != nil {
		return row.AgentID
	}
	return row.LastAgentID
}

func (s *JobService) view(ctx context.Context, row jobRow, names map[domain.TitleID]string, hostnames map[domain.AgentID]string) (JobView, error) {
	view := JobView{
		ID: row.ID, Kind: row.Kind, State: row.State, MediaFile: row.MediaFileID, Title: row.TitleID, Origin: row.Origin,
		TargetCodec: row.TargetCodec, Language: row.Language, Agent: displayedAgent(row), Attempt: row.Attempt, Progress: row.Progress, Prefetched: row.Prefetch,
		Error: row.Error, QueuedAt: row.QueuedAt, NotBefore: row.NotBefore, FinishedAt: row.FinishedAt,
	}
	if view.Agent != nil {
		view.AgentName = hostnames[*view.Agent]
	}
	name, ok := names[row.TitleID]
	if !ok {
		title, err := s.deps.Catalog.Title(ctx, row.TitleID)
		if err != nil && !errors.Is(err, catalogapi.ErrNotFound) {
			return JobView{}, err
		}
		name = title.Name
		names[row.TitleID] = name
	}
	view.TitleName = name
	file, err := s.deps.Library.File(ctx, row.MediaFileID)
	if errors.Is(err, libraryapi.ErrNotFound) {
		return view, nil
	}
	if err != nil {
		return JobView{}, err
	}
	view.FileName = filepath.Base(file.Path)
	view.Episodes = episodeLabel(file.Episodes)
	return view, nil
}

// episodeLabel is S01E02 for one episode and S01E02-E03 for a multi-episode file.
func episodeLabel(episodes []domain.EpisodeRef) string {
	if len(episodes) == 0 {
		return ""
	}
	first := episodes[0]
	label := fmt.Sprintf("S%02dE%02d", first.Season, first.Number)
	if len(episodes) > 1 {
		last := episodes[len(episodes)-1]
		label += fmt.Sprintf("-E%02d", last.Number)
	}
	return label
}
