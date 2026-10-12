/** Mirrors the Go `api.DownloadState` values the Downloads tab shows. */
export enum DownloadState {
	AWAITING_METADATA = 'AWAITING_METADATA',
	DOWNLOADING = 'DOWNLOADING',
	STALLED = 'STALLED',
	COMPLETED = 'COMPLETED',
	SEEDING = 'SEEDING',
	IMPORTED = 'IMPORTED'
}

/** Mirrors the Go `api.SeedingMode` values. */
export enum SeedingMode {
	SEED_UNTIL_GOALS = 'SEED_UNTIL_GOALS',
	DONT_SEED = 'DONT_SEED'
}

/** Mirrors the Go `api.BlocklistReason` values. */
export enum BlocklistReason {
	BLOCKED_FILE = 'BLOCKED_FILE',
	METADATA_TIMEOUT = 'METADATA_TIMEOUT',
	STALLED = 'STALLED',
	IMPORT_FAILED = 'IMPORT_FAILED',
	DOWNLOAD_FAILED = 'DOWNLOAD_FAILED',
	USER = 'USER',
	ISSUE = 'ISSUE',
	MIGRATED = 'MIGRATED'
}

export enum DownloadEvent {
	PROGRESSED = 'acquisition.DownloadProgressed',
	GRABBED = 'acquisition.ReleaseGrabbed',
	REJECTED = 'acquisition.ReleaseRejected',
	COMPLETED = 'acquisition.DownloadCompleted',
	FAILED = 'acquisition.DownloadFailed'
}

export const STATE_LABELS: Record<string, string> = {
	[DownloadState.AWAITING_METADATA]: 'Fetching metadata',
	[DownloadState.DOWNLOADING]: 'Downloading',
	[DownloadState.STALLED]: 'Stalled',
	[DownloadState.COMPLETED]: 'Completed',
	[DownloadState.SEEDING]: 'Seeding',
	[DownloadState.IMPORTED]: 'Imported'
};

export const SEEDING_LABELS: Record<string, string> = {
	[SeedingMode.SEED_UNTIL_GOALS]: 'Seed to goal',
	[SeedingMode.DONT_SEED]: 'No seeding'
};

export const REASON_LABELS: Record<string, string> = {
	[BlocklistReason.BLOCKED_FILE]: 'Blocked file',
	[BlocklistReason.METADATA_TIMEOUT]: 'Metadata timeout',
	[BlocklistReason.STALLED]: 'Stalled',
	[BlocklistReason.IMPORT_FAILED]: 'Import failed',
	[BlocklistReason.DOWNLOAD_FAILED]: 'Download failed',
	[BlocklistReason.USER]: 'Removed by you',
	[BlocklistReason.ISSUE]: 'Reported issue',
	[BlocklistReason.MIGRATED]: 'Migrated'
};

/** Fetching metadata and stalled get the muted badge; the rest the teal one. */
export function isQuiet(state: string): boolean {
	return state === DownloadState.AWAITING_METADATA || state === DownloadState.STALLED;
}

export function isFinished(state: string): boolean {
	return state === DownloadState.COMPLETED || state === DownloadState.SEEDING;
}

const NANOSECONDS_PER_SECOND = 1e9;
const SECONDS_PER_MINUTE = 60;
const SECONDS_PER_HOUR = 3600;
const BYTES_PER_KB = 1024;
const BYTES_PER_MB = 1024 ** 2;
const PERCENT = 100;

/** Live events carry Go durations in nanoseconds; the list carries seconds. */
export function nanosecondsToSeconds(ns: number): number {
	return Math.round(ns / NANOSECONDS_PER_SECOND);
}

export function formatSpeed(bytesPerSecond: number): string {
	if (bytesPerSecond <= 0) {
		return '—';
	}
	if (bytesPerSecond >= BYTES_PER_MB) {
		return `${(bytesPerSecond / BYTES_PER_MB).toFixed(1)} MB/s`;
	}
	return `${Math.round(bytesPerSecond / BYTES_PER_KB)} KB/s`;
}

export function formatEta(seconds: number): string {
	if (seconds <= 0) {
		return '—';
	}
	const hours = Math.floor(seconds / SECONDS_PER_HOUR);
	const minutes = Math.floor((seconds % SECONDS_PER_HOUR) / SECONDS_PER_MINUTE);
	if (hours > 0) {
		return `${hours}h ${minutes}m`;
	}
	return minutes > 0 ? `${minutes}m` : `${seconds}s`;
}

export function formatPercent(progress: number): string {
	return `${Math.floor(progress * PERCENT)}%`;
}

export function formatRatio(ratio: number): string {
	return ratio.toFixed(2);
}
