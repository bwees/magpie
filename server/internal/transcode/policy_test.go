package transcode

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"testing"
	"time"

	catalogapi "magpie/internal/catalog/api"
	"magpie/internal/domain"
	libraryapi "magpie/internal/library/api"
	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"

	"gorm.io/gorm"
)

const (
	uhdWidth  = 3840
	uhdHeight = 2160
)

func (f *fixture) jobsOf(file sourceFile) []jobRow {
	f.t.Helper()
	var rows []jobRow
	if err := f.db.Read(context.Background()).Where("media_file_id = ?", file.ID).Order("queued_at, id").Find(&rows).Error; err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func (f *fixture) skips(reason api.SkipReason) []api.TranscodeSkipped {
	f.t.Helper()
	all := decodeEvents[api.TranscodeSkipped](f.t, f.published(api.TypeTranscodeSkipped))
	return slices.DeleteFunc(all, func(e api.TranscodeSkipped) bool { return e.Reason != reason })
}

func withVideo(info libraryapi.MediaInfo, codec string, mbps int) libraryapi.MediaInfo {
	info.Streams = slices.Clone(info.Streams)
	info.Streams[0].Codec = codec
	info.BitrateKbps = mbps * kbpsPerMbps
	return info
}

func uhd(info libraryapi.MediaInfo) libraryapi.MediaInfo {
	info.Width, info.Height = uhdWidth, uhdHeight
	return info
}

// Seed 3.
func TestCutoffQueuesATargetCodecFileOverItsCap(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(uhd(withVideo(h264Movie(0), api.VideoCodecHEVC, 38)))

	f.cutoff(file, domain.OriginDownload)

	if job := f.onlyJob(file); job.State != api.StateQueued || job.MaxRateKbps != defaultCap2160Mbps*kbpsPerMbps {
		t.Fatalf("job = %+v, want QUEUED at the 2160p cap", job)
	}
}

// Seed 4.
func TestCutoffSkipsALowBitrateSourceBelowHalfItsCap(t *testing.T) {
	f := newFixture(t)
	low := f.addFile(withVideo(h264Movie(0), "h264", 4))
	enough := f.addFile(withVideo(h264Movie(0), "h264", 6))

	f.cutoff(low, domain.OriginDownload)
	f.cutoff(enough, domain.OriginDownload)

	if skips := f.skips(api.SkipLowBitrate); len(skips) != 1 || skips[0].MediaFile != low.ID {
		t.Fatalf("LOW_BITRATE skips = %+v, want one for the 4 Mbps file", skips)
	}
	if jobs := f.jobsOf(low); len(jobs) != 0 {
		t.Fatalf("the 4 Mbps file has %d jobs, want none", len(jobs))
	}
	f.onlyJob(enough)
}

func TestLowBitrateSkipFollowsItsSwitchAndPercentage(t *testing.T) {
	f := newFixture(t)
	f.setPolicy(func(p *Policy) { p.LowBitratePct = 30 })
	file := f.addFile(withVideo(h264Movie(0), "h264", 4))

	f.cutoff(file, domain.OriginDownload)
	f.onlyJob(file)

	f.setPolicy(func(p *Policy) { p.SkipLowBitrate = false })
	other := f.addFile(withVideo(h264Movie(0), "h264", 1))
	f.cutoff(other, domain.OriginDownload)
	f.onlyJob(other)
}

// Seed 5.
func TestCutoffSkipsDynamicHDRAndKeepsHDR10(t *testing.T) {
	for _, hdr := range []libraryapi.HDRFormat{libraryapi.HDRDolbyVision, libraryapi.HDR10Plus} {
		t.Run(string(hdr), func(t *testing.T) {
			f := newFixture(t)
			info := uhd(withVideo(h264Movie(0), api.VideoCodecHEVC, 60))
			info.HDR = hdr
			file := f.addFile(info)

			f.cutoff(file, domain.OriginDownload)

			if skips := f.skips(api.SkipHDRDynamic); len(skips) != 1 {
				t.Fatalf("HDR_DYNAMIC skips = %+v, want one", skips)
			}
		})
	}

	f := newFixture(t)
	info := uhd(withVideo(h264Movie(0), api.VideoCodecHEVC, 60))
	info.HDR = libraryapi.HDR10
	file := f.addFile(info)
	f.cutoff(file, domain.OriginDownload)
	if job := f.onlyJob(file); !job.HDR10 {
		t.Fatalf("job = %+v, want HDR10 kept", job)
	}
	a, _ := f.connect(capableHello("gpu-a"))
	if offer := f.offered(a); !offer.Spec.Transcode.HDR10 {
		t.Fatalf("offer = %+v, want HDR10", offer.Spec.Transcode)
	}
}

// Seed 6.
func TestCutoffSkipsAnExcludedTitleAndIncludingQueuesIt(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	excluded := Exclusion{Scope: ExcludeTitle, Ref: string(file.Title)}
	if err := f.jobs.Exclude(context.Background(), excluded); err != nil {
		t.Fatal(err)
	}

	f.cutoff(file, domain.OriginDownload)

	if skips := f.skips(api.SkipExcluded); len(skips) != 1 {
		t.Fatalf("EXCLUDED skips = %+v, want one", skips)
	}
	if err := f.jobs.Include(context.Background(), excluded); err != nil {
		t.Fatal(err)
	}
	f.onlyJob(file)
}

func TestExcludingATitleCancelsItsQueuedJob(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)

	if err := f.jobs.Exclude(context.Background(), Exclusion{Scope: ExcludeTitle, Ref: string(file.Title)}); err != nil {
		t.Fatal(err)
	}

	if job := f.onlyJob(file); job.State != api.StateCancelled {
		t.Fatalf("job = %s, want CANCELLED", job.State)
	}
	if skips := f.skips(api.SkipExcluded); len(skips) != 1 {
		t.Fatalf("EXCLUDED skips = %+v, want one", skips)
	}
}

