import type { JobResponse } from '$lib/sdk';
import { formatEta, formatSpeed } from '$lib/downloads/downloads';
import { languageName } from '$lib/subtitles/subtitles';

/** Mirrors the Go `api.JobState` values. */
export enum JobState {
	QUEUED = 'QUEUED',
	OFFERED = 'OFFERED',
	LEASED = 'LEASED',
	FETCHING = 'FETCHING',
	ENCODING = 'ENCODING',
	CHECKING = 'CHECKING',
	UPLOADING = 'UPLOADING',
	VERIFYING = 'VERIFYING',
	VERIFIED = 'VERIFIED',
	SWAPPED = 'SWAPPED',
	COMPLETED = 'COMPLETED',
	FAILED = 'FAILED',
	NOT_WORTH_IT = 'NOT_WORTH_IT',
	CANCELLED = 'CANCELLED'
}

export enum JobKind {
	TRANSCODE = 'TRANSCODE',
	WHISPER = 'WHISPER'
}

export enum JobOrigin {
	DOWNLOAD = 'DOWNLOAD',
	LIBRARY_SCAN = 'LIBRARY_SCAN',
	MIGRATION = 'MIGRATION'
}

/** Live events that change the GPU jobs list. */
export enum JobEvent {
	PROGRESSED = 'transcode.JobProgressed',
	QUEUED = 'transcode.JobQueued',
	STARTED = 'transcode.JobStarted',
	FAILED = 'transcode.JobFailed',
	VERIFIED = 'transcode.TranscodeVerified',
	SKIPPED = 'transcode.TranscodeSkipped',
	WHISPER_COMPLETED = 'transcode.WhisperCompleted',
	FILE_TRANSCODED = 'library.FileTranscoded'
}

/** States in which an agent or the server is working on the job. */
export const RUNNING_STATES: string[] = [
	JobState.OFFERED,
	JobState.LEASED,
	JobState.FETCHING,
	JobState.ENCODING,
	JobState.CHECKING,
	JobState.UPLOADING,
	JobState.VERIFYING,
	JobState.VERIFIED
];

/** What the GPU jobs tab asks the server for. Not-worth-it files show among the skips. */
export const LISTED_STATES: string[] = [JobState.QUEUED, ...RUNNING_STATES, JobState.FAILED];

export const QUEUED_PAGE_SIZE = 10;

/** The tabs that split waiting jobs from failed ones. */
export enum JobListTab {
	QUEUED = 'queued',
	FAILED = 'failed'
}

export const STATE_LABELS: Record<string, string> = {
	[JobState.QUEUED]: 'Queued',
	[JobState.OFFERED]: 'Offered',
	[JobState.LEASED]: 'Starting',
	[JobState.FETCHING]: 'Fetching source',
	[JobState.ENCODING]: 'Encoding',
	[JobState.CHECKING]: 'Checking output',
	[JobState.UPLOADING]: 'Uploading',
	[JobState.VERIFYING]: 'Verifying',
	[JobState.VERIFIED]: 'Swapping in',
	[JobState.SWAPPED]: 'Swapped',
	[JobState.COMPLETED]: 'Completed',
	[JobState.FAILED]: 'Failed',
	[JobState.NOT_WORTH_IT]: 'Not worth it',
	[JobState.CANCELLED]: 'Cancelled'
};

const TARGET_LABELS: Record<string, string> = {
	HEVC_10: 'HEVC 10-bit',
	AV1_10: 'AV1 10-bit'
};

const KIND_LABELS: Record<string, string> = {
	[JobKind.TRANSCODE]: 'Transcode',
	[JobKind.WHISPER]: 'Whisper'
};

/** "Transcode · HEVC 10-bit", or "Whisper · English". */
export function kindLabel(job: Pick<JobResponse, 'kind' | 'targetCodec' | 'language'>): string {
	const kind = KIND_LABELS[job.kind] ?? job.kind;
	const detail =
		job.kind === JobKind.WHISPER
			? job.language && languageName(job.language)
			: TARGET_LABELS[job.targetCodec];
	return detail ? `${kind} · ${detail}` : kind;
}

/** The title with the episode, or the file name when the title is gone. */
export function jobName(job: Pick<JobResponse, 'titleName' | 'episodes' | 'fileName'>): string {
	const name = job.titleName || job.fileName;
	return job.episodes ? `${name} ${job.episodes}` : name;
}

/** The live detail of a running job: encode speed as a multiple of realtime, or bytes per second. */
export interface JobDetail {
	speed: number;
	etaSeconds: number;
}

const TRANSFER_STATES: string[] = [JobState.FETCHING, JobState.UPLOADING];

const PREFETCHED_LABEL = 'Prefetched';
const WAITING_FOR_SLOT = 'Fetched ahead; encodes when a slot frees';
const FETCHED = 1;

type JobProgress = Pick<JobResponse, 'state' | 'prefetched' | 'progress'>;

/** A prefetched job that has its whole source waits for a slot rather than fetching. */
function waitingForSlot(job: JobProgress): boolean {
	return job.prefetched && job.state === JobState.FETCHING && job.progress >= FETCHED;
}

export function stateLabel(job: JobProgress): string {
	return waitingForSlot(job) ? PREFETCHED_LABEL : (STATE_LABELS[job.state] ?? job.state);
}

export function detailLabel(job: JobProgress, detail: JobDetail | undefined): string {
	const state = job.state;
	if (waitingForSlot(job)) {
		return WAITING_FOR_SLOT;
	}
	if (!detail || detail.speed <= 0) {
		return '—';
	}
	const speed = TRANSFER_STATES.includes(state)
		? formatSpeed(detail.speed)
		: `${detail.speed.toFixed(1)}× realtime`;
	return `${speed} · ${formatEta(detail.etaSeconds)} left`;
}

/** "Attempt 2 of 3". */
export function attemptLabel(job: Pick<JobResponse, 'attempt' | 'maxAttempts'>): string {
	return `Attempt ${job.attempt} of ${job.maxAttempts}`;
}

const ORIGIN_LABELS: Record<string, string> = {
	[JobOrigin.DOWNLOAD]: 'Download',
	[JobOrigin.LIBRARY_SCAN]: 'Library scan',
	[JobOrigin.MIGRATION]: 'Migration'
};

export function originLabel(origin: string): string {
	return ORIGIN_LABELS[origin] ?? origin;
}
