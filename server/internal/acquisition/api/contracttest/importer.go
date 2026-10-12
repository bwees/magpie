package contracttest

import (
	"context"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"magpie/internal/acquisition/api"
	"magpie/internal/domain"
	"magpie/internal/platform/scripted"
)

const (
	ImporterImportCustomFormats   scripted.Method = "acquisition.Importer.ImportCustomFormats"
	ImporterImportQualityProfiles scripted.Method = "acquisition.Importer.ImportQualityProfiles"
	ImporterImportDelayProfiles   scripted.Method = "acquisition.Importer.ImportDelayProfiles"
	ImporterImportBlocklist       scripted.Method = "acquisition.Importer.ImportBlocklist"
)

// ImporterFactory returns an empty Acquisition.
type ImporterFactory func(t *testing.T) api.Importer

const (
	sonarrRemuxSource  domain.SourceKey = "sonarr:customformat:3"
	radarrRemuxSource  domain.SourceKey = "radarr:customformat:5"
	radarrX265Source   domain.SourceKey = "radarr:customformat:6"
	sonarrHDSource     domain.SourceKey = "sonarr:qualityprofile:4"
	radarrHDSource     domain.SourceKey = "radarr:qualityprofile:4"
	sonarrDelaySource  domain.SourceKey = "sonarr:delayprofile:1"
	blockedSource      domain.SourceKey = "sonarr:blocklist:7"
	remuxFormatName                     = "Remux Tier 01"
	x265FormatName                      = "x265 (HD)"
	hdProfileName                       = "HD-1080p"
	importedRemuxScore                  = 1700
	importedDelay                       = 30 * time.Minute
	blockedReleaseName                  = "Show.S01E01.1080p.WEB-DL-GRP"
	blockedHash                         = "0123456789abcdef0123456789abcdef01234567"
	blockedIndexer                      = "Tracker"
	bluray1080pID                       = 7
	bluray1080pName                     = "Bluray-1080p"
	webdl1080pID                        = 3
	webdl1080pName                      = "WEBDL-1080p"
)

func remuxFormat(source domain.SourceKey) api.ImportCustomFormat {
	return api.ImportCustomFormat{Source: source, Name: remuxFormatName, Specs: []api.FormatSpec{
		{Kind: api.SpecSource, Name: "Bluray", Value: "BLURAY"},
		{Kind: api.SpecQualityModifier, Name: "Remux", Value: "REMUX"},
	}}
}

func hdProfile(source domain.SourceKey, scores map[api.CustomFormatID]int) api.ImportQualityProfile {
	return api.ImportQualityProfile{
		Source: source, Name: hdProfileName, UpgradeAllowed: true,
		Items: []api.ImportProfileItem{
			{Qualities: []domain.Quality{{ID: bluray1080pID, Name: bluray1080pName}}, Allowed: true, Sizes: api.SizeLimits{MaxMBPerMin: maxMBPerMin}},
			{Qualities: []domain.Quality{{ID: webdl1080pID, Name: webdl1080pName}}, Allowed: true, Sizes: api.SizeLimits{MaxMBPerMin: maxMBPerMin}},
		},
		Cutoff: 0, FormatScores: scores,
	}
}