func TestExcludedRootFolderSkipsItsFilesOnTheNextSweep(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)

	if err := f.jobs.Exclude(context.Background(), Exclusion{Scope: ExcludeRootFolder, Ref: file.RootFolder + "/"}); err != nil {
		t.Fatal(err)
	}
	f.runTasks()

	if job := f.onlyJob(file); job.State != api.StateCancelled {
		t.Fatalf("job = %s, want CANCELLED once its root folder is excluded", job.State)
	}
}

// Seed 9.
func TestNoCapableAgentSkipsUntilOneConnects(t *testing.T) {
	f := newFixture(t)
	f.setPolicy(func(p *Policy) { p.TargetCodec = api.TargetAV110 })
	file := f.addFile(h264Movie(0))

	f.cutoff(file, domain.OriginDownload)

	if skips := f.skips(api.SkipNoCapableAgent); len(skips) != 1 {
		t.Fatalf("NO_CAPABLE_AGENT skips = %+v, want one", skips)
	}
	f.connect(capableHello("hevc-only"))
	f.runTasks()
	if jobs := f.jobsOf(file); len(jobs) != 0 {
		t.Fatalf("an HEVC-only agent queued %d jobs", len(jobs))
	}

	hello := capableHello("av1-box")
	hello.Devices[0].Capabilities = []agentproto.CapabilityResult{{Capability: agentproto.CapEncAV110, OK: true}}
	f.connect(hello)
	f.runTasks()

	if job := f.onlyJob(file); job.State != api.StateQueued || job.TargetCodec != api.TargetAV110 {
		t.Fatalf("job = %+v, want an AV1 job queued", job)
	}
	if n := len(f.skips(api.SkipNoCapableAgent)); n != 1 {
		t.Fatalf("NO_CAPABLE_AGENT published %d times, want once", n)
	}
}

func TestCapabilitiesChangedLiftsANoCapableAgentSkip(t *testing.T) {
	f := newFixture(t)
	f.setPolicy(func(p *Policy) { p.TargetCodec = api.TargetAV110 })
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))

	report := capableHello("gpu-a").Devices[0]
	report.Capabilities = []agentproto.CapabilityResult{{Capability: agentproto.CapEncAV110, OK: true}}
	a.changeCapabilities(report)
	f.runTasks()

	f.onlyJob(file)
}

// Seed 19.
func TestNotWorthItStaysSkippedUntilThePolicyChanges(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))
	spec := f.lease(a)
	a.send(agentproto.TypeJobResult, agentproto.JobResult{Job: spec.Job, Outcome: agentproto.OutcomeNotWorthIt, Error: "9% smaller"})
	f.inState(spec.Job, api.StateNotWorthIt)

	if err := f.jobs.RequestSweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.runTasks()
	if jobs := f.jobsOf(file); len(jobs) != 1 {
		t.Fatalf("a sweep under the same policy queued it again: %d jobs", len(jobs))
	}

	f.setPolicy(func(p *Policy) { p.MinSavingPct = 10 })
	f.runTasks()
	jobs := f.jobsOf(file)
	if len(jobs) != 2 || jobs[1].State != api.StateQueued {
		t.Fatalf("jobs = %+v, want a new job queued under the new policy", jobs)
	}
}

