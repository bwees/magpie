import { SvelteMap } from 'svelte/reactivity';
import type { DownloadProgressed, JobProgressed, PipelineRowResponse } from '$lib/sdk';
import { LiveEventType, type LiveEvents } from './activity-source';
import { TranscodeState, type LiveJob } from './pipeline';

const PERCENT = 100;

export function percentOf(progress: number): number {
	return Math.round(progress * PERCENT);
}

export function liveJob(progress: JobProgressed): LiveJob {
	return { state: progress.stage, percent: percentOf(progress.progress) };
}

/**
 * Live percents for pipeline rows, from the ephemeral DownloadProgressed and JobProgressed events.
 * They are never stored, so a refetch keeps them.
 */
export class LiveProgress {
	readonly #downloads = new SvelteMap<string, number>();
	readonly #jobs = new SvelteMap<string, LiveJob>();
	readonly #rows: () => PipelineRowResponse[];

	/** rows are the rows to follow; progress for anything else is ignored. */
	constructor(rows: () => PipelineRowResponse[]) {
		this.#rows = rows;
	}

	/** The row's live percent: its download's, else its running transcode's. */
	of(row: PipelineRowResponse): number | undefined {
		if (row.downloadId) return this.#downloads.get(row.downloadId);
		return this.job(row)?.percent;
	}

	/** The live progress of the row's running transcode. */
	job(row: PipelineRowResponse): LiveJob | undefined {
		const job = row.transcode?.state === TranscodeState.RUNNING ? row.transcode.jobId : null;
		return job ? this.#jobs.get(job) : undefined;
	}

	connect(live: LiveEvents): () => void {
		const stops = [
			live.on(LiveEventType.DOWNLOAD_PROGRESSED, (message) => {
				const progress = message.data as DownloadProgressed;
				if (this.#rows().some((row) => row.downloadId === progress.download)) {
					this.#downloads.set(progress.download, percentOf(progress.progress));
				}
			}),
			live.on(LiveEventType.JOB_PROGRESSED, (message) => {
				const progress = message.data as JobProgressed;
				if (this.#rows().some((row) => row.transcode?.jobId === progress.job)) {
					this.#jobs.set(progress.job, liveJob(progress));
				}
			})
		];
		return () => stops.forEach((stop) => stop());
	}
}
