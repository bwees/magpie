// Package contracttest holds the activity.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"slices"
	"testing"
	"time"

	"magpie/internal/activity/api"
	"magpie/internal/domain"
)

// TitleEntry is one timeline row of a title.
type TitleEntry struct {
	Title domain.TitleID
	Entry api.TimelineEntry
}

// PipelineRow is one stored pipeline row with the episode numbering it is ordered by (0, 0 for movies).
type PipelineRow struct {
	Row            api.PipelineRow
	Season, Number int
}

// numbered is the row as a Reader returns it, with its episode numbering.
func (r PipelineRow) numbered() api.PipelineRow {
	row := r.Row
	row.Season, row.Number = r.Season, r.Number
	return row
}

// UserNotification is one in-app notification of a user.
type UserNotification struct {
	User         domain.UserID
	Notification api.Notification
}

// Fixture is the stored state a Reader starts with. Attention holds open items only.
type Fixture struct {
	Timeline      []TitleEntry
	History       []api.TimelineEntry
	Pipeline      []PipelineRow
	Notifications []UserNotification
	Attention     []api.AttentionItem
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

const (
	// overfull is more timeline entries than the largest page.
	overfull    = api.MaxPageLimit + 1
	smallPage   = 10
	hugePage    = 500
	unreadCount = 2

	fixtureEventType = "catalog.TitleAdded"
	fixtureText      = "fixture"
)

// Run checks a Reader against the activity contract (seed 21 and each method's doc comment).
func Run(t *testing.T, newReader Factory) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	busy := domain.NewID[domain.TitleID]()
	movie := domain.NewID[domain.TitleID]()
	series := domain.NewID[domain.TitleID]()

	var timeline []TitleEntry
	for i := range overfull {
		timeline = append(timeline, TitleEntry{Title: busy, Entry: entry(api.SeverityInfo, now.Add(-time.Duration(i)*time.Minute))})
	}
	movieOld := entry(api.SeverityInfo, now.Add(-time.Hour))
	movieNew := entry(api.SeverityWarn, now)
	timeline = append(timeline, TitleEntry{Title: movie, Entry: movieOld}, TitleEntry{Title: movie, Entry: movieNew})

	historyInfo := entry(api.SeverityInfo, now.Add(-3*time.Minute))
	historyError := entry(api.SeverityError, now.Add(-2*time.Minute))
	historyWarn := entry(api.SeverityWarn, now.Add(-time.Minute))

	s2e1 := pipelineRow(series, api.StageAvailable, 2, 1)
	s1e2 := pipelineRow(series, api.StageSearching, 1, 2)
	s1e1 := pipelineRow(series, api.StageAvailable, 1, 1)
	s1e3 := pipelineRow(series, api.StageSearching, 1, 3)
	s1e3.Row.Failed = true
	movieRow := PipelineRow{Row: api.PipelineRow{Title: movie, Stage: api.StageDownloading, Updated: now}}

	alice := domain.NewID[domain.UserID]()
	readOld := notification(now.Add(-time.Hour), true)
	unreadMid := notification(now.Add(-time.Minute), false)
	unreadNew := notification(now, false)
	someoneElses := notification(now, false)

	attentionNew := attention(now)
	attentionOld := attention(now.Add(-time.Hour))

	fixture := Fixture{
		Timeline: timeline,
		History:  []api.TimelineEntry{historyInfo, historyError, historyWarn},
		Pipeline: []PipelineRow{s2e1, s1e2, s1e1, s1e3, movieRow},
		Notifications: []UserNotification{
			{User: alice, Notification: readOld},
			{User: alice, Notification: unreadNew},
			{User: alice, Notification: unreadMid},
			{User: domain.NewID[domain.UserID](), Notification: someoneElses},
		},
		Attention: []api.AttentionItem{attentionNew, attentionOld},
	}

	t.Run("Timeline is newest first", func(t *testing.T) {
		got, err := newReader(t, fixture).Timeline(ctx, movie, api.Page{})
		if err != nil {
			t.Fatal(err)
		}
		assertEntries(t, got, movieNew, movieOld)
	})

	t.Run("Timeline of an unknown title is an empty slice", func(t *testing.T) {
		got, err := newReader(t, fixture).Timeline(ctx, domain.NewID[domain.TitleID](), api.Page{})
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("Timeline = %v, %v; want empty slice", got, err)
		}
	})

	t.Run("Page.Limit defaults to 50 and is capped at 200", func(t *testing.T) {
		r := newReader(t, fixture)
		for _, tc := range []struct{ limit, want int }{{0, api.DefaultPageLimit}, {smallPage, smallPage}, {hugePage, api.MaxPageLimit}} {
			got, err := r.Timeline(ctx, busy, api.Page{Limit: tc.limit})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("Limit %d returned %d entries, want %d", tc.limit, len(got), tc.want)
			}
		}
	})

	t.Run("Page.Before returns only older entries", func(t *testing.T) {
		before := movieNew.At
		got, err := newReader(t, fixture).Timeline(ctx, movie, api.Page{Before: &before})
		if err != nil {
			t.Fatal(err)
		}
		assertEntries(t, got, movieOld)
	})

	t.Run("History is newest first and filtered by minimum severity", func(t *testing.T) {
		r := newReader(t, fixture)
		all, err := r.History(ctx, "", api.Page{})
		if err != nil {
			t.Fatal(err)
		}
		assertEntries(t, all, historyWarn, historyError, historyInfo)
		warned, err := r.History(ctx, api.SeverityWarn, api.Page{})
		if err != nil {
			t.Fatal(err)
		}
		assertEntries(t, warned, historyWarn, historyError)
	})

	t.Run("Pipeline orders episodes by season, then number", func(t *testing.T) {
		got, err := newReader(t, fixture).Pipeline(ctx, series)
		if err != nil {
			t.Fatal(err)
		}
		assertRows(t, got, s1e1, s1e2, s1e3, s2e1)
	})

	t.Run("Pipeline of an unknown title is an empty slice", func(t *testing.T) {
		got, err := newReader(t, fixture).Pipeline(ctx, domain.NewID[domain.TitleID]())
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("Pipeline = %v, %v; want empty slice", got, err)
		}
	})

	t.Run("TitleStages returns the least-advanced row per known title", func(t *testing.T) {
		unknown := domain.NewID[domain.TitleID]()
		got, err := newReader(t, fixture).TitleStages(ctx, []domain.TitleID{series, movie, unknown})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("TitleStages has %d titles, want 2", len(got))
		}
		if row := got[series]; row.Stage != api.StageSearching || !sameEpisode(row.Episode, s1e2.Row.Episode) {
			t.Errorf("series stage row = %+v, want S01E02 at SEARCHING", row)
		}
		if row := got[movie]; row.Stage != api.StageDownloading {
			t.Errorf("movie stage = %s, want DOWNLOADING", row.Stage)
		}
	})

	t.Run("Notifications are a user's own, newest first, optionally unread only", func(t *testing.T) {
		r := newReader(t, fixture)
		all, err := r.Notifications(ctx, alice, false, api.Page{})
		if err != nil {
			t.Fatal(err)
		}
		assertNotifications(t, all, unreadNew, unreadMid, readOld)
		unread, err := r.Notifications(ctx, alice, true, api.Page{})
		if err != nil {
			t.Fatal(err)
		}
		assertNotifications(t, unread, unreadNew, unreadMid)
		if n, err := r.UnreadCount(ctx, alice); err != nil || n != unreadCount {
			t.Errorf("UnreadCount = %d, %v; want %d", n, err, unreadCount)
		}
	})

	t.Run("Attention lists open items oldest first", func(t *testing.T) {
		got, err := newReader(t, fixture).Attention(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != attentionOld.ID || got[1].ID != attentionNew.ID {
			t.Errorf("Attention = %+v, want oldest first", got)
		}
	})
}