func TestSweepWorksThroughTheBacklogInBatches(t *testing.T) {
	f := newFixture(t)
	files := make([]sourceFile, 0, sweepBatch+1)
	for range sweepBatch + 1 {
		file := f.addFile(withVideo(h264Movie(0), "h264", 1))
		f.cutoff(file, domain.OriginLibraryScan)
		files = append(files, file)
	}

	f.setPolicy(func(p *Policy) { p.SkipLowBitrate = false })
	first := f.queue.drain()
	if len(first) != 1 {
		t.Fatalf("tasks = %+v, want one sweep", first)
	}
	if err := f.jobs.SweepBacklog(context.Background(), first[0].(sweepTask)); err != nil {
		t.Fatal(err)
	}
	next := f.queue.drain()
	if len(next) != 1 || next[0].(sweepTask).After == "" {
		t.Fatalf("tasks = %+v, want the next batch queued", next)
	}
	if err := f.queue.Enqueue(context.Background(), next[0]); err != nil {
		t.Fatal(err)
	}
	f.runTasks()

	for _, file := range files {
		f.onlyJob(file)
	}
}

func TestATranscodedFileIsANoOpUnderTheSamePolicy(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.cutoff(file, domain.OriginDownload)
	job := f.onlyJob(file)
	err := f.db.Write(context.Background(), func(tx *gorm.DB) error {
		_, err := moveJob(tx, job.ID, []api.JobState{api.StateQueued}, map[string]any{"state": api.StateSwapped})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	f.cutoff(file, domain.OriginDownload)

	if jobs := f.jobsOf(file); len(jobs) != 1 {
		t.Fatalf("jobs = %d, want the swapped one only", len(jobs))
	}
	if n := len(f.published(api.TypeTranscodeSkipped)); n != 0 {
		t.Fatalf("published %d skips for a transcoded file", n)
	}
}

func TestAPolicyChangeReplansQueuedJobsAndCancelsThoseNowSkipped(t *testing.T) {
	f := newFixture(t)
	f.knownEncoder(agentproto.CapEncAV110)
	movie := f.addFile(h264Movie(0))
	hevc := f.addFile(withVideo(h264Movie(0), api.VideoCodecHEVC, 12))
	f.cutoff(movie, domain.OriginDownload)
	f.cutoff(hevc, domain.OriginDownload)

	f.setPolicy(func(p *Policy) {
		p.TargetCodec = api.TargetAV110
		p.CapsMbps[api.Resolution1080] = 15
	})
	f.runTasks()

	if job := f.onlyJob(movie); job.TargetCodec != api.TargetAV110 || job.MaxRateKbps != 15*kbpsPerMbps {
		t.Fatalf("job = %+v, want it moved to AV1 at 15 Mbps", job)
	}
	f.setPolicy(func(p *Policy) { p.CapsMbps[api.Resolution1080] = 20 })
	f.runTasks()
	if job := f.onlyJob(hevc); job.State != api.StateQueued {
		t.Fatalf("HEVC file job = %s; an AV1 target still transcodes it", job.State)
	}
	f.setPolicy(func(p *Policy) { p.TargetCodec = api.TargetHEVC10 })
	f.runTasks()
	if job := f.onlyJob(hevc); job.State != api.StateCancelled {
		t.Fatalf("HEVC file job = %s, want CANCELLED once HEVC is the target again and it is under 20 Mbps", job.State)
	}
}

func TestTurningTranscodingOffHoldsTheQueue(t *testing.T) {
	f := newFixture(t)
	file := f.addFile(h264Movie(0))
	f.setPolicy(func(p *Policy) { p.Enabled = false })

	f.cutoff(file, domain.OriginDownload)
	if jobs := f.jobsOf(file); len(jobs) != 0 {
		t.Fatalf("queued %d jobs while transcoding is off", len(jobs))
	}

	f.setPolicy(func(p *Policy) { p.Enabled = true })
	f.runTasks()
	job := f.onlyJob(file)
	f.setPolicy(func(p *Policy) { p.Enabled = false })
	a, _ := f.connect(capableHello("gpu-a"))
	f.schedule()
	if got := f.job(job.ID); got.State != api.StateQueued {
		t.Fatalf("job = %s, want it held while transcoding is off", got.State)
	}
	f.setPolicy(func(p *Policy) { p.Enabled = true })
	f.offered(a)
}

// Seed 21.
func TestDownloadJobGoesAheadOfAnEarlierMigrationJob(t *testing.T) {
	f := newFixture(t)
	migrated := f.addFile(h264Movie(0))
	downloaded := f.addFile(h264Movie(0))
	f.cutoff(migrated, domain.OriginMigration)
	f.clock.Advance(time.Second)
	f.cutoff(downloaded, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))

	offer := f.offered(a)

	if offer.Spec.Job != f.onlyJob(downloaded).ID {
		t.Fatalf("offered %s, want the DOWNLOAD job first", offer.Spec.Job)
	}
	if offer.Prefetch {
		t.Fatal("the DOWNLOAD job came as a prefetch")
	}
}

// Seed 32.
func TestOfferKeepsTheTitlesWantedAndOriginalLanguages(t *testing.T) {
	f := newFixture(t)
	info := h264Movie(0)
	info.Streams = []libraryapi.Stream{
		{Index: 0, Kind: libraryapi.StreamVideo, Codec: "h264", BitDepth: 8},
		{Index: 1, Kind: libraryapi.StreamAudio, Codec: "eac3", Language: "en"},
		{Index: 2, Kind: libraryapi.StreamAudio, Codec: "aac", Language: "ja"},
		{Index: 3, Kind: libraryapi.StreamAudio, Codec: "ac3", Language: "fr"},
		{Index: 4, Kind: libraryapi.StreamSubtitle, Codec: "subrip", Language: "fr"},
		{Index: 5, Kind: libraryapi.StreamSubtitle, Codec: "hdmv_pgs_subtitle", Language: "en", Image: true},
		{Index: 6, Kind: libraryapi.StreamSubtitle, Codec: "subrip", Language: undetermined},
	}
	file := f.addFile(info)
	title, err := f.catalog.Title(context.Background(), file.Title)
	if err != nil {
		t.Fatal(err)
	}
	title.OriginalLanguage = "ja"
	f.catalog.PutTitle(title)
	f.catalog.SetGlobalLanguages([]catalogapi.WantedLanguage{{Language: "fr"}})
	f.catalog.SetLanguageOverride(file.Title, []catalogapi.WantedLanguage{{Language: "en"}})
	f.cutoff(file, domain.OriginDownload)
	a, _ := f.connect(capableHello("gpu-a"))

	params := f.offered(a).Spec.Transcode

	if !slices.Equal(params.KeepAudio, []int{1, 2}) || !slices.Equal(params.KeepSubs, []int{5, 6}) {
		t.Fatalf("kept audio %v and subs %v, want [1 2] and [5 6]", params.KeepAudio, params.KeepSubs)
	}
}

func TestOfferUsesTheCalibratedPresetOrTheEncodersOverride(t *testing.T) {
	f := newFixture(t)
	f.setPolicy(func(p *Policy) {
		p.Preset = api.PresetSmall
		p.EncoderOverrides = map[string]int{EncoderHEVCQSV: 20}
	})
	first := f.addFile(h264Movie(0))
	second := f.addFile(h264Movie(0))
	f.cutoff(first, domain.OriginDownload)
	f.cutoff(second, domain.OriginDownload)
	nvidia, _ := f.connect(capableHello("gpu-nvidia"))
	intelHello := capableHello("gpu-intel")
	intelHello.Devices[0].Vendor = agentproto.VendorIntel
	intel, _ := f.connect(intelHello)

	f.schedule()
	nvenc := decodeBody[agentproto.JobOffer](t, nvidia.expect(agentproto.TypeJobOffer)).Spec.Transcode
	qsv := decodeBody[agentproto.JobOffer](t, intel.expect(agentproto.TypeJobOffer)).Spec.Transcode

	small := strconv.Itoa(calibration.Encoders[EncoderHEVCNVENC].Presets[api.PresetSmall])
	if nvenc.Encoder != EncoderHEVCNVENC || nvenc.EncoderArgs["cq"] != small || nvenc.EncoderArgs["rc"] != "vbr" {
		t.Fatalf("NVENC args = %v, want cq %s", nvenc.EncoderArgs, small)
	}
	if qsv.Encoder != EncoderHEVCQSV || qsv.EncoderArgs["global_quality"] != "20" {
		t.Fatalf("QSV args = %v, want the override 20", qsv.EncoderArgs)
	}
}

func TestVAAPIQualityRateControlGetsTheBitrateItRequires(t *testing.T) {
	const capKbps = 10000
	balanced := strconv.Itoa(calibration.Encoders[EncoderHEVCVAAPI].Presets[api.PresetBalanced])

	capped, err := encoderArgs(EncoderHEVCVAAPI, api.PresetBalanced, nil, capKbps)
	if err != nil || capped["rc_mode"] != "QVBR" || capped["global_quality"] != balanced || capped["b:v"] != "10000k" {
		t.Fatalf("capped VAAPI args = %v, %v; want QVBR at %s with b:v 10000k", capped, err, balanced)
	}
	uncapped, err := encoderArgs(EncoderHEVCVAAPI, api.PresetBalanced, nil, 0)
	if _, hasQuality := uncapped["global_quality"]; err != nil || uncapped["rc_mode"] != "CQP" || uncapped["qp"] != balanced || hasQuality {
		t.Fatalf("uncapped VAAPI args = %v, %v; want CQP with qp %s", uncapped, err, balanced)
	}
	nvenc, err := encoderArgs(EncoderHEVCNVENC, api.PresetBalanced, nil, capKbps)
	if want := calibration.Encoders[EncoderHEVCNVENC].Args["b:v"]; err != nil || nvenc["b:v"] != want {
		t.Fatalf("NVENC args = %v, %v; want its calibrated b:v %q kept", nvenc, err, want)
	}
}

func TestDecideFollowsTheRulesInOrder(t *testing.T) {
	policy := policyKey.Default
	h264 := withVideo(h264Movie(0), "h264", 12)
	dolby := uhd(h264)
	dolby.HDR = libraryapi.HDRDolbyVision
	noBitrate := withVideo(h264Movie(0), "h264", 0)
	cases := []struct {
		name  string
		facts fileFacts
		want  policyResult
		skip  api.SkipReason
	}{
		{"excluded wins over dynamic HDR", fileFacts{Info: dolby, Excluded: true, CapableAgentSeen: true}, resultSkip, api.SkipExcluded},
		{"dynamic HDR wins over done", fileFacts{Info: dolby, DoneUnderPolicy: true, CapableAgentSeen: true}, resultSkip, api.SkipHDRDynamic},
		{"done", fileFacts{Info: h264, DoneUnderPolicy: true}, resultDone, ""},
		{"low bitrate wins over no agent", fileFacts{Info: withVideo(h264, "h264", 1)}, resultSkip, api.SkipLowBitrate},
		{"no capable agent", fileFacts{Info: h264}, resultSkip, api.SkipNoCapableAgent},
		{"unknown bitrate is transcoded", fileFacts{Info: noBitrate, CapableAgentSeen: true}, resultQueue, ""},
		{"queue", fileFacts{Info: h264, CapableAgentSeen: true}, resultQueue, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := policy.decide(c.facts)
			if got.Result != c.want || got.Skip != c.skip {
				t.Fatalf("decide = %+v, want %s %s", got, c.want, c.skip)
			}
		})
	}
}

