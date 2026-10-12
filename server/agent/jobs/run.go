package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"magpie/agent/encode"
	"magpie/agent/transfer"
	"magpie/internal/transcode/api/agentproto"
)

// run takes one job from fetch to result. A job the server cancelled, or that is no longer leased,
// ends without a result; one cut short by the agent stopping keeps its scratch for a resume.
func (m *Manager) run(ctx context.Context, j *job, device agentproto.DeviceReport, mode agentproto.Mode) {
	result, err := m.process(ctx, j, device, mode)
	j.mu.Lock()
	dropped := j.dropped
	j.mu.Unlock()
	switch {
	case dropped || errors.Is(err, transfer.ErrNotLeased):
		slog.InfoContext(ctx, "agent: job dropped", "job", j.spec.Job)
		m.abandon(j, false)
	case ctx.Err() != nil:
		m.abandon(j, true)
	case err != nil:
		result = agentproto.JobResult{Job: j.spec.Job, Outcome: agentproto.OutcomeFailed, Error: err.Error()}
		if errors.Is(err, transfer.ErrSourceChanged) {
			result.Error = agentproto.ErrorSourceChanged
		}
		m.finish(ctx, j, result)
	default:
		m.finish(ctx, j, result)
	}
}

func (m *Manager) process(ctx context.Context, j *job, device agentproto.DeviceReport, mode agentproto.Mode) (agentproto.JobResult, error) {
	if err := prepare(j); err != nil {
		return agentproto.JobResult{}, err
	}
	input, err := m.fetch(ctx, j, mode)
	if err != nil {
		return agentproto.JobResult{}, err
	}
	select {
	case <-ctx.Done():
		return agentproto.JobResult{}, ctx.Err()
	case <-j.started:
	}

	j.set(agentproto.StageEncoding, 0, 0, 0)
	source, err := m.opts.Encoder.Probe(ctx, input)
	if err != nil {
		return agentproto.JobResult{}, err
	}
	output := filepath.Join(j.dir, outputName)
	work := encode.Job{
		Spec: j.spec, Device: device, Input: input, Source: source, Output: output,
		Progress: func(fraction, speed float64) {
			var eta time.Duration
			if speed > 0 {
				eta = time.Duration((1 - fraction) * source.Duration / speed * float64(time.Second))
			}
			j.set(agentproto.StageEncoding, fraction, speed, eta)
		},
	}
	if err := m.opts.Encoder.Transcode(ctx, work); err != nil {
		return agentproto.JobResult{}, err
	}
	report, err := m.check(ctx, work)
	if err != nil {
		return agentproto.JobResult{}, err
	}
	result := agentproto.JobResult{Job: j.spec.Job, Verify: report}
	if bad := failedCheck(report); bad != "" {
		slog.WarnContext(ctx, "agent: output failed its checks", "job", j.spec.Job, "check", bad, "encoder", j.spec.Transcode.Encoder)
		result.Outcome, result.Error = agentproto.OutcomeFailed, agentproto.ErrorVerifyFailed
		return result, nil
	}
	if saving := savingPct(report.OutputSize, j.spec.Source.Size); saving < float64(j.spec.Transcode.MinSavingPct) {
		result.Outcome = agentproto.OutcomeNotWorthIt
		result.Error = fmt.Sprintf("The output is only %.0f%% smaller", saving)
		return result, nil
	}

	j.set(agentproto.StageUploading, 0, 0, 0)
	if _, err := m.opts.Transfers.Upload(ctx, j.spec.OutputURL, output, j.transferred(agentproto.StageUploading)); err != nil {
		return agentproto.JobResult{}, err
	}
	result.Outcome = agentproto.OutcomeUploaded
	return result, nil
}

// fetch downloads the source, or in direct-read mode takes the server's transfer slot and hands ffmpeg
// the URL.
func (m *Manager) fetch(ctx context.Context, j *job, mode agentproto.Mode) (encode.Input, error) {
	if mode == agentproto.ModeDirectRead {
		if err := m.opts.Transfers.Reserve(ctx, j.spec.Source); err != nil {
			return encode.Input{}, err
		}
		return m.opts.DirectRead(j.spec.Source), nil
	}
	path := filepath.Join(j.dir, sourceName)
	if err := m.opts.Transfers.Download(ctx, j.spec.Source, path, j.transferred(agentproto.StageFetching)); err != nil {
		return encode.Input{}, err
	}
	j.set(agentproto.StageFetching, 1, 0, 0)
	return encode.Input{Path: path}, nil
}

// check runs the encoder's checks and adds the output's size and digest.
func (m *Manager) check(ctx context.Context, work encode.Job) (agentproto.VerifyReport, error) {
	report, err := m.opts.Encoder.Verify(ctx, work)
	if err != nil {
		return report, err
	}
	info, err := os.Stat(work.Output)
	if err != nil {
		return report, err
	}
	report.OutputSize = info.Size()
	report.SHA256, err = transfer.Digest(work.Output)
	return report, err
}

// failedCheck names the first check the output failed, or "" when it passed them all.
func failedCheck(r agentproto.VerifyReport) string {
	switch {
	case !r.DecodeOK:
		return "decode"
	case !r.StreamMapOK:
		return "stream map"
	case !r.CopiedStreamsOK:
		return "copied streams"
	case r.HDR10Kept != nil && !*r.HDR10Kept:
		return "HDR10 metadata"
	case abs(r.DurationDelta) > maxDurationDelta:
		return "duration"
	case r.FrameDelta > maxFrameDelta || r.FrameDelta < -maxFrameDelta:
		return "frame count"
	}
	return ""
}

// The output's tolerances against the source, as the server checks them.
const (
	maxDurationDelta = 1.0
	maxFrameDelta    = 1
)

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func savingPct(output, source int64) float64 {
	if source <= 0 {
		return 0
	}
	return (1 - float64(output)/float64(source)) * percent
}