type entryID string

func entry(severity string, at time.Time) api.TimelineEntry {
	return api.TimelineEntry{EventID: string(domain.NewID[entryID]()), Type: fixtureEventType, Severity: severity, Text: severity, At: at}
}

func pipelineRow(title domain.TitleID, stage api.Stage, season, number int) PipelineRow {
	episode := domain.NewID[domain.EpisodeID]()
	return PipelineRow{Row: api.PipelineRow{Title: title, Episode: &episode, Stage: stage}, Season: season, Number: number}
}

type notificationID string

func notification(at time.Time, read bool) api.Notification {
	return api.Notification{ID: string(domain.NewID[notificationID]()), Kind: api.KindRequestApproved, Title: fixtureText, CreatedAt: at, Read: read}
}

type attentionID string

func attention(at time.Time) api.AttentionItem {
	id := string(domain.NewID[attentionID]())
	return api.AttentionItem{ID: id, Kind: api.AttentionAgentOffline, Ref: id, OpenedAt: at}
}

func sameEpisode(a, b *domain.EpisodeID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func assertEntries(t *testing.T, got []api.TimelineEntry, want ...api.TimelineEntry) {
	t.Helper()
	ids := func(entries []api.TimelineEntry) []string {
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.EventID)
		}
		return out
	}
	if !slices.Equal(ids(got), ids(want)) {
		t.Errorf("entries = %v, want %v", ids(got), ids(want))
	}
}

func assertRows(t *testing.T, got []api.PipelineRow, want ...PipelineRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if !sameEpisode(got[i].Episode, want[i].Row.Episode) {
			t.Errorf("row %d is episode %v, want %v", i, *got[i].Episode, *want[i].Row.Episode)
		}
		if got[i].Season != want[i].Season || got[i].Number != want[i].Number {
			t.Errorf("row %d is S%dE%d, want S%dE%d", i, got[i].Season, got[i].Number, want[i].Season, want[i].Number)
		}
	}
}

func assertNotifications(t *testing.T, got []api.Notification, want ...api.Notification) {
	t.Helper()
	ids := func(ns []api.Notification) []string {
		out := make([]string, 0, len(ns))
		for _, n := range ns {
			out = append(out, n.ID)
		}
		return out
	}
	if !slices.Equal(ids(got), ids(want)) {
		t.Errorf("notifications = %v, want %v", ids(got), ids(want))
	}
}