// RunImporter is the acquisition.Importer contract: idempotent on source keys, with same-named custom
// formats and profiles merged, so a resumed migration gets the IDs it got before.
func RunImporter(t *testing.T, factory ImporterFactory) {
	ctx := context.Background()
	formats := []api.ImportCustomFormat{remuxFormat(sonarrRemuxSource), remuxFormat(radarrRemuxSource), {
		Source: radarrX265Source, Name: x265FormatName,
		Specs: []api.FormatSpec{{Kind: api.SpecReleaseTitle, Name: "x265", Value: `[xh][ .]?265`}},
	}}

	t.Run("same-named formats from both apps become one format", func(t *testing.T) {
		importer := factory(t)
		got, err := importer.ImportCustomFormats(ctx, formats)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(formats) || got[sonarrRemuxSource] == "" || got[sonarrRemuxSource] != got[radarrRemuxSource] || got[radarrX265Source] == got[sonarrRemuxSource] {
			t.Fatalf("formats = %v, want the two Remux sources on one format and x265 on another", got)
		}
		again, err := importer.ImportCustomFormats(ctx, formats)
		if err != nil {
			t.Fatal(err)
		}
		if !maps.Equal(got, again) {
			t.Fatalf("re-run = %v, want %v", again, got)
		}
	})

	t.Run("profiles merge by name and keep their IDs on a re-run", func(t *testing.T) {
		importer := factory(t)
		ids, err := importer.ImportCustomFormats(ctx, formats)
		if err != nil {
			t.Fatal(err)
		}
		scores := map[api.CustomFormatID]int{ids[sonarrRemuxSource]: importedRemuxScore}
		profiles := []api.ImportQualityProfile{hdProfile(sonarrHDSource, scores), hdProfile(radarrHDSource, scores)}
		got, err := importer.ImportQualityProfiles(ctx, profiles)
		if err != nil {
			t.Fatal(err)
		}
		if got[sonarrHDSource] == "" || got[sonarrHDSource] != got[radarrHDSource] {
			t.Fatalf("profiles = %v, want both HD-1080p sources on one profile", got)
		}
		again, err := importer.ImportQualityProfiles(ctx, profiles)
		if err != nil {
			t.Fatal(err)
		}
		if !maps.Equal(got, again) {
			t.Fatalf("re-run = %v, want %v", again, got)
		}
	})

	t.Run("delay profiles and the blocklist can be imported again", func(t *testing.T) {
		importer := factory(t)
		delays := []api.ImportDelayProfile{{Source: sonarrDelaySource, Delay: importedDelay}}
		blocked := []api.ImportBlockedRelease{{
			Source: blockedSource, Title: domain.NewID[domain.TitleID](), InfoHash: blockedHash,
			ReleaseName: blockedReleaseName, Indexer: blockedIndexer, At: start,
		}}
		for range 2 {
			if err := importer.ImportDelayProfiles(ctx, delays); err != nil {
				t.Fatal(err)
			}
			if err := importer.ImportBlocklist(ctx, blocked); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// ImporterFake is the in-memory acquisition.Importer the migration job tests against. It passes RunImporter.
type ImporterFake struct {
	scripted.Script

	mu        sync.Mutex
	sources   map[domain.SourceKey]string
	formats   map[string]api.CustomFormatID
	profiles  map[string]domain.ProfileID
	delay     *time.Duration
	blocklist []api.ImportBlockedRelease
	imported  map[string]api.ImportQualityProfile
}

var _ api.Importer = (*ImporterFake)(nil)

func NewImporterFake() *ImporterFake {
	return &ImporterFake{
		sources:  map[domain.SourceKey]string{},
		formats:  map[string]api.CustomFormatID{},
		profiles: map[string]domain.ProfileID{},
		imported: map[string]api.ImportQualityProfile{},
	}
}

func (f *ImporterFake) ImportCustomFormats(ctx context.Context, in []api.ImportCustomFormat) (map[domain.SourceKey]api.CustomFormatID, error) {
	if err := f.Enter(ctx, ImporterImportCustomFormats); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[domain.SourceKey]api.CustomFormatID{}
	for _, format := range in {
		out[format.Source] = api.CustomFormatID(f.byName(format.Source, format.Name, func() string {
			id := domain.NewID[api.CustomFormatID]()
			f.formats[strings.ToLower(format.Name)] = id
			return string(id)
		}, func(name string) (string, bool) {
			id, ok := f.formats[name]
			return string(id), ok
		}))
	}
	return out, nil
}

func (f *ImporterFake) ImportQualityProfiles(ctx context.Context, in []api.ImportQualityProfile) (map[domain.SourceKey]domain.ProfileID, error) {
	if err := f.Enter(ctx, ImporterImportQualityProfiles); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[domain.SourceKey]domain.ProfileID{}
	for _, profile := range in {
		out[profile.Source] = domain.ProfileID(f.byName(profile.Source, profile.Name, func() string {
			id := domain.NewID[domain.ProfileID]()
			f.profiles[strings.ToLower(profile.Name)] = id
			f.imported[string(id)] = profile
			return string(id)
		}, func(name string) (string, bool) {
			id, ok := f.profiles[name]
			return string(id), ok
		}))
	}
	return out, nil
}

// byName returns the ID a source got before, else the ID of the same-named item, else a new one.
func (f *ImporterFake) byName(source domain.SourceKey, name string, create func() string, named func(string) (string, bool)) string {
	if id, ok := f.sources[source]; ok {
		return id
	}
	id, ok := named(strings.ToLower(name))
	if !ok {
		id = create()
	}
	f.sources[source] = id
	return id
}

// ImportDelayProfiles keeps the shortest delay of the untagged profiles, as the real one does.
func (f *ImporterFake) ImportDelayProfiles(ctx context.Context, in []api.ImportDelayProfile) error {
	if err := f.Enter(ctx, ImporterImportDelayProfiles); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range in {
		if len(p.Tags) == 0 && (f.delay == nil || p.Delay < *f.delay) {
			delay := p.Delay
			f.delay = &delay
		}
	}
	return nil
}

func (f *ImporterFake) ImportBlocklist(ctx context.Context, in []api.ImportBlockedRelease) error {
	if err := f.Enter(ctx, ImporterImportBlocklist); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range in {
		if _, ok := f.sources[row.Source]; ok {
			continue
		}
		f.sources[row.Source] = row.InfoHash
		f.blocklist = append(f.blocklist, row)
	}
	return nil
}

// Blocklist lists the imported rows, one per source.
func (f *ImporterFake) Blocklist() []api.ImportBlockedRelease {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]api.ImportBlockedRelease(nil), f.blocklist...)
}

// Profile is the import a profile was created from.
func (f *ImporterFake) Profile(id domain.ProfileID) (api.ImportQualityProfile, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.imported[string(id)]
	return p, ok
}

// Delay is the default delay the untagged profiles set, nil when none was imported.
func (f *ImporterFake) Delay() *time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.delay
}
