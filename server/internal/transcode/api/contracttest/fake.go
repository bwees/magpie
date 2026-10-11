package contracttest

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"

	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
	"magpie/internal/transcode/api"
)

const (
	ReaderJob              scripted.Method = "transcode.Reader.Job"
	ReaderJobsForMediaFile scripted.Method = "transcode.Reader.JobsForMediaFile"
	ReaderActiveJobs       scripted.Method = "transcode.Reader.ActiveJobs"
	ReaderQueueSummary     scripted.Method = "transcode.Reader.QueueSummary"
	ReaderAgents           scripted.Method = "transcode.Reader.Agents"
	ReaderIsExcluded       scripted.Method = "transcode.Reader.IsExcluded"
	ReaderTarget           scripted.Method = "transcode.Reader.Target"
)

// Fake is the in-memory transcode.Reader other contexts test against. It passes Run.
type Fake struct {
	scripted.Script

	mu             sync.RWMutex
	jobs           map[domain.JobID]api.Job
	agents         map[domain.AgentID]api.Agent
	excluded       map[domain.TitleID]bool
	target         api.Target
	estimatedSaved int64
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{
		jobs:     map[domain.JobID]api.Job{},
		agents:   map[domain.AgentID]api.Agent{},
		excluded: map[domain.TitleID]bool{},
		target:   f.Target,
	}
	for _, j := range f.Jobs {
		fake.PutJob(j)
	}
	for _, a := range f.Agents {
		fake.PutAgent(a)
	}
	for _, title := range f.Excluded {
		fake.SetExcluded(title, true)
	}
	return fake
}

// PutJob adds or replaces a job.
func (f *Fake) PutJob(j api.Job) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs[j.ID] = j
}

// PutAgent adds or replaces an agent.
func (f *Fake) PutAgent(a api.Agent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agents[a.ID] = a
}

func (f *Fake) SetExcluded(title domain.TitleID, excluded bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.excluded[title] = excluded
}

func (f *Fake) SetTarget(t api.Target) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.target = t
}

// SetEstimatedSavedBytes sets what QueueSummary reports as the estimated saving.
func (f *Fake) SetEstimatedSavedBytes(n int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.estimatedSaved = n
}

func (f *Fake) Job(ctx context.Context, id domain.JobID) (api.Job, error) {
	if err := f.Enter(ctx, ReaderJob); err != nil {
		return api.Job{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	j, ok := f.jobs[id]
	if !ok {
		return api.Job{}, api.ErrNotFound
	}
	return j, nil
}

func (f *Fake) JobsForMediaFile(ctx context.Context, id domain.MediaFileID) ([]api.Job, error) {
	if err := f.Enter(ctx, ReaderJobsForMediaFile); err != nil {
		return nil, err
	}
	jobs := f.filter(func(j api.Job) bool { return j.MediaFile == id })
	slices.SortFunc(jobs, func(a, b api.Job) int { return b.QueuedAt.Compare(a.QueuedAt) })
	return jobs, nil
}

func (f *Fake) ActiveJobs(ctx context.Context) ([]api.Job, error) {
	if err := f.Enter(ctx, ReaderActiveJobs); err != nil {
		return nil, err
	}
	jobs := f.filter(func(j api.Job) bool { return j.State.Active() })
	slices.SortFunc(jobs, func(a, b api.Job) int {
		return cmp.Or(cmp.Compare(b.Priority, a.Priority), a.QueuedAt.Compare(b.QueuedAt))
	})
	return jobs, nil
}

func (f *Fake) QueueSummary(ctx context.Context) (api.QueueSummary, error) {
	if err := f.Enter(ctx, ReaderQueueSummary); err != nil {
		return api.QueueSummary{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	summary := api.QueueSummary{Counts: map[api.JobState]int{}, EstimatedSavedBytes: f.estimatedSaved}
	for j := range maps.Values(f.jobs) {
		summary.Counts[j.State]++
	}
	return summary, nil
}

func (f *Fake) Agents(ctx context.Context) ([]api.Agent, error) {
	if err := f.Enter(ctx, ReaderAgents); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	agents := slices.Collect(maps.Values(f.agents))
	slices.SortFunc(agents, func(a, b api.Agent) int { return cmp.Compare(a.Hostname, b.Hostname) })
	return agents, nil
}

func (f *Fake) IsExcluded(ctx context.Context, title domain.TitleID) (bool, error) {
	if err := f.Enter(ctx, ReaderIsExcluded); err != nil {
		return false, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.excluded[title], nil
}

func (f *Fake) Target(ctx context.Context) (api.Target, error) {
	if err := f.Enter(ctx, ReaderTarget); err != nil {
		return api.Target{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.target, nil
}

func (f *Fake) filter(keep func(api.Job) bool) []api.Job {
	f.mu.RLock()
	defer f.mu.RUnlock()
	jobs := []api.Job{}
	for j := range maps.Values(f.jobs) {
		if keep(j) {
			jobs = append(jobs, j)
		}
	}
	return jobs
}
