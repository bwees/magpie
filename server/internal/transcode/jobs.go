package transcode

import (
	"fmt"
	"math"
	"time"

	"magpie/internal/transcode/api"
	"magpie/internal/transcode/api/agentproto"
)

// MaxAttempts is how many times a job runs before it fails for good.
const MaxAttempts = 3

// retryBackoff is the wait before the next attempt after a failed one, indexed by the failed attempt.
var retryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}

func backoffAfter(attempt int) time.Duration {
	return retryBackoff[min(attempt, len(retryBackoff))-1]
}

// failuresBeforeExclusion is how many failed attempts at one job keep an agent away from it for good.
const failuresBeforeExclusion = 2

// Attempt outcomes recorded in transcode_job_attempts.
const (
	outcomeUploaded     = "UPLOADED"
	outcomeNotWorthIt   = "NOT_WORTH_IT"
	outcomeFailed       = "FAILED"
	outcomeLeaseExpired = "LEASE_EXPIRED"
	outcomeCancelled    = "CANCELLED"
	outcomeSwapped      = "SWAPPED"
)

// failedOutcomes count towards excluding an agent from a job. A lost lease doesn't: it is usually a
// restart, and the agent may come back to resume.
var failedOutcomes = []string{outcomeFailed}

// Failure reasons a job records and JobFailed carries.
const (
	ReasonLeaseExpired   = "The agent stopped reporting the job"
	ReasonSourceGone     = "The source file is no longer in the library"
	ReasonSourceMissing  = "The source file is missing from disk"
	ReasonSourceChanged  = agentproto.ErrorSourceChanged
	ReasonUploadMissing  = "The agent reported an upload the server never completed"
	ReasonDigestMismatch = "The upload doesn't match the agent's SHA-256"
	ReasonStreamMap      = "The output's streams don't match the job"
	ReasonCopiedStreams  = "Copied audio or subtitle streams differ from the source"
	ReasonHDR10Lost      = "The output lost the HDR10 metadata"
	ReasonDecode         = "The output doesn't decode cleanly"
	ReasonCancelled      = "Cancelled by an admin"
	ReasonFileDeleted    = "The file was deleted"
	ReasonUpgraded       = "The file was replaced by an upgrade"
	ReasonTitleRemoved   = "The title was removed"
	ReasonPolicySkips    = "The transcode policy now skips the file: "
	ReasonNoAudio        = "The file has no audio for Whisper"
	ReasonBadSubtitle    = "Whisper's subtitle isn't valid SRT"
	ReasonNoCues         = "Whisper heard no speech"
)

// failure is why an attempt ended without a usable output.
type failure struct {
	reason    string
	retryable bool
	// leaseLost keeps the job away from its agent for a while instead of waiting out a backoff.
	leaseLost bool
	outcome   string
}

func retryable(reason string) failure {
	return failure{reason: reason, retryable: true, outcome: outcomeFailed}
}

func final(reason string) failure {
	return failure{reason: reason, outcome: outcomeFailed}
}

var leaseExpired = failure{reason: ReasonLeaseExpired, retryable: true, leaseLost: true, outcome: outcomeLeaseExpired}

// Tolerances of the output against the source.
const (
	maxDurationDelta = time.Second
	maxFrameDelta    = 1
)

// judge checks an agent's verification report. Broken stream maps and lost HDR metadata come from the
// job itself, so trying again won't help; decode and timing problems may be the agent's.
func judge(report agentproto.VerifyReport) (failure, bool) {
	switch {
	case !report.StreamMapOK:
		return final(ReasonStreamMap), false
	case !report.CopiedStreamsOK:
		return final(ReasonCopiedStreams), false
	case report.HDR10Kept != nil && !*report.HDR10Kept:
		return final(ReasonHDR10Lost), false
	case !report.DecodeOK:
		return retryable(ReasonDecode), false
	case math.Abs(report.DurationDelta) > maxDurationDelta.Seconds():
		return retryable(fmt.Sprintf("The output runs %.1f s off the source", report.DurationDelta)), false
	case abs(report.FrameDelta) > maxFrameDelta:
		return retryable(fmt.Sprintf("The output has %d frames more than the source", report.FrameDelta)), false
	}
	return failure{}, true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// agentFailure is how a job the agent reported FAILED ends.
func agentFailure(result agentproto.JobResult) failure {
	switch result.Error {
	case agentproto.ErrorSourceChanged:
		return final(ReasonSourceChanged)
	case agentproto.ErrorVerifyFailed:
		if f, ok := judge(result.Verify); !ok {
			return f
		}
	}
	return retryable(result.Error)
}

// stageStates maps the stage an agent reports to the job state.
var stageStates = map[agentproto.Stage]api.JobState{
	agentproto.StageFetching:  api.StateFetching,
	agentproto.StageEncoding:  api.StateEncoding,
	agentproto.StageChecking:  api.StateChecking,
	agentproto.StageUploading: api.StateUploading,
}
