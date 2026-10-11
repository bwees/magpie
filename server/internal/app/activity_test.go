package app

import (
	"slices"
	"strings"
	"testing"

	"magpie/internal/activity"
	transcodeapi "magpie/internal/transcode/api"
)

const activityPrefix = "activity."

// Activity's handlers record every durable event in the catalog except commands and its own events.
func TestActivityRecordsEveryDurableEvent(t *testing.T) {
	var want []string
	for _, m := range Messages() {
		if m.Durable() && m.Type() != transcodeapi.TypeRunAgentJob && !strings.HasPrefix(m.Type(), activityPrefix) {
			want = append(want, m.Type())
		}
	}
	got := activity.DurableEventTypes()
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("activity records %v\nwant %v", got, want)
	}
}
