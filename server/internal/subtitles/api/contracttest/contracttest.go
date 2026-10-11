// Package contracttest holds the subtitles.Reader contract suite and the in-memory Fake that passes it.
// The real repository runs the same suite in its own tests.
package contracttest

import (
	"context"
	"reflect"
	"testing"
	"time"

	"magpie/internal/domain"
	"magpie/internal/subtitles/api"
)

// File is one media file's stored subtitles and wanted-language coverage, in wanted-language order.
type File struct {
	Subtitles []api.Subtitle
	Coverage  []api.LanguageCoverage
}

// Fixture is the stored state a Reader starts with.
type Fixture struct {
	Files map[domain.MediaFileID]File
	Quota api.Quota
}

// Factory returns a Reader holding exactly the fixture's state.
type Factory func(t *testing.T, f Fixture) api.Reader

const (
	english  domain.Language = "en"
	spanish  domain.Language = "es"
	japanese domain.Language = "ja"

	providerScore     = 92
	remainingDownload = 17
	quotaReset        = 3 * time.Hour
)

// Run checks a Reader against the subtitles contract.
func Run(t *testing.T, newReader Factory) {
	ctx := context.Background()
	file := domain.NewID[domain.MediaFileID]()
	score := providerScore
	provider := api.SourceProvider
	embedded := api.SourceEmbedded

	spanishSub := api.Subtitle{Language: spanish, Source: api.SourceProvider, Score: &score, Synced: true, Path: "/m/Film.es.srt"}
	englishForced := api.Subtitle{Language: english, Forced: true, Source: api.SourceRelease, Path: "/m/Film.en.forced.srt"}
	englishSDH := api.Subtitle{Language: english, SDH: true, Source: api.SourceProvider, Score: &score, Path: "/m/Film.en.sdh.srt"}
	englishEmbedded := api.Subtitle{Language: english, Source: api.SourceEmbedded}
	coverage := []api.LanguageCoverage{
		{Language: spanish, Status: api.CoverageSatisfied, Source: &provider},
		{Language: english, Status: api.CoverageSatisfied, Source: &embedded},
		{Language: japanese, Status: api.CoverageMissing},
	}
	quota := api.Quota{Remaining: remainingDownload, ResetAt: time.Now().Add(quotaReset).UTC().Truncate(time.Second)}
	fixture := Fixture{
		Files: map[domain.MediaFileID]File{file: {
			Subtitles: []api.Subtitle{spanishSub, englishForced, englishSDH, englishEmbedded},
			Coverage:  coverage,
		}},
		Quota: quota,
	}

	t.Run("ForFile orders by language, then forced, then SDH", func(t *testing.T) {
		got, err := newReader(t, fixture).ForFile(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		want := []api.Subtitle{englishEmbedded, englishSDH, englishForced, spanishSub}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ForFile = %+v, want %+v", got, want)
		}
	})

	t.Run("ForFile and Coverage return an empty slice for an unknown file", func(t *testing.T) {
		r := newReader(t, fixture)
		unknown := domain.NewID[domain.MediaFileID]()
		subs, err := r.ForFile(ctx, unknown)
		if err != nil || subs == nil || len(subs) != 0 {
			t.Errorf("ForFile = %v, %v; want empty slice", subs, err)
		}
		cov, err := r.Coverage(ctx, unknown)
		if err != nil || cov == nil || len(cov) != 0 {
			t.Errorf("Coverage = %v, %v; want empty slice", cov, err)
		}
	})

	t.Run("Coverage keeps wanted-language order", func(t *testing.T) {
		got, err := newReader(t, fixture).Coverage(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, coverage) {
			t.Errorf("Coverage = %+v, want %+v", got, coverage)
		}
	})

	t.Run("Quota returns downloads left and the reset time", func(t *testing.T) {
		got, err := newReader(t, fixture).Quota(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Remaining != quota.Remaining || !got.ResetAt.Equal(quota.ResetAt) {
			t.Errorf("Quota = %+v, want %+v", got, quota)
		}
	})
}