func TestHashIgnoresTheOnSwitchAndEmptyOverrides(t *testing.T) {
	on := policyKey.Default
	off := on
	off.Enabled = false
	off.EncoderOverrides = nil
	if on.Hash() != off.Hash() {
		t.Fatal("turning transcoding off changed the policy hash")
	}
	changed := on
	changed.MinSavingPct++
	if changed.Hash() == on.Hash() {
		t.Fatal("a new minimum saving kept the policy hash")
	}
}

func TestEveryEncoderHasACalibrationForEveryPreset(t *testing.T) {
	for _, byTarget := range encoders {
		for _, encoder := range byTarget {
			cal, ok := calibration.Encoders[encoder]
			if !ok {
				t.Fatalf("%s has no calibration", encoder)
			}
			values := make([]int, 0, len(presets))
			for _, preset := range presets {
				value, ok := cal.Presets[preset]
				if !ok || !cal.Quality.contains(value) {
					t.Fatalf("%s %s = %d, want a value in %d–%d", encoder, preset, value, cal.Quality.Min, cal.Quality.Max)
				}
				values = append(values, value)
			}
			if !slices.IsSorted(values) || len(slices.Compact(slices.Clone(values))) != len(values) {
				t.Fatalf("%s values %v don't fall from Archive to Small quality", encoder, values)
			}
		}
	}
	if keys := slices.Collect(maps.Keys(calibration.Encoders)); len(keys) != len(encoders)*len(TargetCodecs) {
		t.Fatalf("calibrated encoders %v, want one per vendor and target", keys)
	}
}

func TestKeptTracksKeepAllAudioWhenNoneIsWanted(t *testing.T) {
	info := h264Movie(0)
	audio, subs := keptTracks(info, keptLanguages{Languages: []domain.Language{"de"}})
	if !slices.Equal(audio, []int{1, 2}) || len(subs) != 0 {
		t.Fatalf("kept audio %v subs %v, want all audio and no subs", audio, subs)
	}
}
