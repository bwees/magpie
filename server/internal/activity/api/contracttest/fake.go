package contracttest

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
)

const (
	ReaderTimeline      scripted.Method = "activity.Reader.Timeline"
	ReaderHistory       scripted.Method = "activity.Reader.History"
	ReaderPipeline      scripted.Method = "activity.Reader.Pipeline"
	ReaderTitleStages   scripted.Method = "activity.Reader.TitleStages"
	ReaderNotifications scripted.Method = "activity.Reader.Notifications"
	ReaderUnreadCount   scripted.Method = "activity.Reader.UnreadCount"
	ReaderAttention     scripted.Method = "activity.Reader.Attention"
)

var severities = []string{api.SeverityInfo, api.SeverityWarn, api.SeverityError}

// Fake is the in-memory activity.Reader. It passes Run.
type Fake struct {
	scripted.Script

	mu    sync.RWMutex
	state Fixture
}

var _ api.Reader = (*Fake)(nil)

func NewFake(f Fixture) *Fake {
	fake := &Fake{}
	fake.Reset(f)
	return fake
}

// Reset replaces the whole stored state.
func (f *Fake) Reset(state Fixture) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = Fixture{
		Timeline:      slices.Clone(state.Timeline),
		History:       slices.Clone(state.History),
		Pipeline:      slices.Clone(state.Pipeline),
		Notifications: slices.Clone(state.Notifications),
		Attention:     slices.Clone(state.Attention),
	}
}

func (f *Fake) Timeline(ctx context.Context, title domain.TitleID, p api.Page) ([]api.TimelineEntry, error) {
	if err := f.Enter(ctx, ReaderTimeline); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	entries := []api.TimelineEntry{}
	for _, e := range f.state.Timeline {
		if e.Title == title {
			entries = append(entries, e.Entry)
		}
	}
	return page(entries, p, func(e api.TimelineEntry) time.Time { return e.At }), nil
}

func (f *Fake) History(ctx context.Context, minSeverity string, p api.Page) ([]api.TimelineEntry, error) {
	if err := f.Enter(ctx, ReaderHistory); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	least := max(slices.Index(severities, minSeverity), 0)
	entries := []api.TimelineEntry{}
	for _, e := range f.state.History {
		if slices.Index(severities, e.Severity) >= least {
			entries = append(entries, e)
		}
	}
	return page(entries, p, func(e api.TimelineEntry) time.Time { return e.At }), nil
}

func (f *Fake) Pipeline(ctx context.Context, title domain.TitleID) ([]api.PipelineRow, error) {
	if err := f.Enter(ctx, ReaderPipeline); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	rows := []api.PipelineRow{}
	for _, r := range f.orderedRows(title) {
		rows = append(rows, r.numbered())
	}
	return rows, nil
}

func (f *Fake) TitleStages(ctx context.Context, titles []domain.TitleID) (map[domain.TitleID]api.PipelineRow, error) {
	if err := f.Enter(ctx, ReaderTitleStages); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	stages := map[domain.TitleID]api.PipelineRow{}
	for _, title := range titles {
		rows := f.orderedRows(title)
		if len(rows) == 0 {
			continue
		}
		least := slices.MinFunc(rows, func(a, b PipelineRow) int {
			return cmp.Compare(slices.Index(api.Stages(), a.Row.Stage), slices.Index(api.Stages(), b.Row.Stage))
		})
		stages[title] = least.numbered()
	}
	return stages, nil
}

func (f *Fake) Notifications(ctx context.Context, user domain.UserID, unreadOnly bool, p api.Page) ([]api.Notification, error) {
	if err := f.Enter(ctx, ReaderNotifications); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	notifications := []api.Notification{}
	for _, n := range f.state.Notifications {
		if n.User == user && (!unreadOnly || !n.Notification.Read) {
			notifications = append(notifications, n.Notification)
		}
	}
	return page(notifications, p, func(n api.Notification) time.Time { return n.CreatedAt }), nil
}

func (f *Fake) UnreadCount(ctx context.Context, user domain.UserID) (int, error) {
	if err := f.Enter(ctx, ReaderUnreadCount); err != nil {
		return 0, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	unread := slices.DeleteFunc(slices.Clone(f.state.Notifications), func(n UserNotification) bool {
		return n.User != user || n.Notification.Read
	})
	return len(unread), nil
}

func (f *Fake) Attention(ctx context.Context) ([]api.AttentionItem, error) {
	if err := f.Enter(ctx, ReaderAttention); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	items := append([]api.AttentionItem{}, f.state.Attention...)
	slices.SortFunc(items, func(a, b api.AttentionItem) int { return a.OpenedAt.Compare(b.OpenedAt) })
	return items, nil
}

// orderedRows returns a title's rows in season/number order. The caller holds the lock.
func (f *Fake) orderedRows(title domain.TitleID) []PipelineRow {
	rows := []PipelineRow{}
	for _, r := range f.state.Pipeline {
		if r.Row.Title == title {
			rows = append(rows, r)
		}
	}
	slices.SortFunc(rows, func(a, b PipelineRow) int {
		return cmp.Or(cmp.Compare(a.Season, b.Season), cmp.Compare(a.Number, b.Number))
	})
	return rows
}

// page sorts newest first, drops entries not before p.Before and keeps at most p.EffectiveLimit().
func page[T any](items []T, p api.Page, at func(T) time.Time) []T {
	slices.SortFunc(items, func(a, b T) int { return at(b).Compare(at(a)) })
	if p.Before != nil {
		items = slices.DeleteFunc(items, func(item T) bool { return !at(item).Before(*p.Before) })
	}
	return items[:min(len(items), p.EffectiveLimit())]
}
